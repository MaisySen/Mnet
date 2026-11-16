package session

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"sync"
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

func TestRecorderBrokenFrameRequiresRecoveryKeyframe(t *testing.T) {
	good := ScreenFrame{Full: true, Image: jpegBytes(t, solid(16, 16, color.RGBA{R: 99, A: 255}))}
	tile := Tile{Rect: Rect{W: 8, H: 8}, Image: jpegBytes(t, solid(8, 8, color.RGBA{G: 99, A: 255}))}
	for _, broken := range []ScreenFrame{{Full: true}, {Full: true, Image: []byte("broken")}, {Tiles: []Tile{{Rect: Rect{W: 8, H: 8}, Image: []byte("broken")}}}} {
		r, err := NewRecorder(t.TempDir() + "/record.mjpeg")
		if err != nil {
			t.Fatal(err)
		}
		r.OnFrame(good)
		r.OnFrame(broken)
		if r.base != nil {
			t.Error("broken frame left stale recording base")
		}
		r.OnFrame(ScreenFrame{Tiles: []Tile{tile}})
		if r.Frames() != 1 {
			t.Error("recorded a delta from a broken chain")
		}
		r.OnFrame(good)
		r.OnFrame(ScreenFrame{Tiles: []Tile{tile}})
		if r.Frames() != 3 {
			t.Error("valid keyframe did not recover recording")
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecorderLateFrameAfterCloseIsIgnored(t *testing.T) {
	r, err := NewRecorder(t.TempDir() + "/record.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	good := ScreenFrame{Full: true, Image: jpegBytes(t, solid(16, 16, color.RGBA{R: 99, A: 255}))}
	r.OnFrame(good)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r.OnFrame(good)
	if r.Frames() != 1 {
		t.Fatal("late frame recorded")
	}
	if r.base != nil {
		t.Fatal("closed recorder retains frame pixels")
	}
}

func TestRecorderCloseSerializesWithFrames(t *testing.T) {
	r, err := NewRecorder(t.TempDir() + "/record.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	good := ScreenFrame{Full: true, Image: jpegBytes(t, solid(16, 16, color.RGBA{R: 99, A: 255}))}
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < 20; i++ {
			r.OnFrame(good)
		}
	}()
	go func() {
		defer workers.Done()
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	}()
	workers.Wait()
	if r.base != nil || r.w != nil {
		t.Fatal("closed recorder retains resources")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecorderRefusesExistingDestinationWithoutChangingIt(t *testing.T) {
	path := t.TempDir() + "/existing.mjpeg"
	original := []byte("preserve previous recording")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRecorder(path)
	if r != nil {
		r.Close()
	}
	if err == nil {
		t.Error("recorder overwrote existing destination")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, original) {
		t.Fatalf("destination changed: %q %v", after, readErr)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode() != before.Mode() {
		t.Fatalf("destination permissions changed: %v", err)
	}
}

func TestRecorderRefusesFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := dir + "/target"
	original := []byte("do not truncate")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	link := dir + "/record.mjpeg"
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	r, err := NewRecorder(link)
	if r != nil {
		r.Close()
	}
	if err == nil {
		t.Error("recorder followed final symlink")
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil || !bytes.Equal(content, original) {
		t.Fatalf("symlink target changed: %q %v", content, readErr)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink changed: %v", err)
	}
}

func TestConcurrentRecorderCreationHasOneOwner(t *testing.T) {
	path := t.TempDir() + "/one.mjpeg"
	var wg sync.WaitGroup
	results := make(chan *Recorder, 12)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, _ := NewRecorder(path); results <- r }()
	}
	wg.Wait()
	close(results)
	count := 0
	for r := range results {
		if r != nil {
			count++
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if count != 1 {
		t.Fatalf("recording owners: %d", count)
	}
}

func TestRecorderWriteFailureStopsAndSurvivesRepeatedClose(t *testing.T) {
	path := t.TempDir() + "/readonly"
	if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path) // a deterministic invalid write handle, not disk exhaustion
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := &Recorder{w: f}
	fr := ScreenFrame{Full: true, Image: jpegBytes(t, solid(16, 16, color.RGBA{R: 100, A: 255}))}
	r.OnFrame(fr)
	if r.w != nil || r.base != nil {
		t.Error("failed recording retained writer/frame pixels")
	}
	if r.Frames() != 0 {
		t.Error("failed frame counted")
	}
	first := r.Close()
	if first == nil {
		t.Fatal("recording write failure was lost at close")
	}
	r.OnFrame(fr)
	if again := r.Close(); again == nil || again.Error() != first.Error() {
		t.Fatalf("repeated close lost first error: %v", again)
	}
	if r.Frames() != 0 || r.base != nil {
		t.Fatal("late frame changed failed recording")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("fixture content changed: %q %v", content, err)
	}
}

func TestRecorderDeltaWriteFailureRetainsCompletedFrameCount(t *testing.T) {
	r, err := NewRecorder(t.TempDir() + "/delta.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	img := solid(16, 16, color.RGBA{R: 100, A: 255})
	r.OnFrame(ScreenFrame{Full: true, Image: jpegBytes(t, img)})
	if r.Frames() != 1 {
		t.Fatal("missing initial frame")
	}
	// Replace the output with a read-only handle after a valid base exists.
	if err := r.w.Close(); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/readonly"
	if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r.w = f
	tiles, err := EncodeTiles(img, []Rect{{W: 8, H: 8}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	r.OnFrame(ScreenFrame{Tiles: tiles})
	if r.Frames() != 1 || r.w != nil || r.base != nil {
		t.Fatal("failed delta counted or retained resources")
	}
	if err := r.Close(); err == nil {
		t.Fatal("failed delta lost output error")
	}
}

func TestRecorderCloseErrorIsSticky(t *testing.T) {
	r, err := NewRecorder(t.TempDir() + "/close.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.w.Close(); err != nil {
		t.Fatal(err)
	}
	first := r.Close()
	if first == nil {
		t.Fatal("close error lost")
	}
	if again := r.Close(); again == nil || again.Error() != first.Error() {
		t.Fatalf("repeated close lost error: %v", again)
	}
	if r.w != nil || r.base != nil {
		t.Fatal("close retained resources")
	}
}
