package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/lieranderl/aestheticlab/internal/server"
)

func TestShareImagesAreAvailableAtTheirPublicURLs(t *testing.T) {
	files := fstest.MapFS{}
	for _, locale := range []string{"en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"} {
		files["dist/og/"+locale+".jpg"] = &fstest.MapFile{Data: []byte("image for " + locale)}
	}
	h := server.New(server.Config{Static: files})
	for _, locale := range []string{"en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"} {
		t.Run(locale, func(t *testing.T) {
			res := get(t, h, "/og/"+locale+".jpg")
			if res.Code != http.StatusOK || res.Body.String() != "image for "+locale {
				t.Fatalf("status %d, body %.100q; want the locale's image", res.Code, res.Body.String())
			}
			if got := res.Header().Get("Content-Type"); got != "image/jpeg" {
				t.Errorf("Content-Type = %q, want image/jpeg", got)
			}
			if got := res.Header().Get("Cache-Control"); got != "public, max-age=3600" {
				t.Errorf("Cache-Control = %q, want a short cache for an unhashed URL", got)
			}
		})
	}
	for _, path := range []string{"/og/", "/og/de-BE.jpg", "/og/en-BE.jpg/"} {
		t.Run(path, func(t *testing.T) {
			res := get(t, h, path)
			if res.Code != http.StatusNotFound || res.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s: status %d, cache %q; want an uncached 404", path, res.Code, res.Header().Get("Cache-Control"))
			}
		})
	}
}

// staticSite serves a few made-up static files.
func staticSite() http.Handler {
	return server.New(server.Config{Static: fstest.MapFS{
		"app.3f2a9c1b.css":        {Data: []byte("body{}")},
		"favicon.svg":             {Data: []byte("<svg/>")},
		"media/clip.0a1b2c3d.mp4": {Data: []byte("0123456789")},
	}})
}

func TestHashedStaticFilesAreCachedForAYear(t *testing.T) {
	res := get(t, staticSite(), "/static/app.3f2a9c1b.css")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if got := res.Body.String(); got != "body{}" {
		t.Errorf("body = %q", got)
	}
	if got := res.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := res.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
}

func TestUnhashedStaticFilesAreCachedBriefly(t *testing.T) {
	res := get(t, staticSite(), "/static/favicon.svg")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if got := res.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q", got)
	}
}

func TestVideoCanBeSeekedWithRangeRequests(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/static/media/clip.0a1b2c3d.mp4", nil)
	req.Header.Set("Range", "bytes=2-5")
	res := serve(t, staticSite(), req)
	if res.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.Code)
	}
	if got := res.Body.String(); got != "2345" {
		t.Errorf("body = %q, want %q", got, "2345")
	}
	if got := res.Header().Get("Content-Range"); got != "bytes 2-5/10" {
		t.Errorf("Content-Range = %q", got)
	}
	if got := res.Header().Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestMissingStaticFilesAndFoldersAreNotFoundAndNotCached(t *testing.T) {
	h := staticSite()
	for _, path := range []string{"/static/gone.css", "/static/", "/static/media/", "/static/media"} {
		res := get(t, h, path)
		if res.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, res.Code)
		}
		if got := res.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", path, got)
		}
	}
}

// compressedSite has a stylesheet built with .br and .gz siblings, as
// frontend/build.mjs writes them, and an image without.
func compressedSite() http.Handler {
	return server.New(server.Config{Static: fstest.MapFS{
		"dist/app.3f2a9c1b00.css":    {Data: []byte("body{color:red}")},
		"dist/app.3f2a9c1b00.css.br": {Data: []byte("BROTLI")},
		"dist/app.3f2a9c1b00.css.gz": {Data: []byte("GZIP")},
		"dist/photo.3f2a9c1b00.webp": {Data: []byte("RIFF")},
	}})
}

func getWithEncoding(t *testing.T, h http.Handler, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	return serve(t, h, req)
}

func TestTextFilesAreServedPrecompressedToBrowsersThatAcceptIt(t *testing.T) {
	for _, tc := range []struct {
		accept, encoding, body string
	}{
		{"gzip, deflate, br, zstd", "br", "BROTLI"},
		{"gzip, deflate", "gzip", "GZIP"},
		{"br;q=0, gzip", "gzip", "GZIP"},
		{"BR", "br", "BROTLI"},
		{"identity", "", "body{color:red}"},
		{"", "", "body{color:red}"},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			res := getWithEncoding(t, compressedSite(), "/static/dist/app.3f2a9c1b00.css", tc.accept)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.Code)
			}
			if got := res.Header().Get("Content-Encoding"); got != tc.encoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.encoding)
			}
			if got := res.Body.String(); got != tc.body {
				t.Errorf("body = %q, want %q", got, tc.body)
			}
			if got := res.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
				t.Errorf("Content-Type = %q, want the stylesheet's own type", got)
			}
			if got := res.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q, want Accept-Encoding: caches must keep each encoding apart", got)
			}
			if got := res.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
				t.Errorf("Cache-Control = %q", got)
			}
		})
	}
}

