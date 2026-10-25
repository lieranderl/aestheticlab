package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestRepeatedPlaceholdersReuseTheirArgument(t *testing.T) {
	for _, tc := range []struct {
		name, english, translation, want string
	}{
		{"English repetition", "Hello {{name}}, {{name}}", "Hallo {{name}}, {{name}}", "Hallo Julia, Julia"},
		{"translation-only repetition", "Hello {{name}}", "{{name}}, hallo {{name}}", "Julia, hallo Julia"},
		{"repetition with percent", "Save 30% for {{name}}", "30% voor {{name}}, {{name}}", "30% voor Julia, Julia"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			translated := formatText(tc.translation, placeholders(tc.english))
			if got := fmt.Sprintf(translated, "Julia"); got != tc.want {
				t.Errorf("formatted translation = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPlaceholdersBecomeFmtVerbsInTheEnglishOrder(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"Book with {{name}}", "Book with %v"},
		{"From {{min}} to {{max}} minutes", "From %[1]v to %[2]v minutes"},
		{"Up to 30% off for {{name}}", "Up to 30%% off for %v"},
		{"Save 30% today", "Save 30% today"},
	} {
		if got := formatText(tc.text, placeholders(tc.text)); got != tc.want {
			t.Errorf("formatText(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
	// A translation takes the English text's numbering, whatever order it
	// names the placeholders in.
	if got := formatText("{{max}} à {{min}}", []string{"min", "max"}); got != "%[2]v à %[1]v" {
		t.Errorf("reordered translation = %q", got)
	}
}

func tables(en, nl map[string]string) map[string]map[string]string {
	all := map[string]map[string]string{"en-BE": en}
	for _, l := range locales[1:] {
		all[l.code] = nl
	}
	return all
}

func TestSharedEnglishWithDifferentTranslationsNeedsAContext(t *testing.T) {
	en := map[string]string{"app.a.x": "Back", "app.b.y": "Back"}
	nl := map[string]string{"app.a.x": "Terug", "app.b.y": "Vorige"}
	_, _, err := render([]string{"app.a.x", "app.b.y"}, tables(en, nl))
	if err == nil || !strings.Contains(err.Error(), "add one to contexts") {
		t.Fatalf("err = %v, want a request for a context", err)
	}
}

func TestSharedEnglishWithTheSameTranslationsIsOneText(t *testing.T) {
	en := map[string]string{"app.a.x": "Back", "app.b.y": "Back"}
	nl := map[string]string{"app.a.x": "Terug", "app.b.y": "Terug"}
	src, _, err := render([]string{"app.a.x", "app.b.y"}, tables(en, nl))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), `"Back": "Terug"`); n != len(locales)-1 {
		t.Errorf("Back appears %d times in the tables, want once per locale (%d):\n%s", n, len(locales)-1, src)
	}
}

func TestAMissingTranslationOrPlaceholderFailsTheConversion(t *testing.T) {
	en := map[string]string{"app.a.x": "Book with {{name}}", "app.b.y": "Hello"}
	nl := map[string]string{"app.a.x": "Boek bij {{naam}}"}
	_, _, err := render([]string{"app.a.x", "app.b.y"}, tables(en, nl))
	if err == nil {
		t.Fatal("render succeeded")
	}
	for _, want := range []string{"app.a.x: nl-BE has placeholders [naam], English [name]", "app.b.y: no nl-BE translation"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}
