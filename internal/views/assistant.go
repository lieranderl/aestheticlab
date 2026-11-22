package views

import (
	"bytes"
	"context"
	"html"
	"regexp"
	"strings"

	"github.com/lieranderl/aestheticlab/internal/content"
)

// QA is a question with its answer, both plain text.
type QA struct {
	Question, Answer string
}

var tag = regexp.MustCompile(`<[^>]*>`)

// plain is translated HTML as plain text.
func plain(s string) string {
	return strings.TrimSpace(html.UnescapeString(tag.ReplaceAllString(s, "")))
}

// AssistantFAQ is "Good to know" as plain text in the page's language, so
// the chat assistant answers exactly what the page says.
func AssistantFAQ(ctx context.Context, loc *content.Localized) []QA {
	out := []QA{{tr(ctx, "How do I book an appointment?"), tr(ctx, "Tap any Book button to see live availability, then choose your treatment and your preferred artist.")}}
	for _, item := range faqItems(ctx, loc) {
		out = append(out, QA{plain(item.Question), plain(item.Answer)})
	}
	return out
}

// AssistantFacts are the studio's fixed facts in the page's language, as
// the header, footer and contact section show them.
func AssistantFacts(ctx context.Context, loc *content.Localized) []string {
	facts := []string{
		siteName + ", " + street + ", " + zip + " " + city + ", Belgium",
		tr(ctx, "Monday") + " – " + tr(ctx, "Saturday") + " · " + opens + " – " + closes + " · " + tr(ctx, "By appointment only"),
		"Email: " + siteEmail,
		"Instagram: @aestheticlabbe (" + instagram + ")",
		"Google: " + ratingLabel(ctx),
	}
	for _, p := range loc.Contact.Parking {
		facts = append(facts, tr(ctx, "Parking")+": "+p.Name)
	}
	return facts
}

var (
	// tabLabel is a care-notes tab's radio, whose aria-label names it.
	tabLabel = regexp.MustCompile(`<input[^>]*aria-label="([^"]+)"[^>]*>`)
	// blockEnd ends a line of text.
	blockEnd = regexp.MustCompile(`(?i)</(p|li|h[1-6])>|<br\s*/?>`)
)

const careStart, careEnd = "<!-- assistant:care-notes -->", "<!-- /assistant:care-notes -->"

// AssistantCareNotes is the care notes page's text (preparation and
// aftercare) as plain text in the page's language, one line per paragraph,
// so the chat assistant answers what the page says. Empty if the page
// cannot be read.
func AssistantCareNotes(ctx context.Context) string {
	var buf bytes.Buffer
	if err := Care().Render(ctx, &buf); err != nil {
		return ""
	}
	page := buf.String()
	start, end := strings.Index(page, careStart), strings.Index(page, careEnd)
	if start < 0 || end < start {
		return ""
	}
	section := tabLabel.ReplaceAllString(page[start+len(careStart):end], "\n[$1]\n")
	section = blockEnd.ReplaceAllString(section, "\n")
	var lines []string
	for line := range strings.SplitSeq(plain(section), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
