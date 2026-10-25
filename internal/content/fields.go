package content

import (
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// readString is doc[key] trimmed, or "" when it is not text.
func readString(doc Document, key string) string {
	s, _ := doc[key].(string)
	return strings.TrimSpace(s)
}

// readNumber is doc[key] when it is a finite number.
func readNumber(doc Document, key string) (float64, bool) {
	var n float64
	switch v := doc[key].(type) {
	case int64:
		n = float64(v)
	case int:
		n = float64(v)
	case float64:
		n = v
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	return n, !math.IsInf(n, 0) && !math.IsNaN(n)
}

// readID is doc[key] as text: a non-empty string, or a number written as
// JavaScript's String() writes it (635110, not 635110.0).
func readID(doc Document, key string) string {
	if s := readString(doc, key); s != "" {
		return s
	}
	if n, ok := readNumber(doc, key); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return ""
}

// readLink is doc[key] when it is an absolute http(s) URL, so content can
// never put a javascript: or data: link on the page.
func readLink(doc Document, key string) string {
	s := readString(doc, key)
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return s
}

// bundledPhoto is a staff photo bundled with the site ("julia",
// "julia.jpg"), which the pages resolve to its image variants.
var bundledPhoto = regexp.MustCompile(`(?i)^[a-z0-9_-]+(\.(jpe?g|png|webp|avif))?$`)

// readPhoto is doc[key] when it names a bundled photo or is an http(s)
// URL; anything else (a path, a data: URL) is dropped.
func readPhoto(doc Document, key string) string {
	if s := readString(doc, key); bundledPhoto.MatchString(s) {
		return s
	}
	return readLink(doc, key)
}
