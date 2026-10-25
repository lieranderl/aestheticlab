package views

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// The studio's fixed facts (src/constants/metadata.ts, src/consts.ts).
const (
	siteName  = "Aesthetic Lab"
	siteURL   = "https://aestheticlab.be"
	siteEmail = "aestheticlabbe@gmail.com"
	street    = "Diestsestraat 174"
	city      = "Leuven"
	zip       = "3000"
	opens     = "10:00"
	closes    = "18:00"
	instagram = "https://www.instagram.com/aestheticlabbe"
	// googlePlaceURL opens the studio on Google Maps (reviews, directions).
	googlePlaceURL = "https://maps.app.goo.gl/bsdNssGY4YTJeR7j6"
	bookingURL     = "https://bookings.gettimely.com/aestheticlab2/bb/book"
	// bookingLocationID is Timely's location when the content names none.
	bookingLocationID = "372146"
)

type requestKey struct{}

// request is what a page knows of its request: the path after the locale
// ("/pricelist/") and the query.
type request struct {
	path  string
	query url.Values
}

// localePrefix is a locale-shaped first segment (/nl-BE, /de-DE), which
// language links replace, as the Qwik site's do.
var localePrefix = regexp.MustCompile(`^/[a-z]{2}-[A-Z]{2}(/|$)`)

// WithRequest keeps the request's path and query in ctx for the pages:
// language links, canonical URLs and the pricelist's selected category.
func WithRequest(ctx context.Context, u *url.URL) context.Context {
	p := localePrefix.ReplaceAllString(u.Path, "/")
	return context.WithValue(ctx, requestKey{}, request{path: p, query: u.Query()})
}

func requestOf(ctx context.Context) request {
	if r, ok := ctx.Value(requestKey{}).(request); ok {
		return r
	}
	return request{path: "/"}
}

// queryParam is the request's query parameter key.
func queryParam(ctx context.Context, key string) string { return requestOf(ctx).query.Get(key) }

// href is target in the page's locale: "#services" → "/en-BE/#services",
// "pricelist/" → "/en-BE/pricelist/".
func href(ctx context.Context, target string) string {
	return "/" + pageLang(ctx) + "/" + target
}

// localeLink is the current page in another locale.
type localeLink struct {
	Lang    string
	Label   string // "NL"
	Name    string // "Nederlands"
	Href    string
	Current bool
}

// localeLinks are the current page in every locale, query kept.
func localeLinks(ctx context.Context) []localeLink {
	r := requestOf(ctx)
	query := ""
	if q := r.query.Encode(); q != "" {
		query = "?" + q
	}
	current := i18n.FromContext(ctx)
	var out []localeLink
	for _, l := range i18n.Supported() {
		out = append(out, localeLink{
			Lang: string(l), Label: strings.ToUpper(l.Language()), Name: l.NativeName(),
			Href: "/" + string(l) + r.path + query, Current: l == current,
		})
	}
	return out
}

// absoluteURL is the page in locale l on the production site, with no
// query: canonical and alternate links.
func absoluteURL(l i18n.Locale, path string) string { return siteURL + "/" + string(l) + path }

// price is a euro amount as the Qwik site's server formats it with
// Intl.NumberFormat in each locale (aestheticlab.be, 2026-10-25): "€70"
// (en), "€ 70" (nl), "70 €" (fr, ru), "70 EUR" (uk), with two decimals
// only when the amount has cents and thousands grouped ("1.250",
// "1 250"). Spaces are no-break spaces, as Intl writes them.
func price(ctx context.Context, amount float64) string {
	const nbsp, narrowNBSP = " ", " "
	l := i18n.FromContext(ctx)
	cents := int64(amount*100 + 0.5)
	whole, frac := cents/100, cents%100
	group := "."
	switch l {
	case i18n.FrBE:
		group = narrowNBSP
	case i18n.RuBE, i18n.UkBE:
		group = nbsp
	}
	digits := strconv.FormatInt(whole, 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(d)
	}
	number := b.String()
	if frac != 0 {
		number += "," + strconv.FormatInt(frac/10, 10) + strconv.FormatInt(frac%10, 10)
	}
	switch l {
	case i18n.FrBE, i18n.RuBE:
		return number + nbsp + "€"
	case i18n.UkBE:
		return number + nbsp + "EUR"
	case i18n.NlBE:
		return "€" + nbsp + number
	}
	return "€" + number
}

// upperFirst is s with its first letter upper-cased: some translated
// category names start lower-case.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// twoDigits is n as "01", "02", … for the numbered lists.
func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// minutes is a duration as "60 min", or "" for none.
func minutes(ctx context.Context, d float64) string {
	if d <= 0 {
		return ""
	}
	return strconv.FormatFloat(d, 'f', -1, 64) + " " + tr(ctx, "min")
}
