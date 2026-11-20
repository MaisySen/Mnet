//go:build linux

package main

import "github.com/jezek/xgb/xproto"

// These native controls paint literal RGB pixels, not allocated palette colors.
// A different visual must fail before any window or local-evidence read, rather
// than rendering an unreadable selection, disabled state or consent boundary.
// This is not the raw image upload format check needed by the desktop viewer.
func nativeControlsRGB(screen *xproto.ScreenInfo) bool {
	if screen == nil || screen.RootDepth != 24 {
		return false
	}
	for _, depth := range screen.AllowedDepths {
		if depth.Depth != 24 {
			continue
		}
		for _, visual := range depth.Visuals {
			if visual.VisualId == screen.RootVisual && visual.Class == xproto.VisualClassTrueColor && visual.RedMask == 0xff0000 && visual.GreenMask == 0xff00 && visual.BlueMask == 0xff {
				return true
			}
		}
	}
	return false
}
