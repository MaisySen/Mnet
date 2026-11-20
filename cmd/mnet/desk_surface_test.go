package main

import (
	"bytes"
	"fintex.au/mnet/internal/desk"
	"strings"
	"testing"
)

func TestDeskSurfaceFitsExactFileLabelsAndGeometry(t *testing.T) {
	layout := desk.Layout{Version: 1, Displays: []desk.Display{
		{ID: strings.Repeat("W", 64), Device: strings.Repeat("W", 64), Width: 100, Height: 100, PixelWidth: 65535, PixelHeight: 65535},
		{ID: strings.Repeat("B", 64), Device: strings.Repeat("W", 64), X: 100, Width: 100, Height: 100, PixelWidth: 65535, PixelHeight: 65535},
	}}
	compiled, err := desk.Compile(layout)
	if err != nil {
		t.Fatal(err)
	}
	p := &deskLayoutPreview{layout: layout, compiled: compiled, edge: desk.Right, along: 65534}
	for _, details := range []bool{false, true} {
		for focus := -1; focus < 10; focus++ {
			im, err := renderDeskSurface(p, focus, details)
			if err != nil {
				t.Fatal(err)
			}
			if im.Bounds().Dx() != 900 || im.Bounds().Dy() != 650 {
				t.Fatal("size changed")
			}
		}
	}
	before := p.layout
	plain, err := renderDeskSurface(p, -1, false)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := renderDeskSurface(p, -1, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(plain.Pix, detail.Pix) {
		t.Fatal("detail disclosure not distinct")
	}
	if p.layout.Displays[0] != before.Displays[0] || p.along != 65534 || p.edge != desk.Right {
		t.Fatal("renderer changed layout/query")
	}
}
