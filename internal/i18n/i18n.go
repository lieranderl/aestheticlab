// Package i18n translates the site into its five locales. English text is
// the key: a page writes T(ctx, "Book appointment") and gets the text in the
// locale the request carries. The texts come from the Qwik site's locale
// files (messages_gen.go, written by cmd/i18n-convert).
package i18n

import (
	"context"
	"fmt"
	"strings"
)

// Locale is one of the site's locales, by its URL code ("nl-BE").
type Locale string

// The site's locales.
const (
	EnBE Locale = "en-BE"
	RuBE Locale = "ru-BE"
	NlBE Locale = "nl-BE"
	FrBE Locale = "fr-BE"
	UkBE Locale = "uk-BE"
)

// Default is the locale of / and of paths no locale names.
const Default = EnBE

// supported is the Qwik site's order (src/speak-config.ts).
var supported = []Locale{EnBE, RuBE, NlBE, FrBE, UkBE}

// nativeNames name each language in itself, so "UK" cannot be read as
// English (src/shared/locale-navigation.ts).
var nativeNames = map[Locale]string{
	EnBE: "English",
	NlBE: "Nederlands",
	FrBE: "Français",
	RuBE: "Русский",
	UkBE: "Українська",
}

// Supported lists the site's locales, English first.
func Supported() []Locale {
	return append([]Locale(nil), supported...)
}

// Parse returns the locale whose code is s exactly ("nl-BE", not "nl-be").
func Parse(s string) (Locale, bool) {
	for _, l := range supported {
		if string(l) == s {
			return l, true
		}
	}
	return Default, false
}

// FromPath is the locale a URL path's first segment names, else Default:
// /fr-BE/pricelist/ is French, and so is /fr-BE/old-link/ (its 404 page).
func FromPath(path string) Locale {
	first, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	l, _ := Parse(first)
	return l
}

// Language is the locale's language code, for hreflang and the like ("nl").
func (l Locale) Language() string {
	lang, _, _ := strings.Cut(string(l), "-")
	return lang
}

// NativeName is the language's name in itself ("Nederlands").
func (l Locale) NativeName() string {
	return nativeNames[l]
}

type localeKey struct{}

// WithLocale returns ctx carrying l.
func WithLocale(ctx context.Context, l Locale) context.Context {
	return context.WithValue(ctx, localeKey{}, l)
}

// FromContext is the locale ctx carries: Default when none.
func FromContext(ctx context.Context) Locale {
	if l, ok := ctx.Value(localeKey{}).(Locale); ok {
		return l
	}
	return Default
}

// T is msg in ctx's locale, or msg when it has no translation.
func T(ctx context.Context, msg string) string {
	return lookup(FromContext(ctx), msg, msg)
}

// Tf formats args with the translation of format ("Book with %v").
func Tf(ctx context.Context, format string, args ...any) string {
	return fmt.Sprintf(T(ctx, format), args...)
}

// C is msg in ctx's locale where context tells it apart from the same
// English elsewhere: C(ctx, "treatment overview", "Back to categories").
func C(ctx context.Context, context, msg string) string {
	return lookup(FromContext(ctx), context+"\x04"+msg, msg)
}

func lookup(l Locale, key, english string) string {
	if s, ok := translations[l][key]; ok {
		return s
	}
	return english
}

// Has reports whether l has msg (a context's texts as "context\x04text");
// English has every text the site uses.
func Has(l Locale, msg string) bool {
	if l == EnBE {
		for _, m := range messages {
			if m == msg {
				return true
			}
		}
		return false
	}
	_, ok := translations[l][msg]
	return ok
}

// Messages lists every English text the site uses, a context's texts as
// "context\x04text".
func Messages() []string {
	return append([]string(nil), messages...)
}
