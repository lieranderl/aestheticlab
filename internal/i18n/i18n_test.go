package i18n_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

func in(l i18n.Locale) context.Context {
	return i18n.WithLocale(context.Background(), l)
}

func TestTheSiteSpeaksFiveLocalesWithEnglishFirstByDefault(t *testing.T) {
	want := []i18n.Locale{"en-BE", "ru-BE", "nl-BE", "fr-BE", "uk-BE"}
	if got := i18n.Supported(); strings.Join(codes(got), ",") != strings.Join(codes(want), ",") {
		t.Errorf("Supported = %v, want %v (the Qwik site's order)", got, want)
	}
	if i18n.Default != "en-BE" {
		t.Errorf("Default = %q", i18n.Default)
	}
}

func codes(ls []i18n.Locale) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = string(l)
	}
	return out
}

func TestLocalesHaveTheirLanguageAndNativeName(t *testing.T) {
	for _, tc := range []struct {
		l          i18n.Locale
		lang, name string
	}{
		{i18n.EnBE, "en", "English"},
		{i18n.NlBE, "nl", "Nederlands"},
		{i18n.FrBE, "fr", "Français"},
		{i18n.RuBE, "ru", "Русский"},
		{i18n.UkBE, "uk", "Українська"},
	} {
		if got := tc.l.Language(); got != tc.lang {
			t.Errorf("%s.Language() = %q, want %q", tc.l, got, tc.lang)
		}
		if got := tc.l.NativeName(); got != tc.name {
			t.Errorf("%s.NativeName() = %q, want %q", tc.l, got, tc.name)
		}
	}
}

// As on the Qwik site, a path's first segment names its locale exactly
// (case included); any other path is English.
func TestThePathsFirstSegmentNamesTheLocale(t *testing.T) {
	for path, want := range map[string]i18n.Locale{
		"/nl-BE/":              i18n.NlBE,
		"/fr-BE/pricelist/":    i18n.FrBE,
		"/uk-BE/old-link/x/":   i18n.UkBE,
		"/ru-BE":               i18n.RuBE,
		"/en-be/":              i18n.EnBE,
		"/de-DE/pricelist/":    i18n.EnBE,
		"/xyz/":                i18n.EnBE,
		"/":                    i18n.EnBE,
		"":                     i18n.EnBE,
		"/nl-BEx/":             i18n.EnBE,
		"/static/dist/app.css": i18n.EnBE,
	} {
		if got := i18n.FromPath(path); got != want {
			t.Errorf("FromPath(%q) = %q, want %q", path, got, want)
		}
	}
	if l, ok := i18n.Parse("fr-BE"); !ok || l != i18n.FrBE {
		t.Errorf("Parse(fr-BE) = %q, %v", l, ok)
	}
	if _, ok := i18n.Parse("fr"); ok {
		t.Error("Parse(fr) found a locale")
	}
}

func TestARequestWithoutALocaleIsEnglish(t *testing.T) {
	if got := i18n.FromContext(context.Background()); got != i18n.EnBE {
		t.Errorf("FromContext(empty) = %q", got)
	}
}

// Texts carried over from the Qwik site's locale files read the same.
func TestTextsAreTranslatedByTheirEnglish(t *testing.T) {
	for _, tc := range []struct {
		l         i18n.Locale
		msg, want string
	}{
		{i18n.EnBE, "Book appointment", "Book appointment"},
		{i18n.NlBE, "Open navigation menu", "Menu openen"},
		{i18n.RuBE, "Open navigation menu", "Открыть меню"},
		{i18n.UkBE, "Back to categories", "Назад до категорій"},
		{i18n.FrBE, "Back to categories", "Retour aux catégories"},
	} {
		if got := i18n.T(in(tc.l), tc.msg); got != tc.want {
			t.Errorf("T(%s, %q) = %q, want %q", tc.l, tc.msg, got, tc.want)
		}
	}
}

// Two English texts are translated differently depending on where they
// stand; the context picks which.
func TestAContextTellsApartTextsThatShareTheirEnglish(t *testing.T) {
	for _, tc := range []struct {
		l                  i18n.Locale
		context, msg, want string
	}{
		{i18n.NlBE, "navigation landmark", "Open navigation menu", "Mobiele navigatie"},
		{i18n.FrBE, "treatment overview", "Back to categories", "Retour à l'aperçu"},
		{i18n.EnBE, "treatment overview", "Back to categories", "Back to categories"},
	} {
		if got := i18n.C(in(tc.l), tc.context, tc.msg); got != tc.want {
			t.Errorf("C(%s, %q, %q) = %q, want %q", tc.l, tc.context, tc.msg, got, tc.want)
		}
	}
}

func TestPlaceholdersAreFilledInEveryLocale(t *testing.T) {
	for _, tc := range []struct {
		l    i18n.Locale
		want string
	}{
		{i18n.EnBE, "Book with Julia"},
		{i18n.NlBE, "Boek bij Julia"},
	} {
		if got := i18n.Tf(in(tc.l), "Book with %v", "Julia"); got != tc.want {
			t.Errorf("Tf(%s) = %q, want %q", tc.l, got, tc.want)
		}
	}
	got := i18n.Tf(in(i18n.EnBE), "From %[1]v to %[2]v minutes, depending on the treatment. Every treatment’s time is on the <a href='./pricelist'>price list</a>.", 30, 120)
	if !strings.HasPrefix(got, "From 30 to 120 minutes") {
		t.Errorf("Tf with two placeholders = %q", got)
	}
}

func TestAnUnknownTextIsShownInEnglish(t *testing.T) {
	if got := i18n.T(in(i18n.RuBE), "No such text"); got != "No such text" {
		t.Errorf("T(unknown) = %q", got)
	}
	if i18n.Has(i18n.RuBE, "No such text") {
		t.Error("Has(unknown) = true")
	}
}

// Every text the Qwik site shows has a translation in every locale, so the
// rewrite cannot silently fall back to English.
func TestEveryTextHasATranslationInEveryLocale(t *testing.T) {
	msgs := i18n.Messages()
	if len(msgs) < 200 {
		t.Fatalf("only %d texts: the tables were not generated (make i18n)", len(msgs))
	}
	for _, l := range i18n.Supported() {
		for _, msg := range msgs {
			if !i18n.Has(l, msg) {
				t.Errorf("%s has no translation of %q", l, msg)
			}
		}
	}
}
