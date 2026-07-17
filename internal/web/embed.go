package web

import "embed"

// templateFS, HTML şablonlarını binary'ye gömer.
//
//go:embed templates/*.html
var templateFS embed.FS

// staticFS, statik varlıkları (CSS, JS) binary'ye gömer.
//
//go:embed static
var staticFS embed.FS
