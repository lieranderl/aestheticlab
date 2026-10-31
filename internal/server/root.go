package server

import (
	"io/fs"
	"net/http"
)

// rootFiles are the files frontend/public/ serves at the root;
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
