package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/server"
)

// get sends a GET for path through the site's handler.
func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(t, h, httptest.NewRequest(http.MethodGet, path, nil))
}

// serve sends req through the site's handler.
func serve(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProbesAnswerOKWithoutCaching(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{"/healthz", "/readyz", "/dependencyz"} {
		t.Run(path, func(t *testing.T) {
			res := get(t, h, path)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.Code)
			}
			if got := res.Body.String(); got != "OK" {
				t.Errorf("body = %q, want %q", got, "OK")
			}
			if got := res.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

// (/nope itself is first redirected to /nope/, as on the Qwik site.)
func TestUnknownPathIsNotFound(t *testing.T) {
	res := get(t, server.New(server.Config{}), "/nope/")
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.Code)
	}
}
