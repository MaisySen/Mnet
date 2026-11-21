package main

import (
	"strings"
	"testing"

	"fintex.au/mnet/internal/desk"
)

func editFixtureLayout() desk.Layout {
	return desk.Layout{Version: 1, Displays: []desk.Display{
		{ID: "source", Device: "dev-a", X: -100, Y: 0, Width: 100, Height: 100, PixelWidth: 1000, PixelHeight: 1000},
		{ID: "target", Device: "dev-b", X: 0, Y: 25, Width: 100, Height: 50, PixelWidth: 1500, PixelHeight: 1500},
	}}
}

func newTestEditor(t *testing.T) *deskLayoutEditor {
	t.Helper()
	e, err := newDeskLayoutEditor(editFixtureLayout())
	if err != nil {
		t.Fatalf("fixture layout must compile: %v", err)
	}
	return e
}

func TestEditorRejectsInvalidStart(t *testing.T) {
	bad := editFixtureLayout()
	bad.Version = 2
	if _, err := newDeskLayoutEditor(bad); err == nil {
		t.Fatal("unsupported version accepted")
	}
	bad = editFixtureLayout()
	bad.Displays = bad.Displays[:1]
	if _, err := newDeskLayoutEditor(bad); err == nil {
		t.Fatal("single-display layout accepted")
	}
	if _, err := readDeskLayoutEditor(strings.NewReader(`{"version":1,"displays":[]}`)); err == nil {
		t.Fatal("too few displays decoded")
	}
	if _, err := readDeskLayoutEditor(strings.NewReader(`{"version":1,"Displays":[]}`)); err == nil {
		t.Fatal("uppercase duplicate-style key decoded")
	}
}

func TestEditorMoveClampsAndUndoRestores(t *testing.T) {
	e := newTestEditor(t)
	if e.dirty() {
		t.Fatal("fresh editor dirty")
	}
	if !e.moveDisplay(-1, 0) {
		t.Fatal("move within bounds refused")
	}
	if got := e.display().X; got != -101 {
		t.Fatalf("moved to %d, want -101", got)
	}
	if !e.dirty() {
		t.Fatal("move not marked dirty")
	}
	// Clamp at the far negative bound without wrapping or overshooting.
	for i := 0; i < 20; i++ {
		e.moveDisplay(-100000, 0)
	}
	if got := e.display().X; got != -1000000 {
		t.Fatalf("clamped at %d, want -1000000", got)
	}
	before := len(e.undo)
	if e.moveDisplay(-100000, 0) {
		t.Fatal("clamped no-op move reported a change")
	}
	if len(e.undo) != before {
		t.Fatal("no-op move pushed an undo entry")
	}
	// Moving right is clamped so the rectangle never exceeds the far bound.
	e.layout.Displays[e.selected].Width = 100
	for i := 0; i < 40; i++ {
		e.moveDisplay(100000, 0)
	}
	if got := e.display().X; got != 1000000-100 {
		t.Fatalf("right clamp at %d, want %d", got, 1000000-100)
	}
	for len(e.undo) > 0 {
		if !e.undoEdit() {
			t.Fatal("undo stack underflow")
		}
	}
	if e.dirty() || !equalDeskLayout(e.layout, e.original) {
		t.Fatal("full undo did not restore the original arrangement")
	}
	if e.undoEdit() {
		t.Fatal("empty undo stack succeeded")
	}
}

func TestEditorResizeClamps(t *testing.T) {
	e := newTestEditor(t)
	e.resizeDisplay(-100000, -100000)
	d := e.display()
	if d.Width != 1 || d.Height != 1 {
		t.Fatalf("layout size clamped at %dx%d, want 1x1", d.Width, d.Height)
	}
	e.resizeDisplay(10000000, 10000000)
	d = e.display()
	if d.Width != 1000000 || d.Height != 1000000 {
		t.Fatalf("layout size clamped at %dx%d, want bounds", d.Width, d.Height)
	}
	e.resizePixels(-100000, -100000)
	d = e.display()
	if d.PixelWidth != 1 || d.PixelHeight != 1 {
		t.Fatalf("pixels clamped at %dx%d, want 1x1", d.PixelWidth, d.PixelHeight)
	}
	e.resizePixels(100000, 100000)
	d = e.display()
	if d.PixelWidth != 65535 || d.PixelHeight != 65535 {
		t.Fatalf("pixels clamped at %dx%d, want 65535x65535", d.PixelWidth, d.PixelHeight)
	}
}

