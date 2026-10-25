package server_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lieranderl/aestheticlab/internal/assets"
	"github.com/lieranderl/aestheticlab/internal/server"
	"github.com/lieranderl/aestheticlab/static"
)

func imageSrc(t *testing.T, name string) string {
	t.Helper()
	img, ok := assets.Lookup(name)
	if !ok {
		t.Fatalf("the manifest has no image %s", name)
	}
	return img.Src
}

// Every /assets/ URL the Qwik site rendered (aestheticlab.be, 2026-10-25),
// and the pre-v4.0.1 laser photo, keep working after the cutover: cached
// pages, open tabs and image search still ask for them.
func TestQwikAssetURLsRedirectToTheirBuiltFiles(t *testing.T) {
	h := server.New(server.Config{Static: static.Files})
	cases := map[string]string{
		"/assets/BRvowC9i-chrome-manicure.webp":                 imageSrc(t, "gallery/atelier/chrome-manicure.jpg"),
		"/assets/BCwJRnnZ-cherry-gloss.webp":                    imageSrc(t, "gallery/cherry-gloss.jpg"),
		"/assets/4DypDJGv-lash-lift-result.webp":                imageSrc(t, "gallery/lash-lift-result.jpg"),
		"/assets/B63CrIvG-white-shimmer.webp":                   imageSrc(t, "gallery/white-shimmer.jpg"),
		"/assets/BOMCgWTb-tortoise.webp":                        imageSrc(t, "gallery/tortoise.jpg"),
		"/assets/B0kxtIDJ-universal.webp":                       imageSrc(t, "gallery/universal.jpg"),
		"/assets/B0kxtIDJ-lazer1.webp":                          imageSrc(t, "gallery/universal.jpg"),
		"/assets/BOVeHZP8-taupe-manicure.webp":                  imageSrc(t, "hero/taupe-manicure.jpg"),
		"/assets/B2h8RWR--julia.webp":                           imageSrc(t, "julia.jpg"),
		"/assets/BCXHdqX5-zara.webp":                            imageSrc(t, "zara.jpg"),
		"/assets/-XybHeW8-laser-male.webp":                      imageSrc(t, "services/laser-male.webp"),
		"/assets/8hLra4ph-laser.webp":                           imageSrc(t, "services/laser.jpg"),
		"/assets/B-pJbk-t-laser-body.webp":                      imageSrc(t, "services/laser-body.webp"),
		"/assets/BRGKtw17-manicure.webp":                        imageSrc(t, "services/manicure.jpg"),
		"/assets/B6hEB8O1-pedicure.webp":                        imageSrc(t, "services/pedicure.jpg"),
		"/assets/BlGTXqkC-face-waxing.webp":                     imageSrc(t, "services/face-waxing.jpg"),
		"/assets/BY9zqYgm-reset-poster.jpeg":                    assets.URL("video/reset-poster.jpg"),
		"/assets/2tfgljIk-chrome.mp4":                           assets.URL("video/chrome.mp4"),
		"/assets/B8slvpc5-shimmer.mp4":                          assets.URL("video/shimmer.mp4"),
		"/assets/BBxOatbv-onest-vietnamese-wght-normal.woff2":   assets.URL("fonts/onest-vietnamese-wght-normal.woff2"),
		"/assets/Dun_Rd-l-onest-latin-wght-normal.woff2":        assets.URL("fonts/onest-latin-wght-normal.woff2"),
		"/assets/DOFToQv1-onest-cyrillic-ext-wght-normal.woff2": assets.URL("fonts/onest-cyrillic-ext-wght-normal.woff2"),
		// The Qwik stylesheet loads these fonts from public/fonts.
		"/fonts/cormorant-garamond-600-cyrillic.woff2": assets.URL("fonts/cormorant-garamond-600-cyrillic.woff2"),
		"/fonts/QESTERO-Regular.ttf":                   assets.URL("fonts/QESTERO-Regular.ttf"),
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if want == "" || !strings.HasPrefix(want, "/static/dist/") {
				t.Fatalf("want %q is not a built file", want)
			}
			res := get(t, h, path)
			if res.Code != http.StatusMovedPermanently || res.Header().Get("Location") != want {
				t.Fatalf("status %d, Location %q; want 301 to %s", res.Code, res.Header().Get("Location"), want)
			}
			// A cached redirect to a hashed name outlives that name once the
			// file changes; a day bounds that.
			if got := res.Header().Get("Cache-Control"); got != "public, max-age=86400" {
				t.Errorf("Cache-Control = %q, want a day", got)
			}
			if res := get(t, h, want); res.Code != http.StatusOK {
				t.Errorf("%s: status %d, want the embedded file", want, res.Code)
			}
		})
	}
}

