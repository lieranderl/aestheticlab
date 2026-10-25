package server

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// hashedName matches a content-hashed file name, <name>.<8+ hex>.<ext>
// (app.3f2a9c1b.css): its bytes never change under that name.
var hashedName = regexp.MustCompile(`\.[0-9a-f]{8,}\.[a-z0-9]+$`)

// contentTypes covers the static types Go's own table lacks, so they never
// depend on the image's /etc/mime.types or on sniffing.
var contentTypes = map[string]string{
	".ico":         "image/x-icon",
	".mp4":         "video/mp4",
	".txt":         "text/plain; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".woff2":       "font/woff2",
}

// encodings are the precompressed siblings frontend/build.mjs writes next to
// text files (app.<hash>.css.br), in order of preference.
var encodings = []struct{ name, suffix string }{
	{"br", ".br"},
	{"gzip", ".gz"},
}

// shareImages keeps the public preview URLs advertised by the page head.
// Their unhashed names get staticFiles' short cache and no directory listings.
func shareImages(files fs.FS) http.HandlerFunc {
	serve := staticFiles(files)
	return func(w http.ResponseWriter, r *http.Request) {
		forwarded := r.Clone(r.Context())
		forwarded.URL.Path = "/static/dist" + r.URL.Path
		serve(w, forwarded)
	}
}

// staticFiles serves files under /static/ from files: hashed names are
// cached for a year, others for an hour, and Range requests let videos seek.
// A text file with precompressed siblings is served in the best encoding the
// browser accepts. Folders are never listed.
func staticFiles(files fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/static/")
		f, ok := openFile(files, name)
		if !ok {
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		}
		defer func() { _ = f.Close() }()

		h := w.Header()
		// Even a file with only an identity representation can answer 406
		// for a different Accept-Encoding. Shared caches must keep them apart.
		h.Add("Vary", "Accept-Encoding")
		if hashedName.MatchString(name) {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "public, max-age=3600")
		}
		ext := path.Ext(name)
		if ct := contentTypes[ext]; ct != "" {
			h.Set("Content-Type", ct)
		} else if ct := mime.TypeByExtension(ext); ct != "" {
			h.Set("Content-Type", ct)
		}

		preferences := parseAcceptEncoding(strings.Join(r.Header.Values("Accept-Encoding"), ","))
		bestQuality := preferences["identity"]
		selected := ""
		for _, enc := range encodings {
			packed, ok := openFile(files, name+enc.suffix)
			if !ok {
				continue
			}
			q, explicit := preferences[enc.name]
			if !explicit {
				q = preferences["*"]
			}
			if q > 0 && (q > bestQuality || (q == bestQuality && selected == "")) {
				_ = f.Close()
				f = packed
				bestQuality, selected = q, enc.name
			} else {
				_ = packed.Close()
			}
		}
		identity, explicitIdentity := preferences["identity"]
		wildcard, hasWildcard := preferences["*"]
		identityAllowed := (explicitIdentity && identity > 0) || (!explicitIdentity && (!hasWildcard || wildcard > 0))
		if selected == "" && !identityAllowed {
			h.Set("Cache-Control", "no-store")
			h.Del("Content-Type")
			http.Error(w, "No acceptable content encoding", http.StatusNotAcceptable)
			return
		}
		if selected != "" {
			h.Set("Content-Encoding", selected)
		}

		// Embedded files have no modification time; the hashed name (or
		// the short max-age) stands in for validators.
		http.ServeContent(w, r, name, time.Time{}, f)
	}
}

// parseAcceptEncoding returns qualities in thousandths (RFC 9110, 12.5.3).
// Explicit codings override the wildcard. Identity is an acceptable fallback
// unless explicitly excluded; its preference is compared only when supplied.
// Invalid entries and conflicting duplicate entries fail closed.
func parseAcceptEncoding(header string) map[string]int {
	qualities := make(map[string]int)
	for part := range strings.SplitSeq(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		q := 1000
		if params != "" {
			key, value, found := strings.Cut(strings.TrimSpace(params), "=")
			q = 0
			if found && strings.EqualFold(strings.TrimSpace(key), "q") {
				q = encodingQuality(strings.TrimSpace(value))
			}
		}
		if previous, exists := qualities[name]; exists {
			q = min(q, previous)
		}
		qualities[name] = q
	}
	return qualities
}

func encodingQuality(value string) int {
	whole, fraction, _ := strings.Cut(value, ".")
	if (whole != "0" && whole != "1") || len(fraction) > 3 {
		return 0
	}
	q, place := 0, 100
	for _, digit := range fraction {
		if digit < '0' || digit > '9' || (whole == "1" && digit != '0') {
			return 0
		}
		q += int(digit-'0') * place
		place /= 10
	}
	if whole == "1" {
		return 1000
	}
	return q
}

// openFile opens the regular file name in files, if there is one.
func openFile(files fs.FS, name string) (seekableFile, bool) {
	if files == nil || !fs.ValidPath(name) || name == "." {
		return nil, false
	}
	f, err := files.Open(name)
	if err != nil {
		return nil, false
	}
	info, err := f.Stat()
	sf, seekable := f.(seekableFile)
	if err != nil || !info.Mode().IsRegular() || !seekable {
		_ = f.Close()
		return nil, false
	}
	return sf, true
}

// seekableFile is an fs.File that http.ServeContent can serve ranges of, as
// embed.FS files are.
type seekableFile interface {
	fs.File
	io.Seeker
}
