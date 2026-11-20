package session

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// Valid tiny canvases exercise the complete absolute-coordinate delta path.
// No screen capture, transport, recording or native input is started.
func FuzzDeltaOffsetRoundTrip(f *testing.F) {
	f.Add(int64(17), int64(23), uint8(31), uint8(17), uint8(15), uint8(7), uint8(7))
	f.Add(int64(-19), int64(-11), uint8(1), uint8(1), uint8(0), uint8(0), uint8(255))
	f.Fuzz(func(t *testing.T, x, y int64, width, height, step, px, py uint8) {
		ox, oy := int(x%64), int(y%64)
		w, h := int(width%32)+1, int(height%32)+1
		// Cap tile count to keep every mutation tiny; whole-frame caps are tested
		// separately. Odd origins/parent strides still vary independently.
		size := int(step%16) + 16
		rect := image.Rect(ox, oy, ox+w, oy+h)
		parent := image.NewRGBA(rect.Inset(-3))
		nextParent := image.NewRGBA(parent.Bounds())
		for row := parent.Rect.Min.Y; row < parent.Rect.Max.Y; row++ {
			for column := parent.Rect.Min.X; column < parent.Rect.Max.X; column++ {
				c := color.RGBA{R: uint8(column * 3), G: uint8(row * 5), B: 73, A: 255}
				parent.SetRGBA(column, row, c)
				nextParent.SetRGBA(column, row, c)
			}
		}
		base := parent.SubImage(rect).(*image.RGBA)
		next := nextParent.SubImage(rect).(*image.RGBA)
		changed := image.Pt(ox+int(px)%w, oy+int(py)%h)
		old := next.RGBAAt(changed.X, changed.Y)
		old.R ^= 255
		next.SetRGBA(changed.X, changed.Y, old)
		before := append([]byte(nil), base.Pix...)
		rectangles := DiffTiles(base, next, size)
		if len(rectangles) != 1 {
			t.Fatal("one changed pixel must affect one tile", rectangles)
		}
		tiles, err := EncodeTiles(next, rectangles, 80)
		if err != nil {
			t.Fatal(err)
		}
		out, err := CompositeTiles(base, tiles)
		if err != nil {
			t.Fatal(err)
		}
		if out.Rect != base.Rect || !bytes.Equal(base.Pix, before) {
			t.Fatal("composition changed source ownership/bounds")
		}
		decoded, err := jpeg.Decode(bytes.NewReader(tiles[0].Image))
		if err != nil {
			t.Fatal(err)
		}
		tile := tiles[0].Rect
		for row := rect.Min.Y; row < rect.Max.Y; row++ {
			for column := rect.Min.X; column < rect.Max.X; column++ {
				want := base.RGBAAt(column, row)
				if column >= tile.X && column < tile.X+tile.W && row >= tile.Y && row < tile.Y+tile.H {
					want = color.RGBAModel.Convert(decoded.At(column-tile.X, row-tile.Y)).(color.RGBA)
				}
				if out.RGBAAt(column, row) != want {
					t.Fatal("absolute tile composition differs", column, row)
				}
			}
		}
		out.SetRGBA(changed.X, changed.Y, color.RGBA{})
		if !bytes.Equal(base.Pix, before) {
			t.Fatal("output aliases parent source")
		}
	})
}

// Refused untrusted rectangles/JPEGs must never partly paint or alias a base.
// A known-good first tile makes malformed trailing data exercise atomic refusal.
func FuzzDeltaCompositeRefusalAtomic(f *testing.F) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, solid(4, 4, color.RGBA{G: 177, A: 255}), nil); err != nil {
		f.Fatal(err)
	}
	f.Add(int64(-2), int64(-4), int64(4), int64(4), encoded.Bytes())
	f.Add(int64(-2), int64(-4), int64(5), int64(4), encoded.Bytes())
	f.Add(int64(-2), int64(-4), int64(4), int64(4), encoded.Bytes()[:len(encoded.Bytes())-2])
	f.Add(int64(8), int64(8), int64(8), int64(8), []byte{})
	f.Add(int64(-1), int64(0), int64(8), int64(8), []byte{1})
	f.Add(int64(0), int64(0), int64(1<<62), int64(8), []byte{2, 255})
	f.Fuzz(func(t *testing.T, x, y, width, height int64, raw []byte) {
		if len(raw) > 4096 {
			t.Skip()
		}
		parent := image.NewRGBA(image.Rect(-7, -9, 25, 23))
		for i := range parent.Pix {
			parent.Pix[i] = uint8(i * 17)
		}
		base := parent.SubImage(image.Rect(-3, -5, 13, 11)).(*image.RGBA)
		before := append([]byte(nil), parent.Pix...)
		goodRect := Rect{X: -3, Y: -5, W: 4, H: 4}
		good := Tile{Rect: goodRect, Image: jpegBytes(t, solid(4, 4, color.RGBA{R: 201, A: 255}))}
		candidate := Tile{Rect: Rect{X: int(x), Y: int(y), W: int(width), H: int(height)}, Image: raw}
		tiles := []Tile{good, candidate}
		out, err := CompositeTiles(base, tiles)
		if !bytes.Equal(parent.Pix, before) {
			t.Fatal("composite mutated source parent")
		}
		if err != nil {
			if out != nil {
				t.Fatal("refusal returned partial output")
			}
			return
		}
		if out == nil || out.Bounds() != base.Bounds() {
			t.Fatal("successful composition lost bounds")
		}
		// Every accepted trailing JPEG is tiny, bounded by the advertised16px base.
		// Compare against independently decoded pixels, with last tile winning.
		for row := base.Rect.Min.Y; row < base.Rect.Max.Y; row++ {
			for column := base.Rect.Min.X; column < base.Rect.Max.X; column++ {
				want := base.RGBAAt(column, row)
				for _, tile := range tiles {
					r := image.Rect(tile.Rect.X, tile.Rect.Y, tile.Rect.X+tile.Rect.W, tile.Rect.Y+tile.Rect.H)
					if !image.Pt(column, row).In(r) {
						continue
					}
					decoded, decodeErr := jpeg.Decode(bytes.NewReader(tile.Image))
					if decodeErr != nil {
						t.Fatal(decodeErr)
					}
					want = color.RGBAModel.Convert(decoded.At(column-r.Min.X, row-r.Min.Y)).(color.RGBA)
				}
				if out.RGBAAt(column, row) != want {
					t.Fatal("composition differs from last-tile pixel", column, row)
				}
			}
		}
		out.SetRGBA(-3, -5, color.RGBA{})
		if !bytes.Equal(parent.Pix, before) {
			t.Fatal("output aliases source parent")
		}
	})
}
