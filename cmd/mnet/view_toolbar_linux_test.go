//go:build linux

package main

import (
	"fintex.au/mnet/internal/session"
	"testing"
)

func TestViewerToolbarIsLocalAndRequiresSamePressTarget(t *testing.T) {
	v := pointerFixture()
	endX := v.width - 64
	if end, err := v.toolButton(1, false, endX, 30); end || err != nil {
		t.Fatal("unpressed release ended", err)
	}
	v.toolButton(1, true, endX, 30)
	if end, _ := v.toolButton(1, false, endX, 60); end {
		t.Fatal("outside release ended")
	}
	v.toolButton(1, true, endX, 30)
	if end, _ := v.toolButton(1, false, endX, 30); !end {
		t.Fatal("local End refused")
	}
	if len(v.PointerActions()) != 0 {
		t.Fatal("End queued remote input")
	}
	v.pointerPressed = 1
	v.toolButton(1, true, v.width-200, 30)
	if v.pointerPressed != 0 {
		t.Fatal("header kept remote pending click")
	}
}
func TestViewerToolbarPointerButtonsRespectSupportAndExactRevision(t *testing.T) {
	v := pointerFixture()
	x := v.width - 350
	if got := v.toolAt(x, 30); got != "" {
		t.Fatal("request enabled while allowed")
	}
	v.pointerGrant.State = "available"
	if got := v.toolAt(x, 30); got != "request" {
		t.Fatal(got)
	}
	v.toolButton(1, true, x, 30)
	v.pointerGrant.Revision++
	v.toolButton(1, false, x, 30)
	if len(v.PointerActions()) != 0 {
		t.Fatal("old press rebound")
	}
	v.toolButton(1, true, x, 30)
	v.toolButton(1, false, x, 30)
	actions := v.PointerActions()
	if len(actions) != 1 || actions[0].Kind != "request" || actions[0].Grant != v.pointerGrant {
		t.Fatal(actions)
	}
	v.pointerGrant.PointerV1 = false
	if v.toolAt(x, 30) != "" {
		t.Fatal("old host could request broader grant")
	}
	v.pointerGrant.State = "allowed"
	x = v.width - 200
	v.toolButton(1, true, x, 30)
	v.toolButton(1, false, x, 30)
	actions = v.PointerActions()
	if len(actions) != 1 || actions[0].Kind != "release" {
		t.Fatal(actions)
	}
}
func TestViewerToolbarNeverOverlapsFrameAndViewOnlyHasNoPointerControls(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		v := &xDesktopView{width: 960, pointer: pointer, pointerGrant: session.ControlStatus{State: "available", PointerV1: true}}
		tools := v.tools()
		if len(tools) != map[bool]int{false: 1, true: 3}[pointer] {
			t.Fatal(tools)
		}
		for _, b := range tools {
			if b.x < 0 || b.x+b.width > v.width {
				t.Fatal(b)
			}
			for _, y := range []int{-1, 0, 7, 52, 71, 72, 200} {
				if v.toolAt(b.x+5, y) != "" {
					t.Fatal("frame/header gap selected button", b, y)
				}
			}
		}
		for i, a := range tools {
			for j, b := range tools {
				if i != j && max(a.x, b.x) < min(a.x+a.width, b.x+b.width) {
					t.Fatal("overlap")
				}
			}
		}
	}
}
func TestViewerToolbarPermissionChangeRetiresOnlyInputIncreasingPress(t *testing.T) {
	before := session.ControlStatus{State: "available", Revision: 1}
	after := before
	after.Revision++
	if viewerToolGrantChanged("request", before, after) != "" {
		t.Fatal("old request press survived")
	}
	if viewerToolGrantChanged("end", before, after) != "end" {
		t.Fatal("local End disabled by remote status")
	}
}
