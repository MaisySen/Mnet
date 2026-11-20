//go:build linux

package main

import "fintex.au/mnet/internal/session"

type viewerTool struct {
	kind, label string
	x, width    int
	enabled     bool
}

func (v *xDesktopView) tools() []viewerTool {
	tools := []viewerTool{{"end", "END (Esc)", v.width - 120, 112, true}}
	if v.pointer {
		state := v.pointerGrant.State
		request := v.pointerGrant.PointerV1 && (state == "available" || state == "declined" || state == "revoked")
		release := state == "pending" || state == "allowed" || state == "paused"
		tools = append(tools, viewerTool{"request", "ASK POINTER", v.width - 416, 140, request}, viewerTool{"release", "RELEASE", v.width - 268, 140, release})
	}
	return tools
}
func (v *xDesktopView) toolAt(x, y int) string {
	if y < 8 || y >= 52 {
		return ""
	}
	for _, b := range v.tools() {
		if b.enabled && x >= b.x && x < b.x+b.width {
			return b.kind
		}
	}
	return ""
}

// Press and release must name the same local target under the same permission
// revision. Header interaction never queues a remote click or held button.
func (v *xDesktopView) toolButton(button byte, down bool, x, y int) (bool, error) {
	if down {
		v.toolPressed = ""
		if y < viewerHeader {
			v.pointerPressed = 0
		}
		if button == 1 {
			v.toolPressed = v.toolAt(x, y)
			v.toolPressGrant = v.pointerGrant
		}
		return false, nil
	}
	pressed, grant := v.toolPressed, v.toolPressGrant
	v.toolPressed = ""
	if button != 1 || pressed == "" || pressed != v.toolAt(x, y) {
		return false, nil
	}
	if pressed == "end" {
		return true, nil
	}
	if grant != v.pointerGrant {
		return false, nil
	}
	return false, v.queuePointer(desktopPointerAction{Kind: pressed, Grant: grant})
}

func viewerToolGrantChanged(pressed string, before, after session.ControlStatus) string {
	if pressed != "end" && before != after {
		return ""
	}
	return pressed
}
