//go:build linux

package main

import (
	"errors"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

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

// Fixed-size engineering controls do not reflow when the desktop changes.
// Watch the root, not only the child window, before any evidence read. The
// round trip after subscription closes the setup-to-watch geometry gap.
func nativeControlsWatchRoot(c *xgb.Conn, screen *xproto.ScreenInfo) error {
	if err := xproto.ChangeWindowAttributesChecked(c, screen.Root, xproto.CwEventMask, []uint32{xproto.EventMaskStructureNotify}).Check(); err != nil {
		return errors.New("native controls root watch unavailable")
	}
	geometry, err := xproto.GetGeometry(c, xproto.Drawable(screen.Root)).Reply()
	if err != nil || geometry.Width != screen.WidthInPixels || geometry.Height != screen.HeightInPixels {
		return errors.New("native controls root geometry changed")
	}
	return nil
}
func nativeControlsRootChanged(screen *xproto.ScreenInfo, event xproto.ConfigureNotifyEvent) bool {
	return event.Window == screen.Root && (event.Width != screen.WidthInPixels || event.Height != screen.HeightInPixels)
}
