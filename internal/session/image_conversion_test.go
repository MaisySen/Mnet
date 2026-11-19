package session

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"
)

func originalImageToRGBA(src image.Image) *image.RGBA {
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}
func TestImageToRGBAExactPixelsAndOwnership(t *testing.T) {
	decoded, err := jpeg.Decode(bytes.NewReader(fixtureJPEG(t, 0)))
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(519))
	sources := []image.Image{decoded}
	for _, ratio := range []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440, image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410} {
		y := image.NewYCbCr(image.Rect(-3, 5, 34, 38), ratio)
		rng.Read(y.Y)
		rng.Read(y.Cb)
		rng.Read(y.Cr)
		sources = append(sources, y, y.SubImage(image.Rect(1, 7, 30, 35)))
	}
	n := image.NewNRGBA(image.Rect(3, -4, 28, 19))
	rng.Read(n.Pix)
	g := image.NewGray(image.Rect(-4, -3, 13, 14))
	rng.Read(g.Pix)
	a := image.NewAlpha(image.Rect(7, 8, 22, 25))
	rng.Read(a.Pix)
	n64 := image.NewNRGBA64(image.Rect(2, 3, 19, 20))
	rng.Read(n64.Pix)
	rgba := image.NewRGBA(image.Rect(4, 5, 21, 22))
	rng.Read(rgba.Pix)
	pal := image.NewPaletted(image.Rect(2, 3, 19, 20), color.Palette{color.NRGBA{1, 2, 3, 0}, color.NRGBA{200, 100, 50, 123}, color.RGBA{9, 8, 7, 255}})
	for i := range pal.Pix {
		pal.Pix[i] = uint8(i % 3)
	}
	sources = append(sources, n, n.SubImage(image.Rect(5, 0, 22, 14)), g, a, n64, rgba, pal, image.NewGray(image.Rect(0, 0, 0, 0)))
	for i, src := range sources {
		want := originalImageToRGBA(src)
		got := imageToRGBA(src)
		if got.Rect != want.Rect || !bytes.Equal(got.Pix, want.Pix) {
			t.Errorf("image%d type%T pixel/bounds mismatch", i, src)
		}
		if source, ok := src.(*image.RGBA); ok && got != source {
			t.Error("existing RGBA ownership fast path changed")
		}
		if _, ok := src.(*image.RGBA); !ok && len(got.Pix) > 0 {
			before := originalImageToRGBA(src)
			got.Pix[0] ^= 255
			after := originalImageToRGBA(src)
			if !bytes.Equal(before.Pix, after.Pix) {
				t.Error("converted output aliases source")
			}
		}
	}
}
func BenchmarkImageConversion(b *testing.B) {
	for _, size := range []image.Point{{320, 240}, {1280, 720}} {
		src := image.NewYCbCr(image.Rect(0, 0, size.X, size.Y), image.YCbCrSubsampleRatio420)
		rng := rand.New(rand.NewSource(1))
		rng.Read(src.Y)
		rng.Read(src.Cb)
		rng.Read(src.Cr)
		for _, implementation := range []string{"original", "bulk"} {
			b.Run(fmt.Sprintf("%dx%d/%s", size.X, size.Y, implementation), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var out *image.RGBA
					if implementation == "original" {
						out = originalImageToRGBA(src)
					} else {
						out = imageToRGBA(src)
					}
					if len(out.Pix) != size.X*size.Y*4 {
						b.Fatal("bad output")
					}
				}
			})
		}
	}
}

func TestImageToRGBAConversionAllocationsDoNotScalePerPixel(t *testing.T) {
	for _, size := range []image.Point{{320, 240}, {1280, 720}} {
		src := image.NewYCbCr(image.Rect(0, 0, size.X, size.Y), image.YCbCrSubsampleRatio420)
		var out *image.RGBA
		allocations := testing.AllocsPerRun(3, func() { out = imageToRGBA(src) })
		if allocations > 4 || out == nil {
			t.Fatalf("%v conversion allocated per-pixel objects: %.0f", size, allocations)
		}
	}
}
