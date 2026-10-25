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

// staticFiles serves files under /static/ from files: hashed names are
// cached for a year, others for an hour, and Range requests let videos seek.
// Folders are never listed.
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

		if hashedName.MatchString(name) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		ext := path.Ext(name)
		if ct := contentTypes[ext]; ct != "" {
			w.Header().Set("Content-Type", ct)
		} else if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		// Embedded files have no modification time; the hashed name (or
		// the short max-age) stands in for validators.
		http.ServeContent(w, r, name, time.Time{}, f)
	}
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
