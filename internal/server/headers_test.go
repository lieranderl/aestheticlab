package server_test

import (
	"testing"

	"github.com/lieranderl/aestheticlab/internal/server"
)

// The policy is src/shared/security-headers.ts's, without 'unsafe-inline':
// Datastar evaluates expressions ('unsafe-eval'), and pages carry no inline
// scripts or style attributes.
func TestEveryResponseCarriesTheSecurityHeaders(t *testing.T) {
	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'self'; form-action 'self'; " +
			"script-src 'self' 'unsafe-eval' https://www.googletagmanager.com; style-src 'self'; img-src 'self' data: https:; font-src 'self' data:; " +
			"connect-src 'self' https://www.googletagmanager.com https://*.google-analytics.com; frame-src https://bookings.gettimely.com; upgrade-insecure-requests",
		"Permissions-Policy":        "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "SAMEORIGIN",
	}
	h := server.New(server.Config{})
	// A page that exists and one that does not get the same headers.
	for _, path := range []string{"/healthz", "/nope"} {
		res := get(t, h, path)
		for name, value := range want {
			if got := res.Header().Get(name); got != value {
				t.Errorf("%s %s = %q, want %q", path, name, got, value)
			}
		}
	}
}
