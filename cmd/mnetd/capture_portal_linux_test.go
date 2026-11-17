//go:build linux

package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
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

func TestPortalCapturerDelta(t *testing.T) {
	w, h := 128, 128
	f1 := solidFrame(w, h, 10, 20, 30)
	f2 := append([]byte{}, f1...)
	// Dirty one 64x64 tile: the top-left quadrant changes color.
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			i := (y*w + x) * 4
			f2[i+0], f2[i+1], f2[i+2] = 200, 100, 50
		}
	}
	f3 := solidFrame(w, h, 90, 80, 70)
	var stream bytes.Buffer
	stream.Write(f1)
	stream.Write(f1) // identical: no change
	stream.Write(f2) // one tile dirty
	stream.Write(f3) // everything dirty
	c := portalCapturerFromPump(portal.NewRawPump(&stream, w, h))

	full, tiles, err := c.CaptureDelta()
	if err != nil || len(full) == 0 || len(tiles) != 0 {
		t.Fatalf("first frame: full=%d tiles=%d err=%v", len(full), len(tiles), err)
	}
	if full, tiles, err = c.CaptureDelta(); err != nil || full != nil || tiles != nil {
		t.Fatalf("identical frame: full=%v tiles=%v err=%v", full, tiles, err)
	}
	full, tiles, err = c.CaptureDelta()
	if err != nil || full != nil || len(tiles) != 1 {
		t.Fatalf("one dirty tile: full=%d tiles=%d err=%v", len(full), len(tiles), err)
	}
	if tiles[0].Rect.X != 0 || tiles[0].Rect.Y != 0 {
		t.Fatalf("tile origin = %d,%d", tiles[0].Rect.X, tiles[0].Rect.Y)
	}
	full, tiles, err = c.CaptureDelta()
	if err != nil || len(full) == 0 || len(tiles) != 0 {
		t.Fatalf("full repaint: full=%d tiles=%d err=%v", len(full), len(tiles), err)
	}
}

func TestPortalTokenPersistence(t *testing.T) {
	dir := t.TempDir()
	if got := loadPortalToken(dir); got != "" {
		t.Fatalf("missing token = %q", got)
	}
	savePortalToken(dir, "v2_token-abc")
	if got := loadPortalToken(dir); got != "v2_token-abc" {
		t.Fatalf("round trip = %q", got)
	}
	fi, err := os.Stat(filepath.Join(dir, "portal-restore-token"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("token mode = %o", fi.Mode().Perm())
	}
	// Empty state dir (proof rigs): a no-op, never a write.
	savePortalToken("", "v2_token-abc")
	if got := loadPortalToken(""); got != "" {
		t.Fatalf("stateless load = %q", got)
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

func TestPortalCapturerMismatchedBaseReturnsFullJPEG(t *testing.T) {
	for _, oldSize := range []int{8, 32} {
		c := portalCapturerFromPump(portal.NewRawPump(bytes.NewReader(solidFrame(16, 16, 0, 0, 0)), 16, 16))
		c.lastRGBA = image.NewRGBA(image.Rect(0, 0, oldSize, oldSize))
		full, tiles, err := c.CaptureDelta()
		if err != nil || len(full) == 0 || len(tiles) != 0 {
			t.Fatalf("old%d: expected full frame, got full%d tiles%d %v", oldSize, len(full), len(tiles), err)
		}
		config, err := jpeg.DecodeConfig(bytes.NewReader(full))
		if err != nil || config.Width != 16 || config.Height != 16 {
			t.Fatalf("new canvas not encoded: %+v %v", config, err)
		}
	}
}
