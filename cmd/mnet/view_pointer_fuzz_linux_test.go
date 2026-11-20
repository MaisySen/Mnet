//go:build linux

package main

import (
	"image"
	"testing"

	"fintex.au/mnet/internal/session"
)

// Geometry only. No X server, transport, host grant or native input is created.
func FuzzDesktopPointerMappingAndRefusal(f *testing.F) {
	for _, raw := range [][]byte{{0}, {255}, {1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, {128, 64, 32, 16, 8}} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) == 0 || len(raw) > 128 {
			t.Skip()
		}
		at := func(i int) int { return int(raw[i%len(raw)]) }
		sw, sh := 1+at(0), 1+at(1)
		dw, dh := 1+at(2)*4, 1+at(3)*4
		v := &xDesktopView{pointer: true, width: dw, height: dh + viewerHeader, image: image.NewRGBA(image.Rect(0, 0, sw, sh)), pointerGrant: session.ControlStatus{State: "allowed", Scope: session.ControlScopePointer, PointerV1: true}}
		v.viewport.one = at(4)%2 == 1
		v.viewport.x = at(5)*3 - 256
		v.viewport.y = at(6)*3 - 256
		// Derive visible source/destination rectangles without production viewport
		// or fit helpers. Downstream input must follow precisely the painted region.
		width, height, sourceX, sourceY := dw, dh, 0, 0
		sourceWidth, sourceHeight := sw, sh
		if v.viewport.one {
			width = min(sw, dw)
			height = min(sh, dh)
			sourceWidth, sourceHeight = width, height
			sourceX = max(0, min(v.viewport.x, sw-width))
			sourceY = max(0, min(v.viewport.y, sh-height))
		} else if sw*dh > sh*dw {
			height = max(1, sh*dw/sw)
		} else {
			width = max(1, sw*dh/sh)
		}
		left, top := (dw-width)/2, (dh-height)/2+viewerHeader
		points := [][2]int{{left, top}, {left + width - 1, top + height - 1}, {left + width/2, top + height/2}, {left - 1, top}, {left, top - 1}, {left + width, top}, {left, top + height}, {-1, -1}, {dw, dh + viewerHeader}, {at(7) * 4, at(8) * 4}}
		for _, p := range points {
			inside := p[0] >= left && p[0] < left+width && p[1] >= top && p[1] < top+height
			x, y, ok := v.pointerPosition(p[0], p[1])
			if ok != inside {
				t.Fatal("letterbox/header mapping differs", sw, sh, dw, dh, p, ok, inside)
			}
			if ok && (x != sourceX+(p[0]-left)*sourceWidth/width || y != sourceY+(p[1]-top)*sourceHeight/height || x < 0 || y < 0 || x >= sw || y >= sh) {
				t.Fatal("input differs from received-pixel geometry", p, x, y)
			}
			for _, deny := range []struct {
				pointer      bool
				state, scope string
			}{{false, "allowed", session.ControlScopePointer}, {true, "paused", session.ControlScopePointer}, {true, "revoked", session.ControlScopePointer}, {true, "pending", session.ControlScopePointer}, {true, "allowed", ""}} {
				originalPointer, grant := v.pointer, v.pointerGrant
				v.pointer = deny.pointer
				v.pointerGrant.State = deny.state
				v.pointerGrant.Scope = deny.scope
				if _, _, accepted := v.pointerPosition(p[0], p[1]); accepted {
					t.Fatal("unavailable input geometry accepted", deny, p)
				}
				v.pointer, v.pointerGrant = originalPointer, grant
			}
		}
		if len(v.PointerActions()) != 0 {
			t.Fatal("geometry inspection queued input")
		}
	})
}
