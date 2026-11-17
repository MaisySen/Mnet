package session

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"reflect"
	"testing"
	"time"
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

func TestViewerRejectsBrokenKeyframeAndWaitsForRecovery(t *testing.T) {
	for _, broken := range [][]byte{nil, []byte("not-jpeg")} {
		v := &Viewer{base: solid(16, 16, color.RGBA{R: 99, A: 255})}
		fr := ScreenFrame{Full: true, Image: broken}
		if v.resolveFrame(&fr) {
			t.Fatal("broken keyframe delivered")
		}
		if v.base != nil {
			t.Fatal("broken keyframe left stale delta base")
		}
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, solid(16, 16, color.RGBA{G: 99, A: 255}), nil); err != nil {
			t.Fatal(err)
		}
		delta := ScreenFrame{Tiles: []Tile{{Rect: Rect{W: 16, H: 16}, Image: encoded.Bytes()}}}
		if v.resolveFrame(&delta) {
			t.Fatal("delta accepted after broken keyframe")
		}
		good := ScreenFrame{Full: true, Image: encoded.Bytes()}
		if !v.resolveFrame(&good) {
			t.Fatal("valid keyframe did not recover")
		}
		if !v.resolveFrame(&delta) {
			t.Fatal("valid delta after recovery rejected")
		}
	}
}