func TestEditorAddAttachesSharedEdge(t *testing.T) {
	edges := []struct {
		edge       desk.Edge
		x, y, w, h int64
		overlaps   bool
	}{
		{desk.Right, 0, 0, 100, 100, true}, // lands on the fixture target at (0,25)
		{desk.Left, -200, 0, 100, 100, false},
		{desk.Bottom, -100, 100, 100, 100, false},
		{desk.Top, -100, -100, 100, 100, false},
	}
	for _, tc := range edges {
		e := newTestEditor(t)
		if err := e.selectEdge(tc.edge); err != nil {
			t.Fatal(err)
		}
		if err := e.addDisplay(); err != nil {
			t.Fatalf("add on %s: %v", tc.edge, err)
		}
		if len(e.layout.Displays) != 3 {
			t.Fatalf("add on %s gave %d displays", tc.edge, len(e.layout.Displays))
		}
		n := e.layout.Displays[2]
		if n.X != tc.x || n.Y != tc.y || n.Width != tc.w || n.Height != tc.h {
			t.Fatalf("add on %s placed at (%d,%d) %dx%d, want (%d,%d) %dx%d", tc.edge, n.X, n.Y, n.Width, n.Height, tc.x, tc.y, tc.w, tc.h)
		}
		if n.ID == "source" || n.ID == "target" || n.Device == "dev-a" || n.Device == "dev-b" {
			t.Fatalf("add on %s reused an identifier", tc.edge)
		}
		if e.selected != 2 {
			t.Fatalf("add on %s did not select the new display", tc.edge)
		}
		verr := e.validate()
		if tc.overlaps {
			if verr == nil || !strings.Contains(verr.Error(), "overlap") {
				t.Fatalf("add on %s should report overlap, got %v", tc.edge, verr)
			}
			if !e.undoEdit() {
				t.Fatalf("add on %s could not be undone", tc.edge)
			}
			if e.validate() != nil {
				t.Fatalf("undo of add on %s did not restore validity", tc.edge)
			}
		} else if verr != nil {
			t.Fatalf("add on %s produced invalid arrangement: %v", tc.edge, verr)
		}
		if !e.dirty() && !tc.overlaps {
			t.Fatalf("add on %s not dirty", tc.edge)
		}
	}
	if err := newTestEditor(t).selectEdge("diagonal"); err == nil {
		t.Fatal("invalid edge accepted")
	}
}

func TestEditorAddUniqueIDsAfterRemoval(t *testing.T) {
	e := newTestEditor(t)
	if err := e.addDisplay(); err != nil {
		t.Fatal(err)
	}
	first := e.layout.Displays[2].ID
	if err := e.removeDisplay(); err != nil {
		t.Fatal(err)
	}
	if err := e.addDisplay(); err != nil {
		t.Fatal(err)
	}
	if e.layout.Displays[2].ID != first {
		t.Fatalf("expected the freed ID %q to be reused, got %q", first, e.layout.Displays[2].ID)
	}
}

func TestEditorAddCapAndRemoveFloor(t *testing.T) {
	layout := desk.Layout{Version: 1}
	for i := 0; i < 32; i++ {
		layout.Displays = append(layout.Displays, desk.Display{
			ID:     "d" + strings.Repeat("x", 1) + string(rune('a'+i/26)) + string(rune('a'+i%26)),
			Device: "dev", X: int64(i), Y: 0, Width: 1, Height: 1, PixelWidth: 10, PixelHeight: 10,
		})
	}
	e, err := newDeskLayoutEditor(layout)
	if err != nil {
		t.Fatalf("32-display chain must compile: %v", err)
	}
	if err := e.addDisplay(); err == nil {
		t.Fatal("33rd display accepted")
	}
	if len(e.undo) != 0 {
		t.Fatal("refused add pushed an undo entry")
	}
	small := newTestEditor(t)
	if err := small.removeDisplay(); err == nil {
		t.Fatal("removal from a 2-display arrangement accepted")
	}
	if len(small.layout.Displays) != 2 || small.dirty() {
		t.Fatal("refused removal changed the arrangement")
	}
}

