//go:build linux

package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"

	"fintex.au/mnet/internal/portal"
)

func TestBGRAToRGBA(t *testing.T) {
	// One pixel, BGRA: blue 1, green 2, red 3.
	raw := []byte{1, 2, 3, 255}
	img := bgraToRGBA(raw, 1, 1)
	if img.Pix[0] != 3 || img.Pix[1] != 2 || img.Pix[2] != 1 || img.Pix[3] != 255 {
		t.Fatalf("pixel = %v, want RGBA 3 2 1 255", img.Pix)
	}
}

// portalCapturerFromPump wires a capturer over a scripted raw source,
// bypassing D-Bus and gst: the same path Capture exercises live.
func portalCapturerFromPump(p *portal.Pump) *portalCapturer {
	c := &portalCapturer{pump: p}
	c.jpegQ.Store(70)
	return c
}

func solidFrame(w, h int, b, g, r byte) []byte {
	raw := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		raw[i*4+0] = b
		raw[i*4+1] = g
		raw[i*4+2] = r
		raw[i*4+3] = 255
	}
	return raw
}

func TestPortalCapturerChangeDetection(t *testing.T) {
	w, h := 8, 4
	f1 := solidFrame(w, h, 10, 20, 30)
	f2 := solidFrame(w, h, 90, 80, 70)
	stream := bytes.NewReader(append(append(append([]byte{}, f1...), f1...), f2...))
	c := portalCapturerFromPump(portal.NewRawPump(stream, w, h))

	if gw, gh := c.Geometry(); gw != w || gh != h {
		t.Fatalf("Geometry = %dx%d", gw, gh)
	}
	j1, changed, err := c.Capture()
	if err != nil || !changed {
		t.Fatalf("first frame: changed=%v err=%v", changed, err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(j1)); err != nil {
		t.Fatalf("first frame not JPEG: %v", err)
	}
	if _, changed, err := c.Capture(); err != nil || changed {
		t.Fatalf("identical second frame: changed=%v err=%v", changed, err)
	}
	j3, changed, err := c.Capture()
	if err != nil || !changed {
		t.Fatalf("changed third frame: changed=%v err=%v", changed, err)
	}
	if bytes.Equal(j1, j3) {
		t.Fatal("different content produced identical JPEG")
	}
	img, _ := jpeg.Decode(bytes.NewReader(j3))
	px := img.(*image.YCbCr)
	_ = px
	if _, changed, err := c.Capture(); err == nil || changed {
		t.Fatalf("read past end: changed=%v err=%v", changed, err)
	}
}

func TestPortalCapturerQualityClamp(t *testing.T) {
	c := portalCapturerFromPump(portal.NewRawPump(bytes.NewReader(nil), 2, 2))
	c.SetQuality(5)
	if got := c.jpegQ.Load(); got != 20 {
		t.Fatalf("low clamp = %d", got)
	}
	c.SetQuality(200)
	if got := c.jpegQ.Load(); got != 90 {
		t.Fatalf("high clamp = %d", got)
	}
}
