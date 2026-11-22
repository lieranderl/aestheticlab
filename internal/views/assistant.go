package views

import (
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
