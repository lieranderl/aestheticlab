package views_test

import (
	"bytes"
	"context"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
	"github.com/lieranderl/aestheticlab/internal/views"
)

// renderAt renders c as the page at rawURL would.
func renderAt(t *testing.T, rawURL string, c func(*content.Localized) templ.Component) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	l := i18n.FromPath(u.Path)
	ctx := views.WithRequest(i18n.WithLocale(context.Background(), l), u)
	var buf bytes.Buffer
	if err := c(content.Snapshot().In(l)).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func home(loc *content.Localized) templ.Component { return views.Home(loc) }

func TestTheHomePageHasEverySection(t *testing.T) {
	html := renderAt(t, "/en-BE/", home)
	for _, id := range []string{"hero", "services", "gallery", "team", "faq", "contact", "main-content", "gallery-lightbox"} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Errorf("no #%s", id)
		}
	}
	for _, want := range []string{
		"The art of natural beauty",
		`aria-controls="hero-video"`,
		"★★★★★ 5.0 · 60 Google reviews",
		"Diestsestraat 174,",
		"Good to know",
		`<link rel="canonical" href="https://aestheticlab.be/en-BE/">`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the home page lacks %q", want)
		}
	}
	if got := count(html, `aria-controls="work-clip-\d"`); got != 4 {
		t.Errorf("%d clip controls, want 4", got)
	}
	if got := count(html, `id="gallery-lightbox-trigger-\d"`); got != 8 {
		t.Errorf("%d gallery photos, want 8", got)
	}
	// Starting prices never come from an add-on.
	if !regexp.MustCompile(`From €\d+`).MatchString(html) || regexp.MustCompile(`From €5(\D)`).MatchString(html) {
		t.Error("starting prices are missing or come from add-ons")
	}
}

func TestTheFAQGivesTheTreatmentTimesFromThePriceList(t *testing.T) {
	html := renderAt(t, "/en-BE/", home)
	if !regexp.MustCompile(`From \d+ to \d+ minutes, depending on the treatment\.`).MatchString(html) {
		t.Error("the duration answer lacks the range")
	}
	if !strings.Contains(html, `<a href='./pricelist'>price list</a>`) {
		t.Error("the answers' links are escaped, want the translated HTML")
	}
}

func TestTheTreatmentInTheURLIsShownOnLoad(t *testing.T) {
	overview := renderAt(t, "/en-BE/", home)
	if strings.Contains(overview, `id="service-details-heading"`) {
		t.Error("the overview shows a category")
	}
	for _, rawURL := range []string{"/en-BE/?treatment=unknown&treatmentArea=unknown", "/en-BE/?treatmentArea=x"} {
		if html := renderAt(t, rawURL, home); strings.Contains(html, `id="service-details-heading"`) {
			t.Errorf("%s: an unknown treatment shows a category, want the overview", rawURL)
		}
	}
	laser := renderAt(t, "/en-BE/?treatment=laser", home)
	if got := count(laser, `aria-describedby="laser-[^"]+-description"`); got != 4 {
		t.Errorf("the laser category shows %d areas, want 4", got)
	}
	if !strings.Contains(laser, "Back to categories") || strings.Contains(laser, "Back to laser category") {
		t.Error("the laser category's back buttons are wrong")
	}
	html := renderAt(t, "/en-BE/?treatment=635112", home)
	if !regexp.MustCompile(`id="service-details-heading"[^>]*>Manicure<`).MatchString(html) {
		t.Error("the manicure category is not shown")
	}
	if got := count(html, `>\s*Book now\s*<`); got == 0 {
		t.Error("the treatments have no Book now buttons")
	}
}

func TestAServicesFragmentRevealsItself(t *testing.T) {
	q := url.Values{"treatment": {"635112"}}
	frag := renderAt(t, "/en-BE/?treatment=635112", func(loc *content.Localized) templ.Component { return views.ServicesFragment(loc, q) })
	if !strings.HasPrefix(strings.TrimSpace(frag), `<div id="services-body" data-init=`) {
		t.Errorf("the fragment is not the services body with its reveal: %.120s", frag)
	}
	if !strings.Contains(frag, "service-details-heading") || strings.Contains(frag, "<html") {
		t.Error("the fragment is not just the category")
	}
}

func TestEverySloganHasItsSize(t *testing.T) {
	for _, l := range i18n.Supported() {
		html := renderAt(t, "/"+string(l)+"/", home)
		if !regexp.MustCompile(`<h1 class="[^"]*text-\[clamp\(1\.25rem,calc\(\(100vw-3rem\)/\(\d+\*0\.46\)\),2\.75rem\)\]`).MatchString(html) {
			t.Errorf("%s: the hero headline has no phone size", l)
		}
	}
}
