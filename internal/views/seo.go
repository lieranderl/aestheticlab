package views

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// ogLocale is l as Open Graph writes it: nl_BE.
func ogLocale(l i18n.Locale) string { return strings.Replace(string(l), "-", "_", 1) }

// jsonLD is the studio as schema.org BeautySalon,
// the same on every page.
var jsonLD = func() string {
	address := street + ", " + zip + " " + city + ", BE"
	data, err := json.Marshal(map[string]any{
		"@context": "https://schema.org",
		"@type":    "BeautySalon",
		"@id":      siteURL + "#salon",
		"name":     siteName,
		"image":    "https://lh3.googleusercontent.com/p/AF1QipOpUazRD7FPPGNWPbIYA8Fyf7vn0fUdm2bBkQDx",
		"url":      siteURL,
		"email":    siteEmail,
		"address": map[string]any{
			"@type": "PostalAddress", "streetAddress": street, "addressLocality": city,
			"postalCode": zip, "addressCountry": "BE",
		},
		"geo":    map[string]any{"@type": "GeoCoordinates", "latitude": 50.88148, "longitude": 4.71053},
		"hasMap": "https://maps.google.com/?q=" + strings.ReplaceAll(url.QueryEscape(address), "+", "%20"),
		"openingHoursSpecification": []any{map[string]any{
			"@type":     "OpeningHoursSpecification",
			"dayOfWeek": []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
			"opens":     opens, "closes": closes,
		}},
		"priceRange": "€€",
		"sameAs":     []string{instagram},
	})
	if err != nil {
		panic(err)
	}
	// json.Marshal escapes <, > and &, so the data cannot close the script.
	return string(data)
}()

// jsonLDScript is the structured-data block. It is data, not script, so the
// CSP's script-src does not apply to it.
func jsonLDScript() string { return `<script type="application/ld+json">` + jsonLD + `</script>` }
