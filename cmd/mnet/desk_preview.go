package main

import (
	"errors"
	"io"

	"fintex.au/mnet/internal/desk"
)

// An offline diagram never supplies identity, pairing, consent or an active run.
type deskLayoutPreview struct {
	layout   desk.Layout
	compiled *desk.Compiled
	selected int
	edge     desk.Edge
	along    int64
}

func readDeskLayoutPreview(r io.Reader) (*deskLayoutPreview, error) {
	layout, err := desk.Decode(r)
	if err != nil {
		return nil, err
	}
	compiled, err := desk.Compile(layout)
	if err != nil {
		return nil, err
	}
	return &deskLayoutPreview{layout: layout, compiled: compiled, edge: desk.Right}, nil
}
func (p *deskLayoutPreview) display() desk.Display { return p.layout.Displays[p.selected] }
func (p *deskLayoutPreview) extent() int64 {
	d := p.display()
	if p.edge == desk.Left || p.edge == desk.Right {
		return d.PixelHeight
	}
	return d.PixelWidth
}
func (p *deskLayoutPreview) selectDisplay(step int) {
	count := len(p.layout.Displays)
	p.selected = (p.selected + step%count + count) % count
	p.along = 0
}
func (p *deskLayoutPreview) selectEdge(edge desk.Edge) error {
	if edge != desk.Left && edge != desk.Right && edge != desk.Top && edge != desk.Bottom {
		return errors.New("invalid diagram edge")
	}
	p.edge = edge
	p.along = 0
	return nil
}
func (p *deskLayoutPreview) move(step int64) {
	// Inputs are explicit native source-edge pixels. Clamp, never wrap or overshoot.
	end := p.extent() - 1
	if step > 0 && step > end-p.along {
		p.along = end
		return
	}
	if step < 0 && step < -p.along {
		p.along = 0
		return
	}
	p.along += step
}
func (p *deskLayoutPreview) candidate() (desk.Candidate, bool, error) {
	return p.compiled.MapEdge(p.display().ID, p.edge, p.along)
}

type deskPreviewRect struct{ x, y, w, h int64 }

// Project all source bounds with a single aspect-preserving rational scale.
// Round only for paint. Collapsed/touching paint pixels never create links.
func (p *deskLayoutPreview) rectangles(width, height int64) ([]deskPreviewRect, error) {
	if width < 1 || height < 1 || width > 4096 || height > 4096 {
		return nil, errors.New("invalid diagram viewport")
	}
	first := p.layout.Displays[0]
	minx, miny, maxx, maxy := first.X, first.Y, first.X+first.Width, first.Y+first.Height
	for _, d := range p.layout.Displays {
		minx = min(minx, d.X)
		miny = min(miny, d.Y)
		maxx = max(maxx, d.X+d.Width)
		maxy = max(maxy, d.Y+d.Height)
	}
	spanx, spany := maxx-minx, maxy-miny
	numerator, denominator := width, spanx
	if height*spanx < width*spany {
		numerator, denominator = height, spany
	}
	ox, oy := (width-spanx*numerator/denominator)/2, (height-spany*numerator/denominator)/2
	out := make([]deskPreviewRect, 0, len(p.layout.Displays))
	for _, d := range p.layout.Displays {
		x, y := (d.X-minx)*numerator/denominator, (d.Y-miny)*numerator/denominator
		right, bottom := (d.X+d.Width-minx)*numerator/denominator, (d.Y+d.Height-miny)*numerator/denominator
		out = append(out, deskPreviewRect{ox + x, oy + y, max(int64(1), right-x), max(int64(1), bottom-y)})
	}
	return out, nil
}

// Direct mouse selection is available only where one unambiguous display has
// a full44px paint target. Tiny/overlapping projections remain selectable with
// Previous/Next; never inflate a hitbox across another display or shared edge.
func (p *deskLayoutPreview) hitDisplay(width, height, x, y int64) int {
	if x < 0 || y < 0 || x >= width || y >= height {
		return -1
	}
	rectangles, err := p.rectangles(width, height)
	if err != nil {
		return -1
	}
	found := -1
	for i, r := range rectangles {
		if x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h {
			if found != -1 {
				return -1
			}
			if r.w < 44 || r.h < 44 {
				return -1
			}
			found = i
		}
	}
	return found
}
