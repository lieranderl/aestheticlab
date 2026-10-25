package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/lieranderl/aestheticlab/internal/server"
)

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
