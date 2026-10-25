package views

import (
	"context"
	"regexp"
)

// policySection is a numbered section of the privacy policy.
type policySection struct {
	Title, Text string
	List        []string
}

// leadingNumber is the "1. " some translated titles start with: the page
// numbers its sections itself.
var leadingNumber = regexp.MustCompile(`^\d+\.\s*`)

// privacySections are the policy's sections, "Who we are" first.
func privacySections(ctx context.Context) []policySection {
	sections := []policySection{
		{Title: tr(ctx, "Who we are"), Text: trf(ctx, "Aesthetic Lab, %[1]v. For anything about your data, write to %[2]v.", street+", "+zip+" "+city, siteEmail)},
		{
			Title: tr(ctx, "1. Information We Collect"),
			Text:  tr(ctx, "We collect the following personal information when you book an appointment or contact us:"),
			List:  []string{tr(ctx, "Name"), tr(ctx, "Email address"), tr(ctx, "Phone number")},
		},
		{
			Title: tr(ctx, "2. How We Use Your Information"),
			Text:  tr(ctx, "We use your information to:"),
			List: []string{
				tr(ctx, "Schedule and confirm appointments"), tr(ctx, "Send appointment reminders and updates"),
				tr(ctx, "Respond to your inquiries"), tr(ctx, "Improve our services"),
			},
		},
		{
			Title: tr(ctx, "3. Data Protection & Security"),
			Text:  tr(ctx, "We take reasonable measures to protect your personal data from unauthorized access, loss, or misuse."),
		},
		{
			Title: tr(ctx, "4. Sharing Your Information"),
			Text:  tr(ctx, "We do not sell or rent your personal information. We may share it with:"),
			List:  []string{tr(ctx, "Service providers (e.g., booking platforms)"), tr(ctx, "Legal authorities if required by law")},
		},
		{
			Title: tr(ctx, "5. Your Rights"),
			Text:  tr(ctx, "You can request to access, update, or delete your personal data. To make a request, contact us at:"),
		},
		{
			Title: tr(ctx, "6. Cookies & Tracking Technologies"),
			Text:  tr(ctx, "We use strictly necessary cookies to operate the website. Google Analytics runs in Consent Mode v2. Before consent or if you reject analytics, analytics storage and advertising-related consent stay denied; Google may receive cookieless consent and measurement pings, and analytics cookies are not set. If you accept analytics, Google Analytics may use analytics cookies to understand website usage and improve our services. You can change your choice at any time via Cookie settings."),
			List: []string{
				tr(ctx, "Strictly necessary cookies: always active for basic website functionality."),
				tr(ctx, "Analytics cookies: optional; analytics storage is granted only after your consent."),
			},
		},
		{
			Title: tr(ctx, "7. Changes to This Policy"),
			Text:  tr(ctx, "We may update this Privacy Policy from time to time. The latest version will always be available on our website."),
		},
	}
	for i := range sections {
		sections[i].Title = leadingNumber.ReplaceAllString(sections[i].Title, "")
	}
	return sections
}

// popularLink is a page the 404 suggests.
type popularLink struct {
	Target, Label, Note string
}

func popularPages(ctx context.Context) []popularLink {
	return []popularLink{
		{"#services", tr(ctx, "Services"), tr(ctx, "Nails, brows, lashes and laser")},
		{"pricelist/", tr(ctx, "Prices"), tr(ctx, "Every treatment, time and price")},
		{"#contact", tr(ctx, "Visit"), tr(ctx, "Diestsestraat 174, Leuven")},
	}
}
