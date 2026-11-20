//go:build linux

package main

import (
	"bytes"
	"image"
	"testing"
)

// Offline strip generation only. No X11, wire frame, viewer grant or input.
func FuzzViewerPixelStripsMatchIndependentRGBA(f *testing.F) {
	for _, seed := range [][]byte{{0}, {255}, {1, 2, 3, 4, 5, 6, 7, 8}, {0, 255, 127, 64}} {
		f.Add(seed, uint8(0), uint8(0))
	}
	f.Fuzz(func(t *testing.T, raw []byte, ox, oy uint8) {
		if len(raw) == 0 || len(raw) > 256 {
			t.Skip()
		}
		at := func(i int) int { return int(raw[i%len(raw)]) }
		w, h := 1+at(0)%64, 1+at(1)%64
		parent := image.NewRGBA(image.Rect(int(ox)-128, int(oy)-128, int(ox)-128+w+8, int(oy)-128+h+8))
		for i := range parent.Pix {
			parent.Pix[i] = raw[i%len(raw)]
		}
		frame := parent.SubImage(image.Rect(parent.Rect.Min.X+3, parent.Rect.Min.Y+2, parent.Rect.Min.X+3+w, parent.Rect.Min.Y+2+h)).(*image.RGBA)
		before := append([]byte(nil), parent.Pix...)
		sx, sy := at(2)%w, at(3)%h
		source := image.Rect(sx, sy, sx+1+at(4)%(w-sx), sy+1+at(5)%(h-sy))
		dw, dh := 1+at(6)%128, 1+at(7)%128
		sampler := newViewerPixelSource(frame, source, dw, dh)
		full := make([]byte, dw*dh*4)
		// Read channels through RGBAAt and independently calculate coordinates,
		// rather than reusing byte offsets, stride or cached column calculations.
		for y := 0; y < dh; y++ {
			for x := 0; x < dw; x++ {
				point := image.Pt(frame.Rect.Min.X+sx+x*source.Dx()/dw, frame.Rect.Min.Y+sy+y*source.Dy()/dh)
				color := frame.RGBAAt(point.X, point.Y)
				i := (y*dw + x) * 4
				full[i], full[i+1], full[i+2] = color.B, color.G, color.R
			}
		}
		var combined []byte
		step := 1 + at(8)%dh
		for y := 0; y < dh; y += step {
			rows := min(step, dh-y)
			strip := sampler.strip(y, rows)
			want := full[y*dw*4 : (y+rows)*dw*4]
			if !bytes.Equal(strip, want) {
				t.Fatal("strip differs from independent RGBA channels", frame.Bounds(), source, dw, dh, y, rows)
			}
			combined = append(combined, strip...)
			if len(strip) > 0 {
				strip[0] ^= 255
			} // No returned strip aliases retained pixels.
		}
		if !bytes.Equal(combined, full) || !bytes.Equal(parent.Pix, before) {
			t.Fatal("strip partition or source ownership changed")
		}
		// A repeated read must not inherit mutations to an earlier returned strip.
		if !bytes.Equal(sampler.strip(0, dh), full) {
			t.Fatal("strip output aliased another result")
		}
	})
}