func TestEditorInvalidStatesBlockSave(t *testing.T) {
	// Overlap: drag the source onto the target.
	e := newTestEditor(t)
	e.moveDisplay(50, 10)
	if err := e.validate(); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap invalidity, got %v", err)
	}
	if _, err := e.saveBytes(); err == nil {
		t.Fatal("invalid arrangement saved")
	}
	if !e.undoEdit() {
		t.Fatal("undo after overlap failed")
	}
	if err := e.validate(); err != nil {
		t.Fatalf("undo did not restore validity: %v", err)
	}
	// Disconnect: move the target far away from the source.
	e = newTestEditor(t)
	e.selectDisplay(1)
	for i := 0; i < 5; i++ {
		e.moveDisplay(100, 0)
	}
	if err := e.validate(); err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("expected disconnected invalidity, got %v", err)
	}
	if _, err := e.saveBytes(); err == nil {
		t.Fatal("disconnected arrangement saved")
	}
}

func TestEditorSaveBytesRoundTrip(t *testing.T) {
	e := newTestEditor(t)
	if _, err := e.saveBytes(); err == nil {
		t.Fatal("clean editor saved without changes")
	}
	if err := e.selectEdge(desk.Left); err != nil { // right of source overlaps the fixture target
		t.Fatal(err)
	}
	if err := e.addDisplay(); err != nil {
		t.Fatal(err)
	}
	e.resizePixels(10, -10)
	data, err := e.saveBytes()
	if err != nil {
		t.Fatalf("valid dirty arrangement refused to save: %v", err)
	}
	decoded, err := desk.Decode(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("saved bytes fail the strict decoder: %v", err)
	}
	if !equalDeskLayout(decoded, e.layout) {
		t.Fatal("saved bytes do not round-trip to the edited arrangement")
	}
	if _, err := desk.Compile(decoded); err != nil {
		t.Fatalf("saved bytes fail compile: %v", err)
	}
	if len(data) > 65536 {
		t.Fatal("saved document exceeds the decoder bound")
	}
}

func TestEditorUndoLimit(t *testing.T) {
	e := newTestEditor(t)
	for i := 0; i < deskEditUndoLimit+10; i++ {
		if !e.moveDisplay(1, 0) {
			t.Fatal("move refused")
		}
	}
	if len(e.undo) != deskEditUndoLimit {
		t.Fatalf("undo depth %d, want cap %d", len(e.undo), deskEditUndoLimit)
	}
	// The oldest entries were dropped: full undo cannot reach the original.
	for len(e.undo) > 0 {
		e.undoEdit()
	}
	if !e.dirty() {
		t.Fatal("bounded undo wrongly claimed to restore the original")
	}
}

func TestEditorSelectionWrapsAndClamps(t *testing.T) {
	e := newTestEditor(t)
	e.selectDisplay(-1)
	if e.selected != 1 {
		t.Fatalf("wrap-back selected %d, want 1", e.selected)
	}
	e.selectDisplay(1)
	if e.selected != 0 {
		t.Fatalf("wrap-forward selected %d, want 0", e.selected)
	}
	if err := e.addDisplay(); err != nil {
		t.Fatal(err)
	}
	e.selected = 2
	if err := e.removeDisplay(); err != nil {
		t.Fatal(err)
	}
	if e.selected != 1 {
		t.Fatalf("selection after tail removal %d, want 1", e.selected)
	}
}

func TestEditorHitDisplayMatchesInspectorRule(t *testing.T) {
	e := newTestEditor(t)
	rects, err := e.rectangles(840, 230)
	if err != nil {
		t.Fatal(err)
	}
	if len(rects) != 2 {
		t.Fatal("fixture projection missing displays")
	}
	r := rects[1]
	if got := e.hitDisplay(840, 230, r.x+r.w/2, r.y+r.h/2); got != 1 {
		t.Fatalf("unambiguous center hit gave %d, want 1", got)
	}
	if got := e.hitDisplay(840, 230, r.x+r.w, r.y+r.h); got != -1 {
		t.Fatalf("outside-corner hit gave %d, want -1", got)
	}
	if got := e.hitDisplay(840, 230, -1, -1); got != -1 {
		t.Fatalf("outside hit gave %d, want -1", got)
	}
}

func TestEditorOriginalIsolation(t *testing.T) {
	layout := editFixtureLayout()
	e, err := newDeskLayoutEditor(layout)
	if err != nil {
		t.Fatal(err)
	}
	e.moveDisplay(5, 5)
	if layout.Displays[0].X != -100 {
		t.Fatal("editing mutated the caller's layout slice")
	}
	if e.original.Displays[0].X != -100 {
		t.Fatal("editing mutated the original snapshot")
	}
}
