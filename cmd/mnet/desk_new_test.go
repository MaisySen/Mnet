package main

import (
	"testing"

	"fintex.au/mnet/internal/desk"
)

func TestStarterDeskLayoutCompiles(t *testing.T) {
	layout := starterDeskLayout(1920, 1080)
	compiled, err := desk.Compile(layout)
	if err != nil {
		t.Fatalf("starter layout must satisfy the exact saved-file gate: %v", err)
	}
	if len(layout.Displays) != 2 {
		t.Fatalf("starter displays = %d, want 2", len(layout.Displays))
	}
	local, next := layout.Displays[0], layout.Displays[1]
	if local.ID != "this-display" || local.Device != "this-computer" {
		t.Fatalf("local placeholder labels changed: %+v", local)
	}
	if next.ID != "next-display" || next.Device != "next-computer" {
		t.Fatalf("next placeholder labels changed: %+v", next)
	}
	if local.PixelWidth != 1920 || local.PixelHeight != 1080 {
		t.Fatalf("starter must carry the actual local pixel size, got %dx%d", local.PixelWidth, local.PixelHeight)
	}
	if local.X+local.Width != next.X || local.Y != next.Y || local.Height != next.Height {
		t.Fatalf("starter displays must share one full edge: %+v vs %+v", local, next)
	}
	links := compiled.Links()
	if len(links) != 2 {
		t.Fatalf("starter must link the shared edge in both directions: %+v", links)
	}
	if links[0].From != local.ID || links[0].Edge != desk.Right || links[0].To != next.ID || links[0].Start != 0 || links[0].End != 100 {
		t.Fatalf("starter must link the local display right edge to the next display: %+v", links)
	}
}

func TestStarterDeskLayoutBoundsPixelSize(t *testing.T) {
	for _, in := range [][2]int64{{0, 0}, {-5, -1}, {70000, 65536}, {65535, 1}} {
		layout := starterDeskLayout(in[0], in[1])
		if _, err := desk.Compile(layout); err != nil {
			t.Fatalf("bounded starter %v must still compile: %v", in, err)
		}
		local := layout.Displays[0]
		if local.PixelWidth < 1 || local.PixelWidth > 65535 || local.PixelHeight < 1 || local.PixelHeight > 65535 {
			t.Fatalf("starter %v produced out-of-bounds pixels %dx%d", in, local.PixelWidth, local.PixelHeight)
		}
	}
}

func TestStarterEditorStartsDirtyAndValid(t *testing.T) {
	e, err := newDeskLayoutStarterEditor(starterDeskLayout(1024, 768))
	if err != nil {
		t.Fatal(err)
	}
	if !e.dirty() {
		t.Fatal("a starter is unsaved new content and must start dirty")
	}
	if err := e.validate(); err != nil {
		t.Fatalf("starter must start valid so Save is available: %v", err)
	}
	if len(e.undo) != 0 {
		t.Fatal("a starter has nothing to undo")
	}
	if got := e.display().ID; got != "this-display" {
		t.Fatalf("starter selection = %q, want this-display", got)
	}
}

func TestStarterEditorRejectsInvalidLayout(t *testing.T) {
	bad := starterDeskLayout(1024, 768)
	bad.Displays = bad.Displays[:1]
	if _, err := newDeskLayoutStarterEditor(bad); err == nil {
		t.Fatal("starter editor must apply the exact Compile gate")
	}
}
