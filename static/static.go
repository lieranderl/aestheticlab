// Package static embeds the site's static files in the Go binary;
// internal/server serves them under /static/.
package static

import "embed"

// Files holds dist/: the content-hashed CSS, scripts, fonts and media
// frontend/build.mjs builds (bun run assets), and the link-preview images
// and root files (robots.txt, manifest.json, …) it copies from frontend/public/.
//
//go:embed all:dist
var Files embed.FS
