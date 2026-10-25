// Package server is Aesthetic Lab's HTTP handler for Cloud Run (cmd/web):
// the routes, the probes and the middleware around them.
package server

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
)

// Config holds what the site needs to serve requests.
type Config struct {
	// Static holds the files served under /static/ (the embedded
	// static.Files in production); nil serves none.
	Static fs.FS
	// Logger receives the request log; nil is slog.Default().
	Logger *slog.Logger
}

// New returns the site's HTTP handler, wrapped in Middleware.
func New(cfg Config) http.Handler {
	mux := http.NewServeMux()
	// /healthz answers as soon as the process listens: Cloud Run's startup
	// probe uses it, so it never waits on anything outside the process.
	mux.HandleFunc("GET /healthz", probe)
	mux.HandleFunc("GET /readyz", probe)
	// TODO(Phase 5): one bounded Firestore read.
	mux.HandleFunc("GET /dependencyz", probe)
	mux.HandleFunc("GET /static/", staticFiles(cfg.Static))
	mux.HandleFunc("GET /{$}", home)
	mux.HandleFunc("/", pages)
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return Middleware(log, withLocale(mux))
}

// probe answers a health probe, never from a cache.
func probe(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, "OK")
}
