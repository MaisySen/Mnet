package desk

import (
	"math"
	"testing"
)

func fixture() Layout {
	return Layout{Version: 1, Displays: []Display{{"left", "source", -1000, -200, 1000, 800, 2000, 1600}, {"right", "target", 0, 0, 1200, 600, 1800, 900}}}
}
func TestLayoutUnequalDPIAndNegativeOffsets(t *testing.T) {
	c, err := Compile(fixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source      string
		edge        Edge
		along, x, y int64
		ok          bool
	}{{"left", Right, 0, 0, 0, false}, {"left", Right, 399, 0, 0, false}, {"left", Right, 400, 0, 0, true}, {"left", Right, 800, 0, 300, true}, {"left", Right, 1599, 0, 899, true}, {"right", Left, 0, 1999, 400, true}, {"right", Left, 899, 1999, 1598, true}} {
		got, ok, err := c.MapEdge(tc.source, tc.edge, tc.along)
		if err != nil || ok != tc.ok || ok && (got.X != tc.x || got.Y != tc.y) {
			t.Fatalf("%+v got%+v ok%v err%v", tc, got, ok, err)
		}
	}
}
func TestLayoutRejectsInvalidAndDisconnected(t *testing.T) {
	for _, mutation := range []func(*Layout){func(l *Layout) { l.Version = 2 }, func(l *Layout) { l.Displays[1].ID = l.Displays[0].ID }, func(l *Layout) { l.Displays[1].X = 1 }, func(l *Layout) { l.Displays[1].X = -1 }, func(l *Layout) { l.Displays[1].Y = 600 }, func(l *Layout) { l.Displays[0].Width = math.MaxInt64 }, func(l *Layout) { l.Displays[0].PixelWidth = 0 }, func(l *Layout) { l.Displays[0].Device = "a\nforged" }} {
		l := fixture()
		mutation(&l)
		if _, err := Compile(l); err == nil {
			t.Fatalf("invalid layout accepted %+v", l)
		}
	}
}
func TestLayoutOwnsImmutableCopiesAndBoundsInput(t *testing.T) {
	l := fixture()
	c, err := Compile(l)
	if err != nil {
		t.Fatal(err)
	}
	l.Displays[1].Device = "changed"
	links := c.Links()
	links[0].To = "changed"
	got, ok, err := c.MapEdge("left", Right, 800)
	if err != nil || !ok || got.Device != "target" {
		t.Fatal("caller mutated geometry")
	}
	for _, tc := range []struct {
		id    string
		edge  Edge
		along int64
	}{{"missing", Right, 0}, {"left", "diagonal", 0}, {"left", Right, -1}, {"left", Right, 1600}, {"left", Right, math.MaxInt64}} {
		if _, _, err := c.MapEdge(tc.id, tc.edge, tc.along); err == nil {
			t.Fatal("unbounded candidate query accepted")
		}
	}
}
func TestLayoutNarrowSplitEdgesAndMultiDisplayDevice(t *testing.T) {
	l := Layout{Version: 1, Displays: []Display{{"primary", "same", 0, 0, 100, 100, 1000, 1000}, {"upper", "same", 100, 0, 100, 50, 100, 50}, {"lower", "other", 100, 50, 100, 50, 200, 100}}}
	c, err := Compile(l)
	if err != nil {
		t.Fatal(err)
	}
	a, ok, _ := c.MapEdge("primary", Right, 499)
	if !ok || a.Display != "upper" || a.Y != 49 {
		t.Fatal(a)
	}
	b, ok, _ := c.MapEdge("primary", Right, 500)
	if !ok || b.Display != "lower" || b.Y != 0 {
		t.Fatal(b)
	}
	if _, ok, _ := c.MapEdge("upper", Top, 50); ok {
		t.Fatal("corner-only transition invented")
	}
}
func FuzzLayoutCandidateBounds(f *testing.F) {
	f.Add(int64(0), int64(600), int64(900), int64(500))
	f.Fuzz(func(t *testing.T, y, h, pixel, along int64) {
		l := fixture()
		l.Displays[1].Y = y
		l.Displays[1].Height = h
		l.Displays[1].PixelHeight = pixel
		c, err := Compile(l)
		if err != nil {
			return
		}
		got, ok, err := c.MapEdge("left", Right, along)
		if err == nil && ok && (got.X < 0 || got.Y < 0 || got.X >= l.Displays[1].PixelWidth || got.Y >= pixel) {
			t.Fatalf("mapped out of bounds %+v", got)
		}
	})
}

func TestLayoutAllEdgesAndExtremeBounds(t *testing.T) {
	for _, tc := range []struct {
		edge         Edge
		x, y         int64
		wantX, wantY int64
	}{{Left, -100, 0, 1499, 750}, {Right, 100, 0, 0, 750}, {Top, 0, -100, 750, 1499}, {Bottom, 0, 100, 750, 0}} {
		l := Layout{Version: 1, Displays: []Display{{"a", "one", 0, 0, 100, 100, 1000, 1000}, {"b", "two", tc.x, tc.y, 100, 100, 1500, 1500}}}
		c, err := Compile(l)
		if err != nil {
			t.Fatal(err)
		}
		got, ok, err := c.MapEdge("a", tc.edge, 500)
		if err != nil || !ok || got.X != tc.wantX || got.Y != tc.wantY {
			t.Fatalf("%s %+v %v", tc.edge, got, err)
		}
	}
	l := Layout{Version: 1, Displays: []Display{{"a", "one", 1000000, -1000000, 1000000, 1000000, 65535, 65535}, {"b", "two", 1000000, 0, 1000000, 1000000, 65535, 65535}}}
	c, err := Compile(l)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := c.MapEdge("a", Bottom, 65534)
	if err != nil || !ok || got.X != 65534 || got.Y != 0 {
		t.Fatal("bounded arithmetic failed", got, err)
	}
}
func TestLayoutOneUnitOverlapAndHalfOpenSplit(t *testing.T) {
	l := Layout{Version: 1, Displays: []Display{{"a", "one", 0, 0, 10, 10, 1000, 1000}, {"b", "two", 10, 9, 10, 1, 100, 7}}}
	c, err := Compile(l)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.MapEdge("a", Right, 899); ok {
		t.Fatal("crossed outside narrow overlap")
	}
	got, ok, err := c.MapEdge("a", Right, 900)
	if err != nil || !ok || got.Y != 0 {
		t.Fatal(got, ok, err)
	}
	got, ok, err = c.MapEdge("a", Right, 999)
	if err != nil || !ok || got.Y != 6 {
		t.Fatal(got, ok, err)
	}
}
