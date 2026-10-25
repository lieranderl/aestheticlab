package server_test

import (
	"net/http"
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
