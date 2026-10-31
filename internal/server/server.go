// Package server is Aesthetic Lab's HTTP handler for Cloud Run (cmd/web):
// the routes, the probes and the middleware around them.
package server

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// Config holds what the site needs to serve requests.
type Config struct {
	// Static holds the files served under /static/ (the embedded
	// static.Files in production); nil serves none.
	Static fs.FS
	// Logger receives the request log; nil is slog.Default().
	Logger *slog.Logger
	// Content is the content cache the pages render from, and that
	// /dependencyz checks; nil serves the embedded snapshot and fails
	// /dependencyz.
	Content *content.Store
	// PlainHTTP leaves upgrade-insecure-requests out of the
	// Content-Security-Policy, for a server reached without TLS (off Cloud
	// Run). Every other directive stays.
	PlainHTTP bool
}

// New returns the site's HTTP handler, wrapped in Middleware.
func New(cfg Config) http.Handler {
	mux := http.NewServeMux()
	// /healthz answers as soon as the process listens: Cloud Run's startup
	// probe uses it, so it never waits on anything outside the process.
	mux.HandleFunc("GET /healthz", probe)
	mux.HandleFunc("GET /readyz", probe)
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	store := cfg.Content
	if store == nil {
		store = content.NewStore(content.Options{Logger: log})
	}
	// /dependencyz is for deploy smoke tests: one bounded Firestore read.
	// Phase 9a removes the uptime check that calls it today, so that it
	// never keeps an instance warm.
	mux.HandleFunc("GET /dependencyz", dependency(store, log))
	mux.HandleFunc("GET /static/", staticFiles(cfg.Static))
	mux.HandleFunc("GET /og/", shareImages(cfg.Static))
	for _, name := range rootFiles {
		mux.HandleFunc("GET /"+name, rootFile(cfg.Static, name))
	}
	mux.HandleFunc("GET /{$}", root)
	// Each page under each locale, spelled out: a locale-shaped segment that
	// is not a locale (/de-BE/pricelist/) falls through to the 404.
	// The slashless form gets pages' 301, as on the Qwik site (the mux's own
	// redirect would be a 307).
	for _, l := range i18n.Supported() {
		mux.HandleFunc("GET /"+string(l)+"/{$}", home(l, store, log))
		mux.HandleFunc("GET /"+string(l), pages)
		mux.HandleFunc("GET /"+string(l)+"/pricelist/{$}", page(l, store, log, pricelist))
		mux.HandleFunc("GET /"+string(l)+"/pricelist", pages)
		mux.HandleFunc("GET /"+string(l)+"/privacy-policy/{$}", page(l, store, log, privacy))
		mux.HandleFunc("GET /"+string(l)+"/privacy-policy", pages)
		mux.HandleFunc("GET /"+string(l)+"/notice/{$}", page(l, store, log, care))
		mux.HandleFunc("GET /"+string(l)+"/notice", pages)
	}
	mux.HandleFunc("/", pages)
	policy := contentSecurityPolicy
	if cfg.PlainHTTP {
		policy = localContentSecurityPolicy
	}
	return middleware(log, policy, withLocale(mux))
}

// probe answers a health probe, never from a cache.
func probe(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, "OK")
}

// dependency answers OK when the content database answers one read, else
// 503 (with the reason only in the log).
func dependency(store *content.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Ping(r.Context()); err != nil {
			log.WarnContext(r.Context(), "dependency_check_failed", "error", err.Error())
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "Unavailable")
			return
		}
		probe(w, r)
	}
}
