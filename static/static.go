// Package static embeds the site's built static files in the Go binary;
// internal/server serves them under /static/.
package static

import "embed"

// Files holds the static files. Phase 3 adds the built CSS, scripts, fonts
// and image variants.
//
//go:embed *.svg
var Files embed.FS
