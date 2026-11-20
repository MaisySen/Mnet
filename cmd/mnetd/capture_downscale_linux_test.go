//go:build linux

package main

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestDownscaleUsesAbsoluteSourceBoundsAndStride(t *testing.T) {
	for _, origin := range []image.Point{{0, 0}, {11, 17}, {-13, -19}} {
		parent := image.NewRGBA(image.Rect(origin.X-3, origin.Y-2, origin.X+14, origin.Y+12))
		for y := parent.Rect.Min.Y; y < parent.Rect.Max.Y; y++ {
			for x := parent.Rect.Min.X; x < parent.Rect.Max.X; x++ {
				parent.SetRGBA(x, y, color.RGBA{uint8((x - parent.Rect.Min.X) * 13), uint8((y - parent.Rect.Min.Y) * 17), uint8((x - parent.Rect.Min.X + y - parent.Rect.Min.Y) * 7), 255})
			}
		}
		source := parent.SubImage(image.Rect(origin.X, origin.Y, origin.X+11, origin.Y+9)).(*image.RGBA)
		before := append([]byte(nil), parent.Pix...)
		for _, factor := range []int{1, 2, 3, 4} {
			got := downscale(source, factor)
			wantBounds := image.Rect(0, 0, 11/factor, 9/factor)
			if got.Bounds() != wantBounds {
				t.Fatal("scaled bounds", origin, factor, got.Bounds())
			}
			for y := 0; y < wantBounds.Dy(); y++ {
				for x := 0; x < wantBounds.Dx(); x++ {
					var red, green, blue int
					for dy := 0; dy < factor; dy++ {
						for dx := 0; dx < factor; dx++ {
							c := source.RGBAAt(origin.X+x*factor+dx, origin.Y+y*factor+dy)
							red += int(c.R)
							green += int(c.G)
							blue += int(c.B)
						}
					}
					n := factor * factor
					want := color.RGBA{uint8(red / n), uint8(green / n), uint8(blue / n), 255}
					if c := got.RGBAAt(x, y); c != want {
						t.Fatal("independent source average differs", origin, factor, x, y, c, want)
					}
				}
			}
			if !bytes.Equal(before, parent.Pix) {
				t.Fatal("downscale mutated source")
			}
		}
	}
}

func FuzzDownscaleAbsoluteOriginAndStride(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7}, uint8(0), uint8(0), uint8(2))
	f.Add([]byte{255, 17, 31, 63}, uint8(255), uint8(255), uint8(4))
	f.Fuzz(func(t *testing.T, raw []byte, ox, oy, scale uint8) {
		if len(raw) == 0 || len(raw) > 256 {
			t.Skip()
		}
		w, h := 1+int(raw[0]%32), 1+int(raw[len(raw)-1]%32)
		origin := image.Pt(int(ox)-128, int(oy)-128)
		parent := image.NewRGBA(image.Rect(origin.X-3, origin.Y-2, origin.X+w+4, origin.Y+h+3))
		for i := range parent.Pix {
			parent.Pix[i] = raw[i%len(raw)]
		}
		source := parent.SubImage(image.Rectangle{Min: origin, Max: origin.Add(image.Pt(w, h))}).(*image.RGBA)
		before := append([]byte(nil), parent.Pix...)
		factor := 1 + int(scale)%min(w, h)
		out := downscale(source, factor)
		if out.Bounds() != image.Rect(0, 0, w/factor, h/factor) {
			t.Fatal("wrong output bounds")
		}
		for y := 0; y < h/factor; y++ {
			for x := 0; x < w/factor; x++ {
				var red, green, blue int
				for yy := 0; yy < factor; yy++ {
					for xx := 0; xx < factor; xx++ {
						c := source.RGBAAt(origin.X+x*factor+xx, origin.Y+y*factor+yy)
						red += int(c.R)
						green += int(c.G)
						blue += int(c.B)
					}
				}
				n := factor * factor
				if got := out.RGBAAt(x, y); got != (color.RGBA{uint8(red / n), uint8(green / n), uint8(blue / n), 255}) {
					t.Fatal("independent average differs")
				}
			}
		}
		if !bytes.Equal(before, parent.Pix) {
			t.Fatal("source changed")
		}
	})
}
