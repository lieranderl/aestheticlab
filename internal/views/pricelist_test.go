package views_test

import (
	"bytes"
	"context"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
	"github.com/lieranderl/aestheticlab/internal/views"
)

// renderPricelist renders the pricelist at rawURL from the embedded snapshot.
func renderPricelist(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	l := i18n.FromPath(u.Path)
	ctx := views.WithRequest(i18n.WithLocale(context.Background(), l), u)
	var buf bytes.Buffer
	if err := views.Pricelist(content.Snapshot().In(l)).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func count(html, pattern string) int { return len(regexp.MustCompile(pattern).FindAllString(html, -1)) }

func TestEveryCategoryIsInTheHTMLWithOnlyTheSelectedOneShown(t *testing.T) {
	html := renderPricelist(t, "/en-BE/pricelist/")
	tabs := count(html, `href="\?category=[^"]+"`)
	if tabs < 2 {
		t.Fatalf("%d category links, want several", tabs)
	}
	if got := count(html, `<section[^>]*aria-labelledby="pricelist-`); got != tabs {
		t.Errorf("%d category sections, want one per tab (%d)", got, tabs)
	}
	// The first category is selected; the others ship hidden.
	if got := count(html, `<section hidden[^>]*aria-labelledby="pricelist-`); got != tabs-1 {
		t.Errorf("%d hidden sections, want %d", got, tabs-1)
	}
	if got := count(html, `aria-current="true"[^>]*>\s*<span[^>]*>01</span>`); got != 1 {
		t.Errorf("the first tab is not the current one")
	}
	if !strings.Contains(html, `aria-controls="price-row-`) || !strings.Contains(html, "Book this treatment") {
		t.Error("treatments lack their rows or Book buttons")
	}
	if !strings.Contains(html, "Add-ons · with any treatment") {
		t.Error("add-ons are not listed apart")
	}
}

func TestTheCategoryInTheURLIsSelectedWithoutJavaScript(t *testing.T) {
	html := renderPricelist(t, "/en-BE/pricelist/?category=laser")
	if !regexp.MustCompile(`<section data-attr:hidden="[^"]*" aria-labelledby="pricelist-laser-title"`).MatchString(html) {
		t.Error("the laser section is hidden, want it shown")
	}
	if !regexp.MustCompile(`href="\?category=laser"\s+aria-current="true"`).MatchString(html) {
		t.Error("the laser tab is not current")
	}
	// Laser areas are lists inside the laser category.
	for _, area := range []string{"Face", "Body", "Sets", "Men"} {
		if !strings.Contains(html, ">"+area+"\n") && !strings.Contains(html, ">"+area+"<") {
			t.Errorf("laser area %q is missing", area)
		}
	}
}

func TestThePricelistHeadNamesLeuvenAndItsLocales(t *testing.T) {
	html := renderPricelist(t, "/fr-BE/pricelist/?campaign=ignored")
	for _, want := range []string{
		`<html lang="fr-BE">`,
		`<link rel="canonical" href="https://aestheticlab.be/fr-BE/pricelist/">`,
		`<link rel="alternate" hreflang="en-BE" href="https://aestheticlab.be/en-BE/pricelist/">`,
		`<link rel="alternate" hreflang="x-default" href="https://aestheticlab.be/en-BE/pricelist/">`,
		`<meta property="og:image" content="https://aestheticlab.be/og/fr-BE.jpg">`,
		`<meta property="og:locale" content="fr_BE">`,
		`"@type":"BeautySalon"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("head lacks %s", want)
		}
	}
	if got := count(html, `<link rel="alternate"`); got != 6 {
		t.Errorf("%d alternate links, want 6", got)
	}
	if got := count(html, `og:locale:alternate`); got != 4 {
		t.Errorf("%d og:locale:alternate, want 4", got)
	}
	if !regexp.MustCompile(`<title>[^<]*à Louvain \| Aesthetic Lab</title>`).MatchString(html) {
		t.Error("the French title does not name Louvain")
	}
	// Language links keep the query.
	if !strings.Contains(html, `href="/nl-BE/pricelist/?campaign=ignored"`) {
		t.Error("language links lose the page or its query")
	}
}

// CSP allows no inline script and no style attributes.
func TestPagesCarryNoInlineScriptOrStyle(t *testing.T) {
	html := renderPricelist(t, "/en-BE/pricelist/")
	if regexp.MustCompile(`\sstyle="`).MatchString(html) {
		t.Error("a style attribute would be blocked by the CSP")
	}
	for _, m := range regexp.MustCompile(`<script[^>]*>`).FindAllString(html, -1) {
		if !strings.Contains(m, "src=") && !strings.Contains(m, `type="application/ld+json"`) {
			t.Errorf("inline script %s would be blocked by the CSP", m)
		}
	}
}
