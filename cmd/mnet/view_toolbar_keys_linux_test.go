//go:build linux

package main

import "testing"

func TestViewerToolbarKeyboardAlwaysLocalAndSkipsDisabled(t *testing.T) {
	v := pointerFixture()
	v.pointerGrant.State = "available"
	v.focusTool(false)
	if v.toolFocus != "end" {
		t.Fatal(v.toolFocus)
	}
	v.focusTool(false)
	if v.toolFocus != "request" {
		t.Fatal(v.toolFocus)
	}
	v.toolKeyPress(0xff0d)
	end, err := v.toolKeyRelease(0xff0d)
	actions := v.PointerActions()
	if end || err != nil || len(actions) != 1 || actions[0].Kind != "request" {
		t.Fatal(end, err, actions)
	}
	v.focusTool(false)
	if v.toolFocus != "end" {
		t.Fatal("disabled release focused", v.toolFocus)
	}
	v.focusTool(true)
	if v.toolFocus != "request" {
		t.Fatal(v.toolFocus)
	}
	v.pointer = false
	v.toolFocus = ""
	v.focusTool(true)
	if v.toolFocus != "end" {
		t.Fatal("view-only focus", v.toolFocus)
	}
	v.toolKeyPress(0x20)
	end, err = v.toolKeyRelease(0x20)
	if !end || err != nil || len(v.PointerActions()) != 0 {
		t.Fatal("view-only local End", end, err)
	}
}
func TestViewerToolbarKeyboardNeedsSameTargetKeyAndGrant(t *testing.T) {
	v := pointerFixture()
	v.pointerGrant.State = "available"
	v.toolFocus = "request"
	if end, _ := v.toolKeyRelease(0x20); end || len(v.PointerActions()) != 0 {
		t.Fatal("unpressed key activated")
	}
	v.toolKeyPress(0xff0d)
	v.toolKeyRelease(0x20)
	v.toolKeyRelease(0xff0d)
	if len(v.PointerActions()) != 0 {
		t.Fatal("different key accepted")
	}
	v.toolKeyPress(0xff0d)
	v.focusTool(false)
	v.toolKeyRelease(0xff0d)
	if len(v.PointerActions()) != 0 {
		t.Fatal("Tab rebound")
	}
	v.toolFocus = "request"
	v.toolKeyPress(0x20)
	v.pointerGrant.Revision++
	v.toolKeyRelease(0x20)
	if len(v.PointerActions()) != 0 {
		t.Fatal("grant rebound")
	}
	v.toolFocus = "request"
	v.toolKeyPress(0x20)
	v.cancelToolKey()
	v.toolKeyRelease(0x20)
	if len(v.PointerActions()) != 0 {
		t.Fatal("canceled key accepted")
	}
	v.toolFocus = "end"
	v.toolKeyPress(0x20)
	v.pointerGrant.Revision++
	if end, _ := v.toolKeyRelease(0x20); !end {
		t.Fatal("remote status prevented local End")
	}
}
