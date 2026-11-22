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

// A patch morphs #services-body in place, and Datastar re-runs a data-init
// only when its value changes: each selection must reveal itself anew.
func TestEachSelectionRevealsItselfAfterAPatch(t *testing.T) {
	reveal := regexp.MustCompile(`<div id="services-body" data-init="([^"]*)"`)
	seen := map[string]string{}
	for _, q := range []url.Values{
		{}, {"treatment": {"635112"}}, {"treatment": {"635110"}},
		{"treatment": {"laser"}}, {"treatment": {"laser"}, "treatmentArea": {"674219"}},
	} {
		frag := renderAt(t, "/en-BE/", func(loc *content.Localized) templ.Component { return views.ServicesFragment(loc, q) })
		m := reveal.FindStringSubmatch(frag)
		if m == nil {
			t.Fatalf("%v: no reveal", q)
		}
		if other, dup := seen[m[1]]; dup {
			t.Errorf("%v reveals with the same data-init as %s", q, other)
		}
		seen[m[1]] = q.Encode()
	}
}

// Requests for different selections share one abort signal, so a slow
// earlier response never lands after the latest one.
func TestSectionRequestsCancelEachOther(t *testing.T) {
	html := renderAt(t, "/en-BE/?treatment=laser", home)
	gets, shared := strings.Count(html, "@get("), strings.Count(html, "{requestCancellation: aesthetic.sectionSignal()})")
	if gets < 3 || shared != gets {
		t.Errorf("%d section requests, %d sharing the abort signal; want all of several", gets, shared)
	}
}

func TestTheAssistantReadsTheCareNotesInThePageLanguage(t *testing.T) {
	en := views.AssistantCareNotes(i18n.WithLocale(context.Background(), i18n.EnBE))
	for _, want := range []string{"[Nails]", "[Laser]", "Complimentary Fix Policy", "Shave the area 24 hours before your session."} {
		if !strings.Contains(en, want) {
			t.Errorf("care notes lack %q:\n%.600s", want, en)
		}
	}
	if strings.ContainsAny(en, "<>") {
		t.Errorf("care notes still hold markup:\n%.600s", en)
	}
	nl := views.AssistantCareNotes(i18n.WithLocale(context.Background(), i18n.NlBE))
	if nl == "" || nl == en {
		t.Error("the Dutch care notes are missing or English")
	}
}

// The assistant describes the studio and each category as the site does.
func TestTheAssistantKnowsTheStudioAndEachCategory(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.EnBE)
	loc := &content.Localized{ServiceGroups: []content.ServiceGroup{
		{ID: "1", Name: "Laser hair removal FACE", NameEN: "Laser hair removal FACE"},
		{ID: "2", Name: "Pedicure", NameEN: "Pedicure"},
	}}
	facts := strings.Join(views.AssistantFacts(ctx, loc), "\n")
	if !strings.Contains(facts, "Opened in 2024 with a simple promise") {
		t.Errorf("facts lack the studio's story:\n%s", facts)
	}
	about := views.AssistantCategories(ctx, loc)
	if !strings.Contains(about["1"], "diode laser") || !strings.Contains(about["2"], "hygienic pedicure") {
		t.Errorf("categories = %q", about)
	}
	nl := views.AssistantCategories(i18n.WithLocale(context.Background(), i18n.NlBE), loc)
	if nl["2"] == "" || nl["2"] == about["2"] {
		t.Error("the Dutch category description is missing or English")
	}
}

// Good to know answers payment, and the cancellation answer matches the
// studio's policy: no changes within 24 hours.
func TestGoodToKnowCoversPaymentAndTheCancellationPolicy(t *testing.T) {
	html := renderAt(t, "/en-BE/", home)
	for _, want := range []string{"How can I pay?", "We don’t accept card payments.", "cancellations and changes are not possible"} {
		if !strings.Contains(html, want) {
			t.Errorf("Good to know lacks %q", want)
		}
	}
	if strings.Contains(html, "Late cancellations may be charged") {
		t.Error("Good to know still has the old cancellation answer")
	}
}

// The back-to-top arrow sits bottom right; where the chat assistant runs,
// it moves up above the chat button instead of covering it.
func TestTheBackToTopArrowMakesRoomForTheChat(t *testing.T) {
	render := func(ctx context.Context) string {
		var buf bytes.Buffer
		if err := home(content.Snapshot().In(i18n.EnBE)).Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	u, _ := url.Parse("/en-BE/")
	ctx := views.WithRequest(i18n.WithLocale(context.Background(), i18n.EnBE), u)
	if html := render(ctx); !strings.Contains(html, "lg:right-6 lg:bottom-6") || strings.Contains(html, "lg:bottom-23") {
		t.Error("without the chat, the arrow left its place at the bottom right")
	}
	if html := render(views.WithChat(ctx)); !strings.Contains(html, "lg:right-7 lg:bottom-23") {
		t.Error("with the chat, the arrow does not sit above the chat button")
	}
}

// While the assistant answers, "Thinking…" shimmers (daisyUI skeleton-text)
// and is read out once.
func TestTheChatShowsThinkingAsShimmeringText(t *testing.T) {
	var buf bytes.Buffer
	u, _ := url.Parse("/en-BE/")
	ctx := views.WithChat(views.WithRequest(i18n.WithLocale(context.Background(), i18n.EnBE), u))
	if err := home(content.Snapshot().In(i18n.EnBE)).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, `class="skeleton skeleton-text`) || strings.Count(html, "Thinking…") != 1 {
		t.Error("the chat's busy bubble is not a single shimmering \"Thinking…\"")
	}
}
