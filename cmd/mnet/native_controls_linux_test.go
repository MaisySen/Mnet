//go:build linux

package main

import (
	"github.com/jezek/xgb/xproto"
	"testing"
)

func controlsScreenFixture() *xproto.ScreenInfo {
	return &xproto.ScreenInfo{RootDepth: 24, RootVisual: 42, AllowedDepths: []xproto.DepthInfo{{Depth: 24, Visuals: []xproto.VisualInfo{{VisualId: 42, Class: xproto.VisualClassTrueColor, RedMask: 0xff0000, GreenMask: 0xff00, BlueMask: 0xff}}}}}
}
func TestNativeControlsRequireExactRGBVisual(t *testing.T) {
	if nativeControlsRGB(nil) {
		t.Fatal("nil screen")
	}
	if !nativeControlsRGB(controlsScreenFixture()) {
		t.Fatal("supported visual refused")
	}
	for _, change := range []func(*xproto.ScreenInfo){
		func(s *xproto.ScreenInfo) { s.RootDepth = 8 }, func(s *xproto.ScreenInfo) { s.RootDepth = 16 }, func(s *xproto.ScreenInfo) { s.RootDepth = 32 }, func(s *xproto.ScreenInfo) { s.RootVisual = 43 }, func(s *xproto.ScreenInfo) { s.AllowedDepths[0].Depth = 16 }, func(s *xproto.ScreenInfo) { s.AllowedDepths = nil }, func(s *xproto.ScreenInfo) { s.AllowedDepths[0].Visuals[0].Class = xproto.VisualClassPseudoColor }, func(s *xproto.ScreenInfo) { s.AllowedDepths[0].Visuals[0].RedMask = 0xff }, func(s *xproto.ScreenInfo) { s.AllowedDepths[0].Visuals[0].GreenMask = 0xff }, func(s *xproto.ScreenInfo) { s.AllowedDepths[0].Visuals[0].BlueMask = 0xff0000 },
	} {
		screen := controlsScreenFixture()
		change(screen)
		if nativeControlsRGB(screen) {
			t.Fatal("unsupported RGB accepted", screen)
		}
	}
	screen := controlsScreenFixture()
	screen.AllowedDepths = append([]xproto.DepthInfo{{Depth: 8, Visuals: []xproto.VisualInfo{{VisualId: 99}}}}, screen.AllowedDepths...)
	if !nativeControlsRGB(screen) {
		t.Fatal("unrelated depths broke exact root")
	}
}

func TestNativeControlsRootChangesAreNotChildResizeOrMovement(t *testing.T) {
	screen := controlsScreenFixture()
	screen.Root = 1
	screen.WidthInPixels = 1024
	screen.HeightInPixels = 768
	for _, tc := range []struct {
		event   xproto.ConfigureNotifyEvent
		changed bool
	}{
		{xproto.ConfigureNotifyEvent{Window: 1, Width: 800, Height: 600}, true},
		{xproto.ConfigureNotifyEvent{Window: 1, Width: 1024, Height: 600}, true},
		{xproto.ConfigureNotifyEvent{Window: 1, Width: 2048, Height: 1536}, true},
		{xproto.ConfigureNotifyEvent{Window: 1, Width: 1024, Height: 768, X: 50, Y: 50}, false},
		{xproto.ConfigureNotifyEvent{Window: 2, Width: 800, Height: 600}, false},
	} {
		if got := nativeControlsRootChanged(screen, tc.event); got != tc.changed {
			t.Fatal(tc, got)
		}
	}
}
