package views

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// Prices read as the Qwik site's server renders them (aestheticlab.be,
// 2026-10-25), no-break spaces included.
func TestPricesAreFormattedAsOnTheQwikSite(t *testing.T) {
	const nbsp, narrow = " ", " "
	for _, tc := range []struct {
		locale i18n.Locale
		amount float64
		want   string
	}{
		{i18n.EnBE, 35, "€35"},
		{i18n.EnBE, 37.5, "€37,50"},
		{i18n.EnBE, 1250, "€1.250"},
		{i18n.NlBE, 35, "€" + nbsp + "35"},
		{i18n.FrBE, 35, "35" + nbsp + "€"},
		{i18n.FrBE, 1250, "1" + narrow + "250" + nbsp + "€"},
		{i18n.RuBE, 37.5, "37,50" + nbsp + "€"},
		{i18n.UkBE, 1250, "1" + nbsp + "250" + nbsp + "EUR"},
	} {
		if got := price(i18n.WithLocale(context.Background(), tc.locale), tc.amount); got != tc.want {
			t.Errorf("price(%s, %v) = %q, want %q", tc.locale, tc.amount, got, tc.want)
		}
	}
}

func TestCategoryImagesFollowTheEnglishName(t *testing.T) {
	for name, want := range map[string]string{
		"Manicure":                "services/manicure.jpg",
		"Brows & lashes":          "services/brows.webp",
		"Laser hair removal FACE": "services/laser-face.webp",
		"laser":                   "services/laser.jpg",
		"Face Waxing":             "services/face-waxing.jpg",
		"Something new":           "gallery/universal.jpg",
	} {
		if got := coverImage(name); got != want {
			t.Errorf("coverImage(%q) = %q, want %q", name, got, want)
		}
	}
}

// Each locale's slogan length has its own literal size class.
func TestSloganSizesCoverEveryLocale(t *testing.T) {
	for _, l := range i18n.Supported() {
		slogan := tr(i18n.WithLocale(context.Background(), l), "The art of natural beauty")
		want := "(" + strconv.Itoa(utf8.RuneCountInString(slogan)) + "*0.46)"
		if got := sloganSize(slogan); !strings.Contains(got, want) {
			t.Errorf("%s: sloganSize(%q) = %s, want a case for %d characters", l, slogan, got, utf8.RuneCountInString(slogan))
		}
	}
}
