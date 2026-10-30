// Command i18n-convert writes internal/i18n/messages_gen.go from the site's
// locale files (internal/i18n/locales/<locale>.json): every text in the
// English file, keyed by its English, in every other locale.
//
// The locale files are the source of the site's texts: change them, then run
// make i18n (make i18n-check fails when the Go tables are stale). A new text
// goes in all five files under a dotted key; the key only names it.
//
// Usage: go run ./cmd/i18n-convert
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	outFile    = "internal/i18n/messages_gen.go"
	localesDir = "internal/i18n/locales"
)

// locales are the site's locales and the Go constants naming them.
var locales = []struct{ code, constant string }{
	{"en-BE", "EnBE"},
	{"nl-BE", "NlBE"},
	{"fr-BE", "FrBE"},
	{"ru-BE", "RuBE"},
	{"uk-BE", "UkBE"},
}

// contexts tell apart texts whose English is shared by keys translated
// differently: the key gets the text as "context\x04English" (i18n.C).
var contexts = map[string]string{
	// "Open navigation menu" is also the menu button's label (app.nav.open_menu).
	"app.nav.mobile_menu": "navigation landmark",
	// "Back to categories" is also the price list's link (app.pricelist.back_to_categories).
	"app.services.back": "treatment overview",
}

var placeholder = regexp.MustCompile(`\{\{(\w+)\}\}`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "i18n-convert:", err)
		os.Exit(1)
	}
}

func run() error {
	tables := map[string]map[string]string{}
	for _, l := range locales {
		t, err := readLocale(filepath.Join(localesDir, l.code+".json"))
		if err != nil {
			return err
		}
		tables[l.code] = t
	}
	used := slices.Sorted(maps.Keys(tables["en-BE"]))
	src, texts, err := render(used, tables)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(outFile)
	if err == nil && bytes.Equal(current, src) {
		return nil
	}
	if err := os.WriteFile(outFile, src, 0o600); err != nil {
		return err
	}
	fmt.Printf("i18n: wrote %d keys as %d texts in %d locales to %s\n", len(used), texts, len(locales), outFile)
	return nil
}

// readLocale flattens a locale file to dotted keys: "app.nav.services".
func readLocale(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- this repository's locale files
	if err != nil {
		return nil, err
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := map[string]string{}
	var walk func(prefix string, node map[string]any)
	walk = func(prefix string, node map[string]any) {
		for k, v := range node {
			switch v := v.(type) {
			case map[string]any:
				walk(prefix+k+".", v)
			case string:
				out[prefix+k] = v
			}
		}
	}
	walk("", tree)
	return out, nil
}

// formatText turns {{name}} placeholders into fmt verbs, in the order the
// English text names them: %v for one occurrence, %[1]v, %[2]v… for multiple
// occurrences, including repeated arguments. A text with placeholders has
// its literal % doubled; one without is left as it is.
func formatText(text string, order []string) string {
	if len(order) == 0 {
		return text
	}
	text = strings.ReplaceAll(text, "%", "%%")
	single := len(order) == 1 && len(placeholder.FindAllStringIndex(text, -1)) == 1
	return placeholder.ReplaceAllStringFunc(text, func(m string) string {
		name := placeholder.FindStringSubmatch(m)[1]
		if single {
			return "%v"
		}
		return fmt.Sprintf("%%[%d]v", slices.Index(order, name)+1)
	})
}

// placeholders lists text's placeholder names in order, each once.
func placeholders(text string) []string {
	var names []string
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	return names
}

type entry struct {
	key, msg string // msg is the English, prefixed by its context if it has one
}

// render builds the Go source of messages_gen.go and counts its texts.
func render(used []string, tables map[string]map[string]string) ([]byte, int, error) {
	english := tables["en-BE"]
	var entries []entry
	translated := map[string]map[string]string{} // locale → msg → text
	owner := map[string]string{}                 // msg → first key with it
	var problems []string
	for _, key := range used {
		en, ok := english[key]
		if !ok {
			problems = append(problems, key+": not in "+localesDir+"/en-BE.json")
			continue
		}
		order := placeholders(en)
		msg := formatText(en, order)
		if c, ok := contexts[key]; ok {
			msg = c + "\x04" + msg
		}
		first, dup := owner[msg]
		if !dup {
			owner[msg] = key
			entries = append(entries, entry{key, msg})
		}
		for _, l := range locales[1:] {
			text, ok := tables[l.code][key]
			if !ok || text == "" {
				problems = append(problems, fmt.Sprintf("%s: no %s translation", key, l.code))
				continue
			}
			if got := placeholders(text); !sameSet(got, order) {
				problems = append(problems, fmt.Sprintf("%s: %s has placeholders %v, English %v", key, l.code, got, order))
				continue
			}
			text = formatText(text, order)
			if translated[l.code] == nil {
				translated[l.code] = map[string]string{}
			}
			if prev, ok := translated[l.code][msg]; ok && prev != text {
				problems = append(problems, fmt.Sprintf("%s and %s share the English %q but differ in %s: add one to contexts", first, key, en, l.code))
				continue
			}
			translated[l.code][msg] = text
		}
	}
	for _, l := range locales[1:] {
		for _, key := range slices.Sorted(maps.Keys(tables[l.code])) {
			if _, ok := english[key]; !ok {
				problems = append(problems, fmt.Sprintf("%s: %s has it, English doesn't", key, l.code))
			}
		}
	}
	if len(problems) > 0 {
		return nil, 0, errors.New(strings.Join(problems, "\n"))
	}

	var b strings.Builder
	b.WriteString(`// Code generated by cmd/i18n-convert from internal/i18n/locales/*.json; DO NOT EDIT.
// Change the locale files, then run make i18n.

package i18n

// messages lists the site's English texts, each with the key that names it
// in the locale files. A text told apart by a context is "context\x04text"
// (see C).
var messages = []string{
`)
	for _, e := range entries {
		fmt.Fprintf(&b, "\t%s, // %s\n", strconv.Quote(e.msg), e.key)
	}
	b.WriteString("}\n\n// translations are the texts in every locale but English, by English text.\nvar translations = map[Locale]map[string]string{\n")
	for _, l := range locales[1:] {
		fmt.Fprintf(&b, "\t%s: {\n", l.constant)
		for _, e := range entries {
			fmt.Fprintf(&b, "\t\t%s: %s,\n", strconv.Quote(e.msg), strconv.Quote(translated[l.code][e.msg]))
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
	src, err := format.Source([]byte(b.String()))
	return src, len(entries), err
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}
