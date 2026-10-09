package main

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/mirror.svg
var renewMirrorSVGData []byte

// A monochrome vector hand mirror follows the surrounding theme's accent
// without needing separate light and dark bitmaps.
var renewMirrorSVG = ui.MustParseSVG(renewMirrorSVGData)
