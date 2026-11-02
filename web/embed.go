// Package web holds the embedded, build-free console assets.
package web

import "embed"

// FS contains public browser assets only, never test or Go sources.
//
//go:embed index.html styles.css app.js
var FS embed.FS
