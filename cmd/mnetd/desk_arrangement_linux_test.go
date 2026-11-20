//go:build linux

package main

import (
	"strings"
	"testing"

	"fintex.au/mnet/internal/desk"
)

func TestDeskReviewShowsExactArrangementNotHashOnly(t *testing.T) {
	binding, layout := deskReviewBindingFixture(t)
	for _, role := range []string{"primary", "recipient"} {
		_, text, err := deskReviewText(role, binding, layout)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Primary source: layout (0,0) 100x100; native 1024x768 px", "Recipient recipient: layout (100,0) 100x100; native 1024x768 px", "Primary to recipient: right edge, shared layout span [0,100)", "Layout units are not native pixels", binding.Layout} {
			if !strings.Contains(text, want) {
				t.Fatal("review omitted exact arrangement", role, want)
			}
		}
	}
}
func TestDeskReviewArrangementHandlesUnequalOffsetAndAllEdges(t *testing.T) {
	for _, tc := range []struct {
		x, y       int64
		edge, span string
	}{{100, 25, "right", "[25,100)"}, {-100, -25, "left", "[0,75)"}, {25, -100, "top", "[25,100)"}, {-25, 100, "bottom", "[0,75)"}} {
		binding, _ := deskReviewBindingFixture(t)
		layout, err := desk.Compile(desk.Layout{Version: 1, Displays: []desk.Display{{ID: "src", Device: binding.Primary, Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768}, {ID: "dst", Device: binding.Recipient, X: tc.x, Y: tc.y, Width: 100, Height: 100, PixelWidth: 1280, PixelHeight: 960}}})
		if err != nil {
			t.Fatal(err)
		}
		binding.Layout = layout.Fingerprint()
		binding.Display = "dst"
		text, err := deskArrangementReview(binding, layout)
		if err != nil || !strings.Contains(text, "native 1280x960 px") || !strings.Contains(text, tc.edge+" edge, shared layout span "+tc.span) {
			t.Fatal(tc, text, err)
		}
	}
}
func TestDeskReviewNeverTruncatesExtraOrWrongRoleDisplays(t *testing.T) {
	binding, layout := deskReviewBindingFixture(t)
	displays := layout.Displays()
	displays = append(displays, desk.Display{ID: "extra", Device: binding.Primary, X: -100, Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768})
	more, err := desk.Compile(desk.Layout{Version: 1, Displays: displays})
	if err != nil {
		t.Fatal(err)
	}
	binding.Layout = more.Fingerprint()
	if _, err = deskArrangementReview(binding, more); err == nil {
		t.Fatal("extra display hidden in approval")
	}
	displays = layout.Displays()
	displays[0].Device = "unrelated"
	wrong, err := desk.Compile(desk.Layout{Version: 1, Displays: displays})
	if err != nil {
		t.Fatal(err)
	}
	binding.Layout = wrong.Fingerprint()
	if _, err = deskArrangementReview(binding, wrong); err == nil {
		t.Fatal("source role mismatch approved")
	}
}
