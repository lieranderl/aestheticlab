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