func TestFilesWithoutCompressedSiblingsAreServedAsTheyAre(t *testing.T) {
	res := getWithEncoding(t, compressedSite(), "/static/dist/photo.3f2a9c1b00.webp", "gzip, br")
	if got := res.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none", got)
	}
	if got := res.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding: clients may reject identity", got)
	}
	if got := res.Body.String(); got != "RIFF" {
		t.Errorf("body = %q", got)
	}
}

func TestHeadRequestsGetStaticHeadersWithoutBody(t *testing.T) {
	res := serve(t, staticSite(), httptest.NewRequest(http.MethodHead, "/static/favicon.svg", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if got := res.Header().Get("Content-Length"); got != "6" {
		t.Errorf("Content-Length = %q, want 6", got)
	}
	if got := res.Body.String(); got != "" {
		t.Errorf("body = %q, want none", got)
	}
}

func TestStaticResponsesRespectEncodingPreferencesAndExclusions(t *testing.T) {
	for _, tc := range []struct {
		name, accept, encoding string
		status                 int
	}{
		{"prefer gzip", "gzip;q=1, br;q=0.1", "gzip", 200},
		{"prefer identity", "identity;q=1, br;q=0.5", "", 200},
		{"wildcard", "*;q=1, identity;q=0", "br", 200},
		{"specific exclusion beats wildcard", "*;q=1, br;q=0, identity;q=0", "gzip", 200},
		{"reject everything", "br;q=0,gzip;q=0,identity;q=0", "", 406},
		{"wildcard exclusion", "*;q=0", "", 406},
		{"explicit identity beats wildcard", "*;q=0, identity;q=0.5", "", 200},
		{"smallest positive quality", "br;q=0.001, gzip;q=0", "br", 200},
		{"tab whitespace", "br;\tq=0, gzip", "gzip", 200},
		{"invalid quality", "br;q=bogus, gzip;q=1", "gzip", 200},
		{"quality above one", "br;q=1.1, gzip;q=0.5", "gzip", 200},
		{"too many quality digits", "br;q=0.0001, gzip", "gzip", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := getWithEncoding(t, compressedSite(), "/static/dist/app.3f2a9c1b00.css", tc.accept)
			if res.Code != tc.status || res.Header().Get("Content-Encoding") != tc.encoding {
				t.Errorf("status/encoding = %d/%q, want %d/%q", res.Code, res.Header().Get("Content-Encoding"), tc.status, tc.encoding)
			}
			if res.Header().Get("Vary") != "Accept-Encoding" {
				t.Error("response does not vary by Accept-Encoding")
			}
			if tc.status == 406 && res.Header().Get("Cache-Control") != "no-store" {
				t.Error("unacceptable representation can be cached")
			}
		})
	}
}

func TestEncodingPreferencesUseAllHeaderLinesAndAvailableFiles(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/static/dist/app.3f2a9c1b00.css", nil)
	req.Header.Add("Accept-Encoding", "br;q=0.1")
	req.Header.Add("Accept-Encoding", "gzip;q=1")
	res := serve(t, compressedSite(), req)
	if got := res.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("encoding = %q, want gzip from the second header line", got)
	}
	res = getWithEncoding(t, compressedSite(), "/static/dist/photo.3f2a9c1b00.webp", "br, identity;q=0")
	if res.Code != http.StatusNotAcceptable {
		t.Errorf("uncompressed-only image status = %d, want 406", res.Code)
	}
}

func TestUncompressedAssetsKeepEncodingExclusionsSafeAcrossCaches(t *testing.T) {
	h := compressedSite()
	path := "/static/dist/photo.3f2a9c1b00.webp"
	accepted := getWithEncoding(t, h, path, "br")
	if accepted.Code != http.StatusOK || accepted.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("cacheable identity response = %d, Vary %q; must not be reused for clients rejecting identity", accepted.Code, accepted.Header().Get("Vary"))
	}
	rejected := getWithEncoding(t, h, path, "br, identity;q=0")
	if rejected.Code != http.StatusNotAcceptable || rejected.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("excluded identity response = %d, Cache-Control %q", rejected.Code, rejected.Header().Get("Cache-Control"))
	}
}
