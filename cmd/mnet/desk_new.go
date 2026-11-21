package main

import (
	"fintex.au/mnet/internal/desk"
)

// starterDeskLayout builds a two-display starter arrangement from the actual
// local root pixel size. Every label is a local placeholder the person edits;
// none of it is a verified device, pairing, identity or run.
func starterDeskLayout(pixelWidth, pixelHeight int64) desk.Layout {
	if pixelWidth < 1 {
		pixelWidth = 1
	}
	if pixelWidth > 65535 {
		pixelWidth = 65535
	}
	if pixelHeight < 1 {
		pixelHeight = 1
	}
	if pixelHeight > 65535 {
		pixelHeight = 65535
	}
	return desk.Layout{Version: 1, Displays: []desk.Display{
		{ID: "this-display", Device: "this-computer", X: 0, Y: 0, Width: 100, Height: 100, PixelWidth: pixelWidth, PixelHeight: pixelHeight},
		{ID: "next-display", Device: "next-computer", X: 100, Y: 0, Width: 100, Height: 100, PixelWidth: pixelWidth, PixelHeight: pixelHeight},
	}}
}

// A starter is unsaved new content: no opened file backs it, so it starts
// dirty and Save is immediately available while the arrangement stays valid.
func newDeskLayoutStarterEditor(layout desk.Layout) (*deskLayoutEditor, error) {
	if _, err := desk.Compile(layout); err != nil {
		return nil, err
	}
	return &deskLayoutEditor{
		layout:  cloneDeskLayout(layout),
		edge:    desk.Right,
		starter: true,
	}, nil
}
