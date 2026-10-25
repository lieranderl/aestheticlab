package server

import (
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzhttp"
)

// gzipWrapper compresses pages and Datastar's event streams for browsers
// that accept gzip; other content types pass through unchanged.
var gzipWrapper = func() func(http.Handler) http.HandlerFunc {
	wrap, err := gzhttp.NewWrapper(
		gzhttp.ContentTypes([]string{"text/html", "text/event-stream"}),
		gzhttp.EnableZstd(false),
	)
	if err != nil {
		panic(err) // Only invalid options fail, so this is a programming error.
	}
	return wrap
}()

// gzipPages compresses what is rendered per request. Files under /static/
// and /og/ skip it: they are precompressed at build time or are media, and
// the static handler sets their encoding and Vary itself.
func gzipPages(next http.Handler) http.Handler {
	gz := gzipWrapper(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/og/") {
			next.ServeHTTP(w, r)
			return
		}
		gz.ServeHTTP(w, r)
	})
}
