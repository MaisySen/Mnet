package main

import (
	"bytes"
	"testing"
)

func TestViewerWaitingFitsAllSupportedPanelWidths(t *testing.T) {
	for width := 288; width <= 520; width++ {
		host, err := renderViewerWaiting(width, true)
		if err != nil {
			t.Fatal(width, err)
		}
		screen, err := renderViewerWaiting(width, false)
		if err != nil {
			t.Fatal(width, err)
		}
		if host.Bounds().Dx() != width || host.Bounds().Dy() != 120 {
			t.Fatal("wrong bounds")
		}
		if bytes.Equal(host.Pix, screen.Pix) {
			t.Fatal("admission and first screen conflated")
		}
	}
	for _, width := range []int{0, 287, 521, 8192} {
		if _, err := renderViewerWaiting(width, true); err == nil {
			t.Fatal("invalid size")
		}
	}
}
