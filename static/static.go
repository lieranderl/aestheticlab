// Package static embeds the site's static files in the Go binary;
// internal/server serves them under /static/.
package static

import "embed"

// Files holds favicon.svg and dist/, the content-hashed CSS, scripts, fonts
// and media frontend/build.mjs builds (bun run assets).
//
//go:embed favicon.svg all:dist
var Files embed.FS