func TestUnknownQwikAssetsAreNotFound(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{
		"/assets/MhiRVR3d-style.css",          // Qwik's own stylesheet: nothing to redirect to
		"/assets/BAAAAAAA-gone.webp",          // no such source
		"/assets/tortoise.webp",               // no hash
		"/assets/BOMCgWTb-tortoise.mp4",       // an image is not a video
		"/assets/BOMCgWTb-reset-poster.woff2", // nor a font
		"/fonts/gone.woff2",
		"/assets/",
		"/fonts/",
	} {
		t.Run(path, func(t *testing.T) {
			res := get(t, h, path)
			if res.Code != http.StatusNotFound || res.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("status %d, cache %q; want an uncached 404", res.Code, res.Header().Get("Cache-Control"))
			}
		})
	}
}

// The Qwik site's public/ files keep their root URLs.
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

// A tab opened on the Qwik site keeps running Qwik code after the cutover.
// Its next lazy-loaded chunk reloads the tab, which then gets the Go page.
func TestQwikChunksReloadTheTab(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{"/build/en-BE/q-DjKA_qaK.js", "/build/q-naDMFAHy.js"} {
		t.Run(path, func(t *testing.T) {
			res := get(t, h, path)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.Code)
			}
			if got := res.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
				t.Errorf("Content-Type = %q, want JavaScript", got)
			}
			if got := res.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if !strings.Contains(res.Body.String(), "location.reload()") {
				t.Errorf("body = %q, want a reload", res.Body.String())
			}
		})
	}
	for _, path := range []string{"/build/", "/build/en-BE/", "/build/en-BE/q-manifest.json"} {
		if res := get(t, h, path); res.Code != http.StatusNotFound || res.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: status %d, cache %q; want an uncached 404", path, res.Code, res.Header().Get("Cache-Control"))
		}
	}
}

// Qwik City loads a page's data from …/q-data.json when a link is followed
// inside an open tab. A response that is not JSON makes it load the page
// itself instead.
func TestQwikRouteDataIsNotFoundSoTheTabLoadsThePage(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{"/en-BE/q-data.json", "/fr-BE/pricelist/q-data.json", "/q-data.json"} {
		t.Run(path, func(t *testing.T) {
			res := get(t, h, path)
			if res.Code != http.StatusNotFound || res.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status %d, cache %q; want an uncached 404", res.Code, res.Header().Get("Cache-Control"))
			}
			if got := res.Header().Get("Content-Type"); strings.Contains(got, "json") {
				t.Errorf("Content-Type = %q, which Qwik would parse as route data", got)
			}
		})
	}
}

// Qwik registered /service-worker.js. Its replacement removes itself and
// the caches it kept the next time the browser checks it for updates.
func TestTheQwikServiceWorkerIsReplacedByOneThatRemovesItself(t *testing.T) {
	res := get(t, server.New(server.Config{}), "/service-worker.js")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JavaScript", got)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	for _, want := range []string{"skipWaiting()", "caches.delete", "registration.unregister()"} {
		if !strings.Contains(res.Body.String(), want) {
			t.Errorf("body lacks %s: %s", want, res.Body.String())
		}
	}
}
