package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
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
	store := bookableContent(t)
	store.Prefetch()
	h := server.New(server.Config{Content: store})
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

type failingContent struct{ fetches atomic.Int32 }

func (s *failingContent) Fetch(context.Context) (content.Documents, error) {
	s.fetches.Add(1)
	return content.Documents{}, errors.New("unavailable")
}

func (s *failingContent) Ping(context.Context) error {
	s.fetches.Add(1)
	return errors.New("unavailable")
}

func TestDependencyzDoesNotStartContentReads(t *testing.T) {
	for _, prefetched := range []bool{false, true} {
		source := &failingContent{}
		store := content.NewStore(content.Options{Source: source})
		if prefetched {
			store.Prefetch()
		}
		h := server.New(server.Config{Content: store})
		for range 3 {
			if res := get(t, h, "/dependencyz"); res.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", res.Code)
			}
		}
		want := 0
		if prefetched {
			want = 1
		}
		if source.fetches.Load() != int32(want) {
			t.Fatalf("%d source calls, want %d startup calls", source.fetches.Load(), want)
		}
	}
}

func TestDependencyzFailsWhenTheContentDatabaseDoesNot(t *testing.T) {
	store := content.NewStore(content.Options{Source: &failingContent{}})
	store.Prefetch()
	for name, cfg := range map[string]server.Config{
		"no database":      {},
		"failing database": {Content: store},
	} {
		t.Run(name, func(t *testing.T) {
			res := get(t, server.New(cfg), "/dependencyz")
			if res.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", res.Code)
			}
			if got := res.Body.String(); got != "Unavailable" {
				t.Errorf("body = %q, want no detail", got)
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
