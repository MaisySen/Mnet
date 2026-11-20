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
