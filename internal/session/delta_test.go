package session

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = 255
	}
	return img
}

func TestDiffTiles(t *testing.T) {
	a := solid(200, 130, color.RGBA{R: 10, G: 20, B: 30})
	b := solid(200, 130, color.RGBA{R: 10, G: 20, B: 30})
	// Change one pixel in tile (1,0) and a block in tile (3,2).
	b.SetRGBA(70, 5, color.RGBA{R: 99, A: 255})
	for y := 130 - 2; y < 130; y++ {
		b.SetRGBA(199, y, color.RGBA{G: 99, A: 255})
	}
	rects := DiffTiles(a, b, 64)
	if len(rects) != 2 {
		t.Fatalf("got %d rects %+v, want 2", len(rects), rects)
	}
	found := map[[2]int]bool{}
	for _, r := range rects {
		found[[2]int{r.X, r.Y}] = true
	}
	if !found[[2]int{64, 0}] || !found[[2]int{192, 128}] {
		t.Fatalf("rects %+v", rects)
	}
	if n := len(DiffTiles(a, a, 64)); n != 0 {
		t.Fatalf("identical frames: %d rects", n)
	}
	if n := len(DiffTiles(nil, a, 64)); n == 0 {
		t.Fatal("nil prev should report all tiles")
	}
}

func TestEncodeCompositeRoundTrip(t *testing.T) {
	base := solid(128, 128, color.RGBA{R: 100, A: 255})
	next := solid(128, 128, color.RGBA{R: 100, A: 255})
	for y := 0; y < 64; y++ {
		for x := 64; x < 128; x++ {
			next.SetRGBA(x, y, color.RGBA{B: 200, A: 255})
		}
	}
	rects := DiffTiles(base, next, 64)
	if len(rects) != 1 {
		t.Fatalf("%d rects", len(rects))
	}
	tiles, err := EncodeTiles(next, rects, 90)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := CompositeTiles(base, tiles)
	if err != nil {
		t.Fatal(err)
	}
	// JPEG is lossy; check the composited region is clearly the new color.
	_, _, bb, _ := merged.At(96, 32).RGBA()
	if bb>>8 < 150 {
		t.Fatalf("composited blue %d, want ~200", bb>>8)
	}
	rr, _, _, _ := merged.At(32, 96).RGBA()
	if rr>>8 < 80 {
		t.Fatalf("untouched region red %d, want ~100", rr>>8)
	}
}

func TestViewerDeltaCompositing(t *testing.T) {
	v := &Viewer{}
	// A delta frame before any full frame is skipped.
	fr := ScreenFrame{Seq: 1, Tiles: []Tile{{Rect: Rect{X: 0, Y: 0, W: 64, H: 64}, Image: []byte("x")}}}
	if v.resolveFrame(&fr) {
		t.Fatal("delta without base must be skipped")
	}
	// Full frame establishes the base.
	full := solid(128, 128, color.RGBA{R: 50, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, full, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	fr2 := ScreenFrame{Seq: 2, Full: true, Image: buf.Bytes()}
	if !v.resolveFrame(&fr2) {
		t.Fatal("full frame rejected")
	}
	// Now a delta composits into a full JPEG.
	next := solid(128, 128, color.RGBA{R: 50, A: 255})
	for y := 8; y < 24; y++ {
		for x := 8; x < 24; x++ {
			next.SetRGBA(x, y, color.RGBA{G: 250, A: 255})
		}
	}
	rects := DiffTiles(v.base, next, 64)
	tiles, err := EncodeTiles(next, rects, 90)
	if err != nil {
		t.Fatal(err)
	}
	fr3 := ScreenFrame{Seq: 3, Tiles: tiles}
	if !v.resolveFrame(&fr3) {
		t.Fatal("delta with base rejected")
	}
	if !fr3.Full || len(fr3.Image) == 0 || fr3.Tiles != nil {
		t.Fatalf("resolved frame %+v", fr3)
	}
	_, gg, _, _ := v.base.At(16, 16).RGBA()
	if gg>>8 < 200 {
		t.Fatalf("composited green %d, want ~250", gg>>8)
	}
}
