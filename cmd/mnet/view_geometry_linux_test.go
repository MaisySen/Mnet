//go:build linux

package main

import (
	"image"
	"testing"
)

func TestViewerOneToOneReceivedPixelRegion(t *testing.T) {
	v := viewerViewport{}
	if !v.key('1', 1280, 960, 700, 448) {
		t.Fatal("local one-to-one unavailable")
	}
	r := v.region(1280, 960, 700, 448)
	if r.source != image.Rect(0, 0, 700, 448) || r.target != image.Rect(0, 0, 700, 448) {
		t.Fatalf("one-to-one stretches/crops wrong pixels %+v", r)
	}
	v.key(0xff57, 1280, 960, 700, 448)
	r = v.region(1280, 960, 700, 448)
	if r.source != image.Rect(580, 512, 1280, 960) {
		t.Fatalf("bottom-right pan %+v", r)
	}
	v.key(0xff51, 1280, 960, 700, 448)
	v.key(0xff52, 1280, 960, 700, 448)
	if r = v.region(1280, 960, 700, 448); r.source.Min != image.Pt(516, 448) {
		t.Fatalf("bounded arrow step %+v", r)
	}
	// A smaller received frame must clamp old pan, not display old pixels or
	// invent a larger source crop. Small images stay centered, never upscaled.
	r = v.region(320, 240, 700, 448)
	if r.source != image.Rect(0, 0, 320, 240) || r.target != image.Rect(190, 104, 510, 344) {
		t.Fatalf("frame size change %+v", r)
	}
	v.key('f', 1280, 960, 700, 448)
	if v.one {
		t.Fatal("return to fit failed")
	}
	if v.key(0xff53, 1280, 960, 700, 448) {
		t.Fatal("fit arrows unexpectedly pan")
	}
}
func TestViewerRegionBoundsAcrossFramesAndWindows(t *testing.T) {
	for _, one := range []bool{false, true} {
		for _, sw := range []int{1, 79, 320, 1280, 8192} {
			for _, sh := range []int{1, 250, 960, 8192} {
				for _, dw := range []int{1, 65, 700, 2048} {
					for _, dh := range []int{1, 448, 1500} {
						v := viewerViewport{one: one, x: 100000, y: -10}
						r := v.region(sw, sh, dw, dh)
						if r.source.Empty() || r.target.Empty() || !r.source.In(image.Rect(0, 0, sw, sh)) || !r.target.In(image.Rect(0, 0, dw, dh)) {
							t.Fatalf("invalid region %+v source%d,%d local%d,%d", r, sw, sh, dw, dh)
						}
						if one && (r.source.Dx() != r.target.Dx() || r.source.Dy() != r.target.Dy()) {
							t.Fatalf("received pixel ratio changed %+v", r)
						}
					}
				}
			}
		}
	}
}
func TestViewerViewportInvalidAndUnrelatedKeys(t *testing.T) {
	v := viewerViewport{one: true, x: 10, y: 20}
	before := v
	if v.key('a', 1280, 960, 700, 448) || v != before {
		t.Fatal("unrelated key changed local view")
	}
	for _, dims := range [][4]int{{0, 1, 1, 1}, {1, -1, 1, 1}, {1, 1, 0, 1}, {1, 1, 1, -1}} {
		if r := v.region(dims[0], dims[1], dims[2], dims[3]); !r.source.Empty() || !r.target.Empty() {
			t.Fatal("invalid region accepted")
		}
	}
}
