//go:build linux

package main

import (
	"bytes"
	"image"
	"testing"
)

func referenceViewerStrip(frame *image.RGBA, source image.Rectangle, w, h, y, n int) []byte {
	b := frame.Bounds()
	pixels := make([]byte, w*n*4)
	for row := 0; row < n; row++ {
		for x := 0; x < w; x++ {
			src := frame.PixOffset(b.Min.X+source.Min.X+x*source.Dx()/w, b.Min.Y+source.Min.Y+(y+row)*source.Dy()/h)
			i := (row*w + x) * 4
			pixels[i], pixels[i+1], pixels[i+2] = frame.Pix[src+2], frame.Pix[src+1], frame.Pix[src]
		}
	}
	return pixels
}
func TestViewerPixelMappingMatchesOriginalNearestSampling(t *testing.T) {
	parent := image.NewRGBA(image.Rect(-9, -7, 19, 17))
	for i := range parent.Pix {
		parent.Pix[i] = byte(i*37 + i/19)
	}
	for _, frame := range []*image.RGBA{parent, parent.SubImage(image.Rect(-3, 2, 13, 14)).(*image.RGBA), image.NewRGBA(image.Rect(0, 0, 1, 1))} {
		b := frame.Bounds()
		for _, source := range []image.Rectangle{image.Rect(0, 0, b.Dx(), b.Dy()), image.Rect(b.Dx()/2, b.Dy()/2, b.Dx(), b.Dy())} {
			for _, size := range [][2]int{{1, 1}, {3, 5}, {32, 24}, {127, 93}} {
				w, h := size[0], size[1]
				sampler := newViewerPixelSource(frame, source, w, h)
				for _, step := range []int{1, 3, 64} {
					for y := 0; y < h; y += step {
						n := min(step, h-y)
						if got, want := sampler.strip(y, n), referenceViewerStrip(frame, source, w, h, y, n); !bytes.Equal(got, want) {
							t.Fatal("changed nearest-pixel sampling/stride/origin/BGRX padding", b, source, size, y, n)
						}
					}
				}
			}
		}
	}
}
func BenchmarkViewerPixelSourceOriginal(b *testing.B) {
	frame := image.NewRGBA(image.Rect(0, 0, 1280, 960))
	source := image.Rect(0, 0, 1280, 960)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = referenceViewerStrip(frame, source, 970, 728, 0, 728)
	}
}
func BenchmarkViewerPixelSourceCachedColumns(b *testing.B) {
	frame := image.NewRGBA(image.Rect(0, 0, 1280, 960))
	source := image.Rect(0, 0, 1280, 960)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sampler := newViewerPixelSource(frame, source, 970, 728)
		_ = sampler.strip(0, 728)
	}
}
