package main

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"fintex.au/mnet/internal/desk"
)

func TestDeskPreviewExactGeometryAndNoPaintAuthority(t *testing.T) {
	p, err := readDeskLayoutPreview(strings.NewReader(deskFixture))
	if err != nil {
		t.Fatal(err)
	}
	rects, err := p.rectangles(840, 250)
	if err != nil || len(rects) != 2 {
		t.Fatalf("projection %v %v", rects, err)
	}
	if rects[0].w != 250 || rects[0].h != 250 || rects[1].x != rects[0].x+250 || rects[0].y != rects[1].y {
		t.Fatalf("aspect/adjacency %v", rects)
	}
	p.move(500)
	candidate, ok, err := p.candidate()
	if err != nil || !ok || candidate.Display != "b" || candidate.X != 0 || candidate.Y != 750 {
		t.Fatalf("not exact native candidate %+v %v %v", candidate, ok, err)
	}
	p.selectEdge(desk.Left)
	if p.along != 0 {
		t.Fatal("changed edge retained tangent")
	}
	_, ok, err = p.candidate()
	if err != nil || ok {
		t.Fatal("dead edge inherited paint/link")
	}
	if p.selectEdge("approve") == nil {
		t.Fatal("invalid edge")
	}
	p.selectDisplay(1)
	p.selectEdge(desk.Left)
	p.move(750)
	candidate, ok, err = p.candidate()
	if err != nil || !ok || candidate.Display != "a" || candidate.X != 999 || candidate.Y != 500 {
		t.Fatalf("return candidate %+v %v %v", candidate, ok, err)
	}
	for _, bounds := range [][2]int64{{0, 250}, {840, 0}, {4097, 250}, {840, 4097}} {
		if _, err = p.rectangles(bounds[0], bounds[1]); err == nil {
			t.Fatal("unbounded viewport", bounds)
		}
	}
}
func TestDeskPreviewSelectionAndNativePixelBounds(t *testing.T) {
	p, err := readDeskLayoutPreview(strings.NewReader(deskFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range []desk.Edge{desk.Left, desk.Right, desk.Top, desk.Bottom} {
		p.selectEdge(edge)
		p.move(math.MaxInt64)
		if p.along != p.extent()-1 {
			t.Fatal("positive overflow")
		}
		p.move(1)
		if p.along != p.extent()-1 {
			t.Fatal("endpoint wrapped")
		}
		p.move(math.MinInt64)
		if p.along != 0 {
			t.Fatal("negative overflow")
		}
		p.move(-1)
		if p.along != 0 {
			t.Fatal("underflow")
		}
	}
	p.move(12)
	p.selectDisplay(-1)
	if p.selected != 1 || p.along != 0 {
		t.Fatal("selection inherited tangent")
	}
	p.selectDisplay(1)
	if p.selected != 0 {
		t.Fatal("selection did not wrap")
	}
	p.selectDisplay(-100)
	if p.selected != 0 {
		t.Fatal("large step")
	}
}
func TestDeskPreviewAll32AndTinyRectanglesNeverCreateNeighbors(t *testing.T) {
	//32 legal connected displays, including one native-pixel dimension. The
	// diagram must retain all labels through selection even when paint collapses.
	displays := make([]desk.Display, 32)
	for i := range displays {
		displays[i] = desk.Display{ID: fmt.Sprintf("d%d", i), Device: fmt.Sprintf("node%d", i), X: int64(i)*100000 - 1000000, Y: -1000000, Width: 100000, Height: 1, PixelWidth: 65535, PixelHeight: 1}
	}
	// Keep all origins within the validated +/-one-million bound.
	for i := range displays {
		displays[i].X = int64(i)*60000 - 1000000
		displays[i].Width = 60000
	}
	l := desk.Layout{Version: 1, Displays: displays}
	c, err := desk.Compile(l)
	if err != nil {
		t.Fatal(err)
	}
	p := &deskLayoutPreview{layout: l, compiled: c, edge: desk.Bottom}
	rects, err := p.rectangles(840, 250)
	if err != nil || len(rects) != 32 {
		t.Fatal(rects, err)
	}
	for i, r := range rects {
		if r.x < 0 || r.y < 0 || r.w < 1 || r.h < 1 || r.x+r.w > 840 || r.y+r.h > 250 {
			t.Fatal("out of viewport", i, r)
		}
		p.selected = i
		if p.display().ID != displays[i].ID {
			t.Fatal("lost display")
		}
		if _, ok, err := p.candidate(); err != nil || ok {
			t.Fatal("thin paint supplied nonexistent bottom neighbor")
		}
	}
}
func TestDeskPreviewRefusesInvalidOfflineFiles(t *testing.T) {
	for _, data := range []string{deskFixture + "{}", strings.Replace(deskFixture, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(deskFixture, `"version":1`, `"approved":true,"version":1`, 1), strings.Replace(deskFixture, `"x":0`, `"x":1`, 1), strings.Repeat(" ", 65537)} {
		if _, err := readDeskLayoutPreview(strings.NewReader(data)); err == nil {
			t.Fatal("invalid offline diagram accepted")
		}
	}
}

func TestDeskPreviewArgsCannotBecomeRunOrImplicitInput(t *testing.T) {
	for _, args := range [][]string{nil, {"layout-view"}, {"layout-view", "file", "--approve"}, {"run", "file"}, {"layout-view", "file", "--edge", "a", "right", "50"}} {
		if validDeskArgs(args) {
			t.Fatal("invalid preview command accepted", args)
		}
	}
	for _, args := range [][]string{{"layout-view", "file"}, {"layout-check", "file"}, {"layout-check", "file", "--edge", "a", "right", "50"}} {
		if !validDeskArgs(args) {
			t.Fatal("valid offline command refused", args)
		}
	}
}

func TestDeskPreviewAllEdgesKeepNativeOffsetMapping(t *testing.T) {
	for _, edge := range []desk.Edge{desk.Left, desk.Right, desk.Top, desk.Bottom} {
		a := desk.Display{ID: "a", Device: "source", X: -100, Y: 0, Width: 100, Height: 100, PixelWidth: 1000, PixelHeight: 1000}
		b := desk.Display{ID: "b", Device: "destination", PixelWidth: 1500, PixelHeight: 1500}
		want := desk.Candidate{Display: "b", Device: "destination"}
		switch edge {
		case desk.Left:
			b.X = -150
			b.Y = 25
			b.Width = 50
			b.Height = 50
			want.X = 1499
			want.Y = 750
		case desk.Right:
			b.X = 0
			b.Y = 25
			b.Width = 50
			b.Height = 50
			want.Y = 750
		case desk.Top:
			b.X = -75
			b.Y = -50
			b.Width = 50
			b.Height = 50
			want.X = 750
			want.Y = 1499
		case desk.Bottom:
			b.X = -75
			b.Y = 100
			b.Width = 50
			b.Height = 50
			want.X = 750
		}
		l := desk.Layout{Version: 1, Displays: []desk.Display{a, b}}
		c, err := desk.Compile(l)
		if err != nil {
			t.Fatal(err)
		}
		p := &deskLayoutPreview{layout: l, compiled: c, edge: edge, along: 500}
		got, ok, err := p.candidate()
		if err != nil || !ok || got != want {
			t.Fatalf("%s: %+v expected %+v %v %v", edge, got, want, ok, err)
		}
		p.along = 0
		if _, ok, err = p.candidate(); err != nil || ok {
			t.Fatal("dead corner promoted to candidate", edge)
		}
		rects, err := p.rectangles(840, 250)
		if err != nil || len(rects) != 2 {
			t.Fatal(edge, rects, err)
		}
	}
}

func TestDeskPreviewDirectHitsNeverGuessRoundedGeometry(t *testing.T) {
	p, err := readDeskLayoutPreview(strings.NewReader(deskFixture))
	if err != nil {
		t.Fatal(err)
	}
	rects, err := p.rectangles(840, 250)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rects {
		if got := p.hitDisplay(840, 250, r.x+r.w/2, r.y+r.h/2); got != i {
			t.Fatal("exact direct selection", i, got)
		}
	}
	boundary := rects[0].x + rects[0].w
	if got := p.hitDisplay(840, 250, boundary, 125); got != 1 {
		t.Fatal("half-open shared edge wrong display", got)
	}
	for _, point := range [][2]int64{{-1, 0}, {0, -1}, {840, 125}, {420, 250}, {0, 0}} {
		if p.hitDisplay(840, 250, point[0], point[1]) != -1 {
			t.Fatal("outside paint selected display", point)
		}
	}
	if p.hitDisplay(40, 40, 10, 10) != -1 {
		t.Fatal("tiny preview became mouse target")
	}
	if p.hitDisplay(0, 250, 0, 0) != -1 {
		t.Fatal("invalid preview target")
	}
	// Tall thin layouts keep exact selected labels through keyboard navigation,
	// not silently expanded or ambiguous mouse targets.
	p.layout.Displays[0].Width = 1
	p.layout.Displays[1].X = p.layout.Displays[0].X + 1
	p.layout.Displays[1].Width = 1
	rects, err = p.rectangles(840, 250)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rects {
		if p.hitDisplay(840, 250, r.x, r.y) != -1 {
			t.Fatal("thin display acquired inflated target")
		}
	}
}
