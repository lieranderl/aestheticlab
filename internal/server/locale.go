package server

import (
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// withLocale puts the locale the path's first segment names (else English)
// in each request's context, for i18n.T and the pages.
func withLocale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := i18n.WithLocale(r.Context(), i18n.FromPath(r.URL.Path))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// root sends / to the default locale's home page, as the Qwik site does
// (302: the default could change).
func root(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/"+string(i18n.Default)+"/", http.StatusFound)
}

// pages answers every path no route serves. A page path without its
// trailing slash is redirected to it (301, query kept), as Qwik City does;
// anything else is a 404 in the path's locale.
func pages(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if target, ok := slashed(r.URL); ok {
			// #nosec G710 -- target is this request's own escaped path plus
			// "/", and slashed refuses anything a browser could read as
			// another site (TestTheSlashRedirectNeverLeavesTheSite).
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
	}
	notFound(w, r)
}

// slashed is u's page URL with its trailing slash, if u names a page that
// lacks one: not a file (/favicon.svg), and not a path a browser could read
// as another site's (//evil.example, /\evil.example, /%09/evil.example).
//
// The path is used escaped, as the request sent it, so %3F, %23 and %2F
// stay part of the path rather than becoming a query, a fragment or a
// separator, and control characters stay encoded instead of reaching the
// browser, which would drop them.
func slashed(u *url.URL) (string, bool) {
	p := u.Path
	if p == "" || strings.HasSuffix(p, "/") || strings.Contains(path.Base(p), ".") {
		return "", false
	}
	if strings.HasPrefix(p, "//") || strings.ContainsAny(p, `\`) || strings.ContainsFunc(p, unicode.IsControl) {
		return "", false
	}
	// The Location itself: a path on this site, never //host or /\host.
	escaped := u.EscapedPath()
	if len(escaped) < 2 || escaped[0] != '/' || escaped[1] == '/' || escaped[1] == '\\' {
		return "", false
	}
	target := escaped + "/"
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return target, true
}

// notFound is the 404 answer, in the locale the path names. Phase 6 of
// plans/go-datastar-scale-to-zero.md replaces the text with the branded page.
func notFound(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Language", string(i18n.FromContext(ctx)))
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, i18n.T(ctx, "This page has moved on")+"\n"+
		i18n.T(ctx, "The link may be old. Everything you need is one tap away.")+"\n")
}