func TestCompositeRejectsInvalidTileGeometry(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, solid(8, 8, color.RGBA{G: 99, A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	base := solid(16, 16, color.RGBA{R: 99, A: 255})
	if _, err := CompositeTiles(nil, nil); err == nil {
		t.Fatal("nil base accepted")
	}
	for _, rect := range []Rect{{X: -1, W: 8, H: 8}, {Y: -1, W: 8, H: 8}, {W: 0, H: 8}, {W: 8, H: -1}, {X: 12, W: 8, H: 8}, {Y: 12, W: 8, H: 8}, {W: 7, H: 8}, {W: 8, H: 7}, {X: 1, W: int(^uint(0) >> 1), H: 8}} {
		if _, err := CompositeTiles(base, []Tile{{Rect: rect, Image: encoded.Bytes()}}); err == nil {
			t.Fatalf("invalid tile accepted: %+v", rect)
		}
	}
}

func TestViewerBrokenDeltaClearsChain(t *testing.T) {
	v := &Viewer{base: solid(16, 16, color.RGBA{R: 99, A: 255})}
	bad := ScreenFrame{Tiles: []Tile{{Rect: Rect{W: 8, H: 8}, Image: []byte("broken")}}}
	if v.resolveFrame(&bad) || v.base != nil {
		t.Fatal("broken delta left a usable chain")
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, solid(8, 8, color.RGBA{G: 99, A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	next := ScreenFrame{Tiles: []Tile{{Rect: Rect{W: 8, H: 8}, Image: encoded.Bytes()}}}
	if v.resolveFrame(&next) {
		t.Fatal("delta accepted without recovered keyframe")
	}
}

func TestCompositeBoundsAggregateTileWork(t *testing.T) {
	base := solid(16, 16, color.RGBA{A: 255})
	tile := Tile{Rect: Rect{W: 8, H: 8}, Image: jpegBytes(t, solid(8, 8, color.RGBA{A: 255}))}
	// Four quadrants cover exactly one frame and remain supported.
	valid := []Tile{tile, tile, tile, tile}
	valid[1].Rect.X = 8
	valid[2].Rect.Y = 8
	valid[3].Rect.X = 8
	valid[3].Rect.Y = 8
	if _, err := CompositeTiles(base, valid); err != nil {
		t.Fatal(err)
	}
	if _, err := CompositeTiles(base, append(valid, tile)); err == nil {
		t.Fatal("aggregate tile pixels exceeded frame")
	}
	tiny := Tile{Rect: Rect{W: 1, H: 1}, Image: jpegBytes(t, solid(1, 1, color.RGBA{A: 255}))}
	tooMany := make([]Tile, 4097)
	for i := range tooMany {
		tooMany[i] = tiny
	}
	if _, err := CompositeTiles(solid(128, 128, color.RGBA{A: 255}), tooMany); err == nil {
		t.Fatal("excess tile count accepted")
	}
}

func TestViewerWireMalformedJSONBreaksDeltaChain(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	got := make(chan ScreenFrame, 4)
	ConnectViewerWithOptions(client, ViewerOptions{OnFrame: func(fr ScreenFrame) { got <- fr }})
	image := solid(16, 16, color.RGBA{R: 100, A: 255})
	key := ScreenFrame{Seq: 1, Full: true, Image: jpegBytes(t, image)}
	if err := host.SendJSON(StreamScreen, PriScreen, key); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("initial frame missing")
	}
	if err := host.SendBytes(StreamScreen, PriScreen, []byte(`{"seq":2,"tiles":`)); err != nil {
		t.Fatal(err)
	}
	tiles, err := EncodeTiles(image, []Rect{{W: 8, H: 8}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.SendJSON(StreamScreen, PriScreen, ScreenFrame{Seq: 3, Tiles: tiles}); err != nil {
		t.Fatal(err)
	}
	key.Seq = 4
	if err := host.SendJSON(StreamScreen, PriScreen, key); err != nil {
		t.Fatal(err)
	}
	select {
	case fr := <-got:
		if fr.Seq != 4 {
			t.Fatalf("delta after malformed JSON used stale base: seq%d", fr.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("valid keyframe did not recover")
	}
}

func TestViewerScreenExitReleasesFrameBase(t *testing.T) {
	for _, reason := range []string{"fin", "reset", "mux-close"} {
		t.Run(reason, func(t *testing.T) {
			a, b := net.Pipe()
			defer b.Close()
			m := newMux(a, StreamFile)
			defer m.Close()
			v := &Viewer{Mux: m, base: solid(16, 16, color.RGBA{R: 100, A: 255})}
			in := make(chan Frame, 1)
			switch reason {
			case "fin":
				close(in)
			case "reset":
				in <- Frame{Type: msgRst}
			case "mux-close":
				m.Close()
			}
			done := make(chan struct{})
			go func() { defer close(done); v.screenLoop(Stream{Mux: m, ID: StreamScreen, in: in}) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("screen loop did not exit")
			}
			v.frameMu.Lock()
			retained := v.base != nil
			v.frameMu.Unlock()
			if retained {
				t.Fatal("screen exit retained decoded pixels")
			}
		})
	}
}

func TestDiffTilesChangedBoundsRequestsFullRefresh(t *testing.T) {
	for _, tc := range []struct {
		name       string
		prev, curr *image.RGBA
	}{
		{"grow", solid(8, 8, color.RGBA{}), solid(16, 16, color.RGBA{})},
		{"shrink", solid(16, 16, color.RGBA{}), solid(8, 8, color.RGBA{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DiffTiles(tc.prev, tc.curr, 8)
			want := DiffTiles(nil, tc.curr, 8)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("bounds change not full refresh: %+v want %+v", got, want)
			}
		})
	}
}

func TestEncodeTilesRejectsInvalidRectangles(t *testing.T) {
	img := solid(16, 16, color.RGBA{A: 255})
	for _, r := range []Rect{
		{X: -1, W: 8, H: 8}, {Y: -1, W: 8, H: 8}, {X: 12, W: 8, H: 8}, {Y: 12, W: 8, H: 8},
		{W: 0, H: 8}, {W: 8, H: -1}, {X: int(^uint(0) >> 1), W: 8, H: 8},
	} {
		if tiles, err := EncodeTiles(img, []Rect{r}, 80); err == nil || tiles != nil {
			t.Errorf("invalid rectangle accepted: %+v tiles%d err%v", r, len(tiles), err)
		}
	}
}

func TestEncodeTilesNilAndOffsetSource(t *testing.T) {
	if tiles, err := EncodeTiles(nil, nil, 80); err == nil || tiles != nil {
		t.Fatal("nil source accepted")
	}
	img := image.NewRGBA(image.Rect(10, 20, 26, 36))
	rect := Rect{X: 10, Y: 20, W: 8, H: 8}
	tiles, err := EncodeTiles(img, []Rect{rect}, 80)
	if err != nil || len(tiles) != 1 {
		t.Fatalf("valid offset source rejected: %v", err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(tiles[0].Image))
	if err != nil || config.Width != 8 || config.Height != 8 || tiles[0].Rect != rect {
		t.Fatalf("geometry mismatch: %+v %v", config, err)
	}
}
