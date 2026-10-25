package views

import (
	"context"
	"os"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// Every text a page translates exists in every locale, so no page quietly
// shows English to a Dutch, French, Russian or Ukrainian visitor.
func TestEveryInterfaceTextHasATranslationInEveryLocale(t *testing.T) {
	texts, err := interfaceTexts(os.DirFS(".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range texts {
		for _, l := range i18n.Supported() {
			if !i18n.Has(l, text.msg) {
				t.Errorf("%s: no %s text for %q", text.file, l, text.msg)
			}
		}
	}
	// The 404 page's two texts today; raise this as Phase 6 adds pages, so a
	// pattern that stops matching the calls cannot pass silently.
	if len(texts) < 2 {
		t.Errorf("found %d translated texts; the scanner no longer matches the calls", len(texts))
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
