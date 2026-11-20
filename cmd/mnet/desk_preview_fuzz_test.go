package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"testing"

	"fintex.au/mnet/internal/desk"
)

// Independent arbitrary-precision paint oracle. It does not call the preview's
// projection or convert rounded diagram rectangles into native edge candidates.
func exactPreviewRects(layout desk.Layout, width, height int64) []deskPreviewRect {
	minx, miny, maxx, maxy := int64(math.MaxInt64), int64(math.MaxInt64), int64(math.MinInt64), int64(math.MinInt64)
	for _, d := range layout.Displays {
		minx = min(minx, d.X)
		miny = min(miny, d.Y)
		maxx = max(maxx, d.X+d.Width)
		maxy = max(maxy, d.Y+d.Height)
	}
	scale := big.NewRat(width, maxx-minx)
	if other := big.NewRat(height, maxy-miny); other.Cmp(scale) < 0 {
		scale = other
	}
	floor := func(value int64) int64 {
		r := new(big.Rat).Mul(big.NewRat(value, 1), scale)
		return new(big.Int).Quo(r.Num(), r.Denom()).Int64()
	}
	ox, oy := (width-floor(maxx-minx))/2, (height-floor(maxy-miny))/2
	result := make([]deskPreviewRect, len(layout.Displays))
	for i, d := range layout.Displays {
		x, y := floor(d.X-minx), floor(d.Y-miny)
		result[i] = deskPreviewRect{ox + x, oy + y, max(int64(1), floor(d.X+d.Width-minx)-x), max(int64(1), floor(d.Y+d.Height-miny)-y)}
	}
	return result
}

func FuzzDeskPreviewProjectionAndLocalState(f *testing.F) {
	for _, seed := range [][]byte{{0}, {255}, {30, 255, 0, 127, 1, 254}, {1, 0, 0, 0, 255, 255}, {30, 0, 255, 255, 0, 1}} {
		f.Add(seed, uint16(839), uint16(249))
	}
	f.Add([]byte{30, 255, 255}, uint16(0), uint16(0))
	f.Add([]byte{30, 0, 0}, uint16(4095), uint16(4095))
	f.Fuzz(func(t *testing.T, raw []byte, w, h uint16) {
		if len(raw) == 0 || len(raw) > 128 {
			t.Skip()
		}
		at := func(i int) int64 { return int64(raw[i%len(raw)]) }
		count := 2 + int(at(0)%31)
		// Connected strips cover tiny/large paint dimensions, all32 labels,
		// translated origins, unequal layout widths/heights and native densities.
		vertical := at(1)%2 != 0
		cross := 1 + at(2)*3921
		displays := make([]desk.Display, count)
		origin := int64(-1000000)
		for i := range displays {
			size := 1 + at(i+3)*200
			d := desk.Display{ID: fmt.Sprintf("display%d", i), Device: fmt.Sprintf("device%d", i), X: origin, Y: -1000000, Width: size, Height: cross, PixelWidth: 1 + (at(i+4)*257)%65535, PixelHeight: 1 + (at(i+5)*257)%65535}
			if vertical {
				d.X, d.Y = d.Y, d.X
				d.Width, d.Height = d.Height, d.Width
			}
			displays[i] = d
			origin += size
		}
		layout := desk.Layout{Version: 1, Displays: displays}
		compiled, err := desk.Compile(layout)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(layout)
		preview := &deskLayoutPreview{layout: layout, compiled: compiled, edge: desk.Right}
		width, height := 1+int64(w%4096), 1+int64(h%4096)
		got, err := preview.rectangles(width, height)
		if err != nil {
			t.Fatal(err)
		}
		want := exactPreviewRects(layout, width, height)
		for i, r := range got {
			if r != want[i] || r.x < 0 || r.y < 0 || r.w < 1 || r.h < 1 || r.x+r.w > width || r.y+r.h > height {
				t.Fatal("projection differs or escapes viewport", i, r, want[i], width, height)
			}
			// Half-open hit oracle explicitly counts every projected rectangle. A
			// full-size but overlapped target is still ambiguous, never guessed.
			for _, point := range [][2]int64{{r.x, r.y}, {r.x + r.w/2, r.y + r.h/2}, {r.x + r.w - 1, r.y + r.h - 1}, {r.x + r.w, r.y + r.h}, {-1, -1}, {width, height}} {
				expected, matches := -1, 0
				if point[0] >= 0 && point[1] >= 0 && point[0] < width && point[1] < height {
					for j, other := range want {
						if point[0] >= other.x && point[1] >= other.y && point[0] < other.x+other.w && point[1] < other.y+other.h {
							matches++
							if other.w >= 44 && other.h >= 44 {
								expected = j
							}
						}
					}
				}
				if matches != 1 {
					expected = -1
				}
				if hit := preview.hitDisplay(width, height, point[0], point[1]); hit != expected {
					t.Fatal("ambiguous/tiny/outside target", point, hit, expected)
				}
			}
			preview.selectDisplay(1)
			if preview.selected != (i+1)%count || preview.along != 0 {
				t.Fatal("selection lost labels or retained native pixel")
			}
			for _, edge := range []desk.Edge{desk.Left, desk.Right, desk.Top, desk.Bottom} {
				if err := preview.selectEdge(edge); err != nil {
					t.Fatal(err)
				}
				preview.move(math.MaxInt64)
				if preview.along != preview.extent()-1 {
					t.Fatal("positive state overflow")
				}
				preview.move(math.MinInt64)
				if preview.along != 0 {
					t.Fatal("negative state overflow")
				}
			}
		}
		after, _ := json.Marshal(layout)
		if string(before) != string(after) {
			t.Fatal("local inspection mutated geometry")
		}
	})
}
