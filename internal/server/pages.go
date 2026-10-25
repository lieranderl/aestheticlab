package server

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

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
