package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/server"
)

// The URLs behave as the Qwik site's do (measured on staging, 2026-10-25).

func TestTheRootRedirectsToTheEnglishHomePage(t *testing.T) {
	res := get(t, server.New(server.Config{}), "/")
	if res.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.Code)
	}
	if got := res.Header().Get("Location"); got != "/en-BE/" {
		t.Errorf("Location = %q, want /en-BE/", got)
	}
}

func TestPagesWithoutATrailingSlashRedirectToTheirSlashedURL(t *testing.T) {
	h := server.New(server.Config{})
	for path, want := range map[string]string{
		"/nl-BE":                          "/nl-BE/",
		"/en-BE/pricelist":                "/en-BE/pricelist/",
		"/nl-BE/pricelist?category=laser": "/nl-BE/pricelist/?category=laser",
		"/xyz":                            "/xyz/",
	} {
		res := get(t, h, path)
		if res.Code != http.StatusMovedPermanently {
			t.Errorf("%s: status = %d, want 301", path, res.Code)
			continue
		}
		if got := res.Header().Get("Location"); got != want {
			t.Errorf("%s: Location = %q, want %q", path, got, want)
		}
	}
}

// Files keep their names: /favicon.svg and /index.html are not pages.
func TestFileLikePathsAreNotRedirected(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{"/favicon.svg", "/index.html", "/static/gone.css", "/healthz"} {
		if res := get(t, h, path); res.Code == http.StatusMovedPermanently {
			t.Errorf("%s was redirected to %s", path, res.Header().Get("Location"))
		}
	}
}

// An unknown page is a 404 in the locale its path names, else English.
func TestUnknownPagesAreNotFoundInTheirPathsLocale(t *testing.T) {
	h := server.New(server.Config{})
	for _, tc := range []struct{ path, locale, text string }{
		{"/nl-BE/old-link/", "nl-BE", "Deze pagina is verhuisd"},
		{"/fr-BE/x/y/", "fr-BE", "Cette page a déménagé"},
		{"/de-DE/", "en-BE", "This page has moved on"},
		{"/en-be/", "en-BE", "This page has moved on"},
		{"/xyz/", "en-BE", "This page has moved on"},
	} {
		res := get(t, h, tc.path)
		if res.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", tc.path, res.Code)
			continue
		}
		if got := res.Header().Get("Content-Language"); got != tc.locale {
			t.Errorf("%s: Content-Language = %q, want %q", tc.path, got, tc.locale)
		}
		if got := res.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", tc.path, got)
		}
		if tc.text != "" && !strings.Contains(res.Body.String(), tc.text) {
			t.Errorf("%s: body %q lacks %q", tc.path, res.Body.String(), tc.text)
		}
	}
}

// Adding the slash must never send a browser to another site: a Location
// of //evil.example/ or /\\evil.example/ is protocol-relative.
func TestTheSlashRedirectNeverLeavesTheSite(t *testing.T) {
	h := server.New(server.Config{})
	for _, tc := range []struct{ path, rawPath string }{
		{"//evil.example", ""},
		{"//evil.example", "/%2Fevil.example"},
		{"/\\evil.example", ""},
		{"/\\evil.example", "/%5Cevil.example"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path, req.URL.RawPath = tc.path, tc.rawPath
		res := serve(t, h, req)
		loc := res.Header().Get("Location")
		if loc == "" {
			continue
		}
		if !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") || strings.HasPrefix(loc, "/\\") {
			t.Errorf("%q (raw %q) redirected off the site: %d %q", tc.path, tc.rawPath, res.Code, loc)
		}
	}
}
