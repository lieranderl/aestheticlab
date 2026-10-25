package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/server"
)

func TestThePricelistIsServedInEachLocale(t *testing.T) {
	h := server.New(server.Config{})
	for _, l := range []string{"en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"} {
		res := get(t, h, "/"+l+"/pricelist/")
		if res.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", l, res.Code)
		}
		for name, want := range map[string]string{
			"Content-Type":     "text/html; charset=utf-8",
			"Content-Language": l,
			"Cache-Control":    "public, max-age=60, stale-while-revalidate=600",
		} {
			if got := res.Header().Get(name); got != want {
				t.Errorf("%s: %s = %q, want %q", l, name, got, want)
			}
		}
		if body := res.Body.String(); !strings.Contains(body, `<html lang="`+l+`">`) || !strings.Contains(body, `id="pricelist-tabs"`) {
			t.Errorf("%s: the page is not the pricelist in its locale", l)
		}
	}
}

func TestAPricelistUnderAnUnknownLocaleIsNotFound(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{"/de-BE/pricelist/", "/en-be/pricelist/"} {
		if res := get(t, h, path); res.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, res.Code)
		}
	}
}

func TestTheHomePageIsServedInEachLocale(t *testing.T) {
	h := server.New(server.Config{})
	for _, l := range []string{"en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"} {
		res := get(t, h, "/"+l+"/")
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `id="hero"`) {
			t.Fatalf("%s: status %d, want the home page", l, res.Code)
		}
	}
}

// A tap in the services section asks Datastar for that section only.
func TestADatastarRequestGetsOnlyTheServicesSection(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/en-BE/?treatment=laser", nil)
	req.Header.Set("Datastar-Request", "true")
	res := serve(t, server.New(server.Config{}), req)
	if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want an event stream", got)
	}
	body := res.Body.String()
	if !strings.Contains(body, "event: datastar-patch-elements") || !strings.Contains(body, `id="services-body"`) {
		t.Errorf("the response is not a patch of the services body: %.200s", body)
	}
	if strings.Contains(body, "<html") || strings.Contains(body, `id="hero"`) {
		t.Error("the response carries the whole page")
	}
}

func TestThePolicyPagesAreServedInEachLocale(t *testing.T) {
	h := server.New(server.Config{})
	for _, l := range []string{"en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"} {
		for _, p := range []string{"privacy-policy", "notice"} {
			res := get(t, h, "/"+l+"/"+p+"/")
			if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `<html lang="`+l+`">`) {
				t.Errorf("/%s/%s/: status %d, want the page in its locale", l, p, res.Code)
			}
		}
	}
	privacy := get(t, h, "/en-BE/privacy-policy/").Body.String()
	if got := strings.Count(privacy, `<section id="privacy-`); got != 8 {
		t.Errorf("the privacy policy has %d sections, want 8", got)
	}
	if strings.Contains(privacy, ">1. Information") {
		t.Error("section titles keep their translated numbers")
	}
	for path, want := range map[string]string{"/en-BE/notice/": "Nails", "/en-BE/notice/?tab=laser": "Laser"} {
		body := get(t, h, path).Body.String()
		if !strings.Contains(body, `aria-label="`+want+`" checked`) {
			t.Errorf("%s: the %s tab is not selected", path, want)
		}
	}
}

// As e2e/seo-head.spec.ts checks the Qwik site's.
func TestUnknownLinksGetTheBrandedLocalizedPage(t *testing.T) {
	res := get(t, server.New(server.Config{}), "/nl-BE/oude-link/")
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.Code)
	}
	body := res.Body.String()
	for _, want := range []string{
		"Deze pagina is verhuisd",
		`<meta name="robots" content="noindex">`,
		`href="/nl-BE/#services"`, `href="/nl-BE/pricelist/"`, `href="/nl-BE/#contact"`,
		`id="not-found-book-btn"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the 404 page lacks %s", want)
		}
	}
	if strings.Contains(body, `id="bottom-bar-book-btn"`) {
		t.Error("the 404 page shows the phone booking bar; it has its own Book button")
	}
}
