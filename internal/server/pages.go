package server

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
	"github.com/lieranderl/aestheticlab/internal/views"
)

// page serves a page in locale l, built from the content.
func page(l i18n.Locale, store *content.Store, log *slog.Logger, render func(*content.Localized) templ.Component) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := views.WithRequest(r.Context(), r.URL)
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Language", string(l))
		// As the Qwik site's layout: a short window, so stale HTML never
		// outlives the assets it references for long.
		h.Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
		if err := render(store.Content(ctx).In(l)).Render(ctx, w); err != nil {
			log.ErrorContext(ctx, "page_render_failed", "path", r.URL.Path, "error", err.Error())
		}
	}
}

func pricelist(loc *content.Localized) templ.Component { return views.Pricelist(loc) }

func homePage(loc *content.Localized) templ.Component { return views.Home(loc) }

func privacy(*content.Localized) templ.Component { return views.Privacy() }

func care(*content.Localized) templ.Component { return views.Care() }

// home serves the landing page in locale l. A Datastar request (a tap in
// the services section, or the back button) gets only that section for its
// URL, patched in place.
func home(l i18n.Locale, store *content.Store, log *slog.Logger) http.HandlerFunc {
	full := page(l, store, log, homePage)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Datastar-Request") != "true" {
			full(w, r)
			return
		}
		ctx := views.WithRequest(r.Context(), r.URL)
		loc := store.Content(ctx).In(l)
		w.Header().Set("Cache-Control", "no-store")
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(views.ServicesFragment(loc, r.URL.Query())); err != nil {
			log.ErrorContext(ctx, "services_patch_failed", "path", r.URL.Path, "error", err.Error())
		}
	}
}
