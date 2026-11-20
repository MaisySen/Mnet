//go:build linux

package main

import (
	"fintex.au/mnet/internal/session"
	"image"
	"testing"
)

func pointerFixture() *xDesktopView {
	return &xDesktopView{pointer: true, width: 1000, height: 872, image: image.NewRGBA(image.Rect(0, 0, 2000, 1000)), pointerGrant: session.ControlStatus{State: "allowed", Scope: session.ControlScopePointer, PointerV1: true, Session: "one", Request: "two", Revision: 3}}
}
func TestDesktopPointerMappingAndGrant(t *testing.T) {
	v := pointerFixture()
	x, y, ok := v.pointerPosition(500, 472)
	if !ok || x != 1000 || y != 500 {
		t.Fatal(x, y, ok)
	}
	for _, p := range [][2]int{{0, 72}, {500, 100}, {1000, 472}, {500, 872}} {
		if _, _, ok = v.pointerPosition(p[0], p[1]); ok {
			t.Fatal("letterbox/header/outside forwarded", p)
		}
	}
	v.viewport.one = true
	v.viewport.x = 600
	v.viewport.y = 100
	x, y, ok = v.pointerPosition(10, 82)
	if !ok || x != 610 || y != 110 {
		t.Fatal("one-to-one pan", x, y, ok)
	}
	v.pointerGrant.State = "paused"
	if _, _, ok = v.pointerPosition(10, 82); ok {
		t.Fatal("paused input")
	}
	v.pointerGrant.State = "allowed"
	v.pointer = false
	if _, _, ok = v.pointerPosition(10, 82); ok {
		t.Fatal("view-only input")
	}
}
func TestDesktopPointerClicksOnlyUnderExactPressRevision(t *testing.T) {
	v := pointerFixture()
	v.pointerButton(1, true, 500, 472)
	if len(v.PointerActions()) != 0 {
		t.Fatal("press forwarded remote hold")
	}
	v.pointerMotion(510, 472)
	if len(v.PointerActions()) != 0 {
		t.Fatal("drag forwarded")
	}
	v.pointerButton(1, false, 510, 472)
	if len(v.PointerActions()) != 0 {
		t.Fatal("drag converted to click")
	}
	v.pointerGrant.Revision++
	v.pointerButton(1, false, 500, 472)
	if len(v.PointerActions()) != 0 {
		t.Fatal("click rebound to later grant")
	}
	v.pointerButton(1, true, 500, 472)
	v.pointerButton(1, false, 500, 472)
	a := v.PointerActions()
	if len(a) != 1 || a[0].Input.Action != "click" || a[0].Input.Button != "left" {
		t.Fatal(a)
	}
	v.pointerMotion(500, 472)
	v.queuePointer(desktopPointerAction{Kind: "release"})
	for _, a := range v.PointerActions() {
		if a.Kind == "input" {
			t.Fatal("focus release retained queued input")
		}
	}
}

func TestDesktopPointerButtonsAndUnsupportedDrag(t *testing.T) {
	v := pointerFixture()
	for button, name := range map[byte]string{1: "left", 2: "middle", 3: "right"} {
		v.pointerButton(button, true, 500, 472)
		v.pointerButton(button, false, 500, 472)
		a := v.PointerActions()
		if len(a) != 1 || a[0].Input.Button != name || a[0].Input.Action != "click" {
			t.Fatal(a)
		}
	}
	v.pointerButton(1, true, 500, 472)
	v.pointerButton(1, false, 510, 472)
	if len(v.PointerActions()) != 0 {
		t.Fatal("unsupported movement turned into click")
	}
	v.pointerButton(1, true, 500, 472)
	v.pointerButton(1, false, 501, 472)
	if a := v.PointerActions(); len(a) != 1 {
		t.Fatal("small click jitter refused", a)
	}
}

func TestDesktopPointerNewPressCannotReuseOldClick(t *testing.T) {
	for _, tc := range []struct {
		name   string
		button byte
		x, y   int
	}{
		{"outside image", 8, 500, 100}, {"unsupported button", 8, 500, 472}, {"scroll", 4, 500, 472},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := pointerFixture()
			if err := v.pointerButton(1, true, 500, 472); err != nil {
				t.Fatal(err)
			}
			if err := v.pointerButton(tc.button, true, tc.x, tc.y); err != nil {
				t.Fatal(err)
			}
			if err := v.pointerButton(1, false, 500, 472); err != nil {
				t.Fatal(err)
			}
			for _, action := range v.PointerActions() {
				if action.Input.Action == "click" {
					t.Fatal("second press preserved old left click", action)
				}
			}
			v.pointerButton(tc.button, false, tc.x, tc.y)
			v.pointerButton(1, true, 500, 472)
			v.pointerButton(1, false, 500, 472)
			if actions := v.PointerActions(); len(actions) != 1 || actions[0].Input.Button != "left" {
				t.Fatal("fresh exact click unavailable", actions)
			}
		})
	}
}
