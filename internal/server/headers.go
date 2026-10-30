package server

import (
	"net/http"
	"strings"
)

// localContentSecurityPolicy allows no 'unsafe-inline': Datastar evaluates its expressions
// ('unsafe-eval'), consent and gtag bootstrap come from a file, and pages
// carry no style="" attributes.
var localContentSecurityPolicy = strings.Join([]string{
	"default-src 'self'",
	"base-uri 'self'",
	"object-src 'none'",
	"frame-ancestors 'self'",
	"form-action 'self'",
	"script-src 'self' 'unsafe-eval' https://www.googletagmanager.com",
	"style-src 'self'",
	"img-src 'self' data: https:",
	"font-src 'self' data:",
	"connect-src 'self' https://www.googletagmanager.com https://*.google-analytics.com",
	"frame-src https://bookings.gettimely.com",
}, "; ")

// contentSecurityPolicy is the policy behind Cloud Run's HTTPS. Its
// upgrade-insecure-requests would break a server reached over plain HTTP:
// Safari upgrades even http://localhost, where there is no TLS.
var contentSecurityPolicy = localContentSecurityPolicy + "; upgrade-insecure-requests"

// securityHeaders sets the site's security headers on every response, before
// the handler runs, so errors and panics carry them too.
func securityHeaders(policy string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}
