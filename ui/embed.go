package ui

import "embed"

// Dist contains the built chat UI shell (from ui/dist/).

//go:embed all:dist
var Dist embed.FS