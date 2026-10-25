package server

import (
	"io"
	"net/http"
	"path"
	"strings"

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

// home sends / to the default locale's home page, as the Qwik site does
// (302: the default could change).
func home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/"+string(i18n.Default)+"/", http.StatusFound)
}

// pages answers every path no route serves. A page path without its
// trailing slash is redirected to it (301, query kept), as Qwik City does;
// anything else is a 404 in the path's locale.
func pages(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && wantsSlash(p) {
		target := p + "/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		// #nosec G710 -- target is this request's own path plus "/": wantsSlash
		// refuses the //host and /\host paths a browser would read as another
		// site (TestTheSlashRedirectNeverLeavesTheSite).
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}
	notFound(w, r)
}

// wantsSlash reports whether p names a page that lacks its trailing slash:
// not a file (/favicon.svg), and not a path a browser would read as another
// site's (//evil.example, /\evil.example).
func wantsSlash(p string) bool {
	if p == "" || strings.HasSuffix(p, "/") || strings.HasPrefix(p, "//") || strings.Contains(p, `\`) {
		return false
	}
	return !strings.Contains(path.Base(p), ".")
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
