//go:build linux

package main

import "image"

// Input geometry is the validated viewport region, not wire-supplied indices.
// Cache invariant nearest-pixel columns once for this repaint, not per strip.
type viewerPixelSource struct {
	pixels                        []byte
	stride                        int
	columns                       []int
	sourceY, sourceHeight, height int
}

func newViewerPixelSource(frame *image.RGBA, source image.Rectangle, width, height int) viewerPixelSource {
	columns := make([]int, width)
	for x := range columns {
		columns[x] = (source.Min.X + x*source.Dx()/width) * 4
	}
	return viewerPixelSource{pixels: frame.Pix, stride: frame.Stride, columns: columns, sourceY: source.Min.Y, sourceHeight: source.Dy(), height: height}
}
func (s viewerPixelSource) strip(y, rows int) []byte {
	width := len(s.columns)
	pixels := make([]byte, width*rows*4)
	for row := 0; row < rows; row++ {
		sourceRow := (s.sourceY + (y+row)*s.sourceHeight/s.height) * s.stride
		for x, offset := range s.columns {
			src := sourceRow + offset
			i := (row*width + x) * 4
			pixels[i], pixels[i+1], pixels[i+2] = s.pixels[src+2], s.pixels[src+1], s.pixels[src]
		}
	}
	return pixels
}
