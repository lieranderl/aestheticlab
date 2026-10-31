package server_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lieranderl/aestheticlab/internal/server"
)

// The site's public/ files are served at the root.
func TestRootFilesKeepTheirURLs(t *testing.T) {
	files := fstest.MapFS{}
	want := map[string]string{
		"favicon.svg":   "image/svg+xml",
		"icon-192.svg":  "image/svg+xml",
		"icon-512.svg":  "image/svg+xml",
		"manifest.json": "application/json",
		"robots.txt":    "text/plain; charset=utf-8",
		"sitemap.xml":   "text/xml; charset=utf-8",
	}
	for name := range want {
		files["dist/root/"+name] = &fstest.MapFile{Data: []byte("the " + name)}
	}
	h := server.New(server.Config{Static: files})
	for name, contentType := range want {
		t.Run(name, func(t *testing.T) {
			res := get(t, h, "/"+name)
			if res.Code != http.StatusOK || res.Body.String() != "the "+name {
				t.Fatalf("status %d, body %.100q", res.Code, res.Body.String())
			}
			if got := res.Header().Get("Content-Type"); got != contentType {
				t.Errorf("Content-Type = %q, want %q", got, contentType)
			}
			if got := res.Header().Get("Cache-Control"); got != "public, max-age=3600" {
				t.Errorf("Cache-Control = %q, want an hour", got)
			}
		})
	}
	// Only those names: root/ is not a folder on the site.
	for _, path := range []string{"/root/robots.txt", "/static/root/robots.txt"} {
		if res := get(t, h, path); res.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, res.Code)
		}
	}
}

// The Qwik site's URLs are gone: old image and font URLs, lazy-loaded
// chunks, route data and the service worker all get the branded 404.
func TestTheQwikSitesURLsAreNotFound(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{
		"/assets/BRvowC9i-chrome-manicure.webp",
		"/assets/B0kxtIDJ-lazer1.webp",
		"/fonts/QESTERO-Regular.ttf",
		"/build/en-BE/q-DjKA_qaK.js",
		"/en-BE/pricelist/q-data.json",
		"/service-worker.js",
	} {
		t.Run(path, func(t *testing.T) {
			res := get(t, h, path)
			if res.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", res.Code)
			}
			if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Errorf("Content-Type = %q, want the branded page", got)
			}
			if got := res.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}
