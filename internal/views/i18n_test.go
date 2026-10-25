package views

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// translated matches the texts templates and handlers translate:
// tr(ctx, "…"), trf(ctx, "…", …), trJS(ctx, "…"), i18n.T(ctx, "…") and
// trc(ctx, "context", "…").
var translated = regexp.MustCompile(`\b(?:tr(?:f|JS)?|i18n\.Tf?)\(ctx, "((?:[^"\\]|\\.)*)"|\btrc\(ctx, "((?:[^"\\]|\\.)*)", "((?:[^"\\]|\\.)*)"`)

// Every text a page translates exists in every locale, so no page quietly
// shows English to a Dutch, French, Russian or Ukrainian visitor.
func TestEveryInterfaceTextHasATranslationInEveryLocale(t *testing.T) {
	var files []string
	for _, pattern := range []string{"*.templ", "*.go", "../server/*.go"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	seen := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_templ.go") || strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file) // #nosec G304 -- this repository's own sources
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range translated.FindAllStringSubmatch(string(src), -1) {
			raw := m[1]
			if m[3] != "" {
				raw = m[2] + `\x04` + m[3]
			}
			msg, err := strconv.Unquote(`"` + raw + `"`)
			if err != nil {
				t.Errorf("%s: cannot read %q: %v", file, raw, err)
				continue
			}
			seen++
			for _, l := range i18n.Supported() {
				if !i18n.Has(l, msg) {
					t.Errorf("%s: no %s text for %q", file, l, msg)
				}
			}
		}
	}
	// The 404 page's two texts today; raise this as Phase 6 adds pages, so a
	// pattern that stops matching the calls cannot pass silently.
	if seen < 2 {
		t.Errorf("found %d translated texts; the pattern no longer matches the calls", seen)
	}
}

func in(l i18n.Locale) context.Context {
	return i18n.WithLocale(context.Background(), l)
}

func TestTemplateHelpersTranslateIntoThePagesLocale(t *testing.T) {
	ctx := in(i18n.UkBE)
	if got := tr(ctx, "Open navigation menu"); got != "Відкрити меню" {
		t.Errorf("tr = %q", got)
	}
	if got := trc(ctx, "navigation landmark", "Open navigation menu"); got != "Мобільна навігація" {
		t.Errorf("trc = %q", got)
	}
	if got := trf(in(i18n.EnBE), "Book with %v", "Lera"); got != "Book with Lera" {
		t.Errorf("trf = %q", got)
	}
	if got := pageLang(ctx); got != "uk-BE" {
		t.Errorf("pageLang = %q", got)
	}
}

// Texts go into Datastar expressions as single-quoted JavaScript strings, so
// an apostrophe in a translation ("Retour à l'aperçu") cannot end the string.
func TestTextsQuotedForDatastarCannotBreakOutOfTheirString(t *testing.T) {
	if got := trc(in(i18n.FrBE), "treatment overview", "Back to categories"); got != "Retour à l'aperçu" {
		t.Fatalf("French text = %q", got)
	}
	if got := jsString("Retour à l'aperçu\n"); got != `'Retour à l\'aperçu\n'` {
		t.Errorf("jsString = %s", got)
	}
	if got := trJS(in(i18n.RuBE), "Open navigation menu"); got != "'Открыть меню'" {
		t.Errorf("trJS = %s", got)
	}
}
