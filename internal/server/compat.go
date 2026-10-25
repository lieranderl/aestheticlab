package server

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/lieranderl/aestheticlab/internal/assets"
)

// This file keeps the Qwik site's URLs working after the cutover, for open
// tabs, cached pages and links elsewhere. Phase 10 decides how long.

// rootFiles are the files the Qwik site served from public/ at the root;
// frontend/build.mjs copies them to dist/root/.
var rootFiles = []string{"favicon.svg", "icon-192.svg", "icon-512.svg", "manifest.json", "robots.txt", "sitemap.xml"}

// rootFile serves dist/root/name at /name, with staticFiles' short cache.
func rootFile(files fs.FS, name string) http.HandlerFunc {
	serve := staticFiles(files)
	return func(w http.ResponseWriter, r *http.Request) {
		forwarded := r.Clone(r.Context())
		forwarded.URL.Path = "/static/dist/root/" + name
		serve(w, forwarded)
	}
}

// qwikAsset matches the Qwik build's file names: an 8-character Vite hash,
// a dash, then the source file's base name (BRvowC9i-chrome-manicure.webp;
// a hash may itself contain dashes, as in B-pJbk-t-laser-body.webp).
var qwikAsset = regexp.MustCompile(`^[A-Za-z0-9_-]{8}-([A-Za-z0-9_-]+)\.([a-z0-9]+)$`)

// kinds groups extensions that one source may have been served under: the
// Qwik site served raster images as WebP and posters as JPEG.
var kinds = map[string]string{
	".webp": "image", ".jpg": "image", ".jpeg": "image", ".png": "image", ".svg": "image",
	".mp4": "video", ".woff2": "font", ".ttf": "font",
}

// builtByStem maps "<kind>/<base name without extension>" to the built URL
// a Qwik asset of that name redirects to: an image's largest variant, or
// the hashed copy of a video, poster or font. A name two sources share
// maps to "", which nothing redirects to.
var builtByStem = sync.OnceValue(func() map[string]string {
	byStem := map[string]string{}
	add := func(name, url string) {
		ext := path.Ext(name)
		key := kinds[strings.ToLower(ext)] + "/" + strings.TrimSuffix(path.Base(name), ext)
		if _, taken := byStem[key]; taken {
			url = ""
		}
		byStem[key] = url
	}
	files, images := assets.Names()
	for _, name := range files {
		if kinds[path.Ext(name)] != "" {
			add(name, assets.URL(name))
		}
	}
	for _, name := range images {
		img, _ := assets.Lookup(name)
		add(name, img.Src)
	}
	// Before v4.0.1 the Qwik site published the universal photo under its
	// byte-identical twin's name.
	byStem["image/lazer1"] = byStem["image/universal"]
	return byStem
})

// legacyRedirect answers /assets/<hash>-<name>.<ext> and /fonts/<name> with
// a 301 to the file built from the same source, or an uncached 404.
func legacyRedirect(w http.ResponseWriter, r *http.Request) {
	dir, file := path.Split(r.URL.Path)
	var stem, ext string
	switch dir {
	case "/assets/":
		if m := qwikAsset.FindStringSubmatch(file); m != nil {
			stem, ext = m[1], "."+m[2]
		}
	case "/fonts/":
		ext = path.Ext(file)
		stem = strings.TrimSuffix(file, ext)
	}
	target := ""
	if kind := kinds[ext]; kind != "" {
		target = builtByStem()[kind+"/"+stem]
	}
	if target == "" {
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
		return
	}
	// Browsers keep a 301 indefinitely unless told otherwise, and the
	// target's hashed name changes with the file.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// reloadModule is every Qwik chunk after the cutover. Qwik loads chunks
// with import(), so the body runs, and the reloaded tab gets the Go page.
const reloadModule = "// This tab runs the previous version of the site. Load the current one.\nlocation.reload();\n"

// qwikChunk answers /build/…/*.js with reloadModule, never cached.
func qwikChunk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !strings.HasSuffix(r.URL.Path, ".js") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = io.WriteString(w, reloadModule)
}

// isQwikRouteData reports whether p is the route data Qwik City fetches
// for a page (/en-BE/pricelist/q-data.json) when a link is followed in an
// open tab.
func isQwikRouteData(p string) bool {
	return path.Base(p) == "q-data.json"
}

// qwikRouteData answers route data with a plain 404: anything but JSON
// makes Qwik City load the page itself, and the full branded 404 would be
// wasted on a fetch nobody sees.
func qwikRouteData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.NotFound(w, r)
}

// serviceWorkerKillSwitch replaces the worker the Qwik site registered at
// /service-worker.js. Browsers check that URL for updates; this version
// installs at once, deletes the caches Qwik kept (the site keeps no others),
// then unregisters itself. The Qwik worker handled no fetches, so no open
// page depends on it.
const serviceWorkerKillSwitch = `// The previous version of the site registered a service worker here.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.map((key) => caches.delete(key))))
			.then(() => self.registration.unregister()),
	);
});
`

func serviceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, serviceWorkerKillSwitch)
}
