package session

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
)

func jpegBytes(t *testing.T, img *image.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// splitMJPEG separates concatenated JPEGs on their SOI/EOI markers.
func splitMJPEG(b []byte) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
			return nil
		}
		i := bytes.Index(b[2:], []byte{0xff, 0xd9})
		if i < 0 {
			return nil
		}
		out = append(out, b[:i+4])
		b = b[i+4:]
	}
	return out
}

func TestRecorderKeyframesAndDeltas(t *testing.T) {
	path := t.TempDir() + "/rec.mjpeg"
	r, err := NewRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	red := solid(64, 64, color.RGBA{200, 30, 30, 255})
	r.OnFrame(ScreenFrame{Seq: 1, Full: true, Image: jpegBytes(t, red)})

	// A delta that repaints the top-left 32x32 in blue.
	changed := solid(64, 64, color.RGBA{200, 30, 30, 255})
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			changed.SetRGBA(x, y, color.RGBA{30, 30, 220, 255})
		}
	}
	tiles, err := EncodeTiles(changed, []Rect{{X: 0, Y: 0, W: 32, H: 32}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	r.OnFrame(ScreenFrame{Seq: 2, Tiles: tiles})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if r.Frames() != 2 {
		t.Fatalf("frames: %d", r.Frames())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("recording must be 0600: %v %v", info.Mode(), err)
	}
	parts := splitMJPEG(raw)
	if len(parts) != 2 {
		t.Fatalf("expected 2 playable frames, got %d", len(parts))
	}
	second, err := jpeg.Decode(bytes.NewReader(parts[1]))
	if err != nil {
		t.Fatal(err)
	}
	b := second.Bounds()
	if b.Dx() != 64 || b.Dy() != 64 {
		t.Fatalf("delta frame size: %v", b)
	}
	px := second.At(16, 16)
	_, _, bl, _ := color.RGBA{30, 30, 220, 255}.RGBA()
	_, _, gotB, _ := px.RGBA()
	if gotB < bl/2 {
		t.Fatalf("composited tile missing: blue channel %d", gotB)
	}
}

func TestRecorderSkipsBaselessDelta(t *testing.T) {
	r, err := NewRecorder(t.TempDir() + "/x.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.OnFrame(ScreenFrame{Seq: 1, Tiles: []Tile{{Rect: Rect{X: 0, Y: 0, W: 1, H: 1}, Image: []byte{1, 2, 3}}}})
	if r.Frames() != 0 {
		t.Fatal("delta before keyframe must be skipped")
	}
}
