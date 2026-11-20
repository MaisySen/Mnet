//go:build linux

package main

import (
	"sync/atomic"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)

// Actual disposable-server root resize, never fake Setup data. Turning off its
// output first permits Xvfb's root to shrink; all original CRTC data is restored.
func deskFixtureResizeSource(t *testing.T, c *xgb.Conn) func() {
	t.Helper()
	if err := randr.Init(c); err != nil {
		t.Fatal(err)
	}
	screen := xproto.Setup(c).DefaultScreen(c)
	resources, err := randr.GetScreenResourcesCurrent(c, screen.Root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	type original struct {
		crtc randr.Crtc
		info *randr.GetCrtcInfoReply
	}
	saved := []original{}
	for _, crtc := range resources.Crtcs {
		info, err := randr.GetCrtcInfo(c, crtc, resources.ConfigTimestamp).Reply()
		if err != nil {
			t.Fatal(err)
		}
		saved = append(saved, original{crtc, info})
		reply, err := randr.SetCrtcConfig(c, crtc, xproto.TimeCurrentTime, resources.ConfigTimestamp, 0, 0, 0, randr.RotationRotate0, nil).Reply()
		if err != nil || reply.Status != randr.SetConfigSuccess {
			t.Fatal("fixture output disable", err)
		}
	}
	var restored atomic.Bool
	restore := func() {
		if restored.Swap(true) {
			return
		}
		if err := randr.SetScreenSizeChecked(c, screen.Root, screen.WidthInPixels, screen.HeightInPixels, uint32(screen.WidthInMillimeters), uint32(screen.HeightInMillimeters)).Check(); err != nil {
			t.Error(err)
			return
		}
		for _, s := range saved {
			resources, err := randr.GetScreenResourcesCurrent(c, screen.Root).Reply()
			if err != nil {
				t.Error(err)
				return
			}
			reply, err := randr.SetCrtcConfig(c, s.crtc, xproto.TimeCurrentTime, resources.ConfigTimestamp, s.info.X, s.info.Y, s.info.Mode, s.info.Rotation, s.info.Outputs).Reply()
			if err != nil || reply.Status != randr.SetConfigSuccess {
				t.Error("fixture original CRTC restoration", err)
			}
		}
	}
	if err = randr.SetScreenSizeChecked(c, screen.Root, 800, 600, 200, 150).Check(); err != nil {
		restore()
		t.Fatal(err)
	}
	geometry, err := xproto.GetGeometry(c, xproto.Drawable(screen.Root)).Reply()
	if err != nil || geometry.Width != 800 || geometry.Height != 600 {
		restore()
		t.Fatal("actual root did not resize", err)
	}
	return restore
}
