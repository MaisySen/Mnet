//go:build linux

package main

import (
	"errors"
	"fintex.au/mnet/internal/session"
)

func (v *xDesktopView) PointerStatus(status session.ControlStatus) error {
	if !v.pointer {
		return errors.New("view-only window cannot enable pointer")
	}
	if status == v.pointerGrant {
		return nil
	}
	v.pointerGrant = status
	v.pointerPressed = 0
	// An event observed under one revision cannot survive pause/revoke/resume.
	pending := v.pointerActions[:0]
	for _, a := range v.pointerActions {
		if a.Kind != "input" {
			pending = append(pending, a)
		}
	}
	v.pointerActions = pending
	v.state = "Pointer: " + status.State + ". Host-enforced pointer only. No keyboard or dragging."
	if !status.PointerV1 && status.State != "unavailable" {
		v.state = "View only: host lacks pointer-only permission support. F8 cannot request broader control."
	}
	return v.State(v.state)
}
func (v *xDesktopView) PointerActions() []desktopPointerAction {
	actions := v.pointerActions
	v.pointerActions = nil
	return actions
}
func (v *xDesktopView) queuePointer(action desktopPointerAction) error {
	if !v.pointer {
		return errors.New("view-only input refused")
	}
	if action.Kind == "release" {
		pending := v.pointerActions[:0]
		for _, a := range v.pointerActions {
			if a.Kind != "input" {
				pending = append(pending, a)
			}
		}
		v.pointerActions = pending
		v.pointerPressed = 0
	}
	if len(v.pointerActions) >= 64 {
		return errors.New("native pointer event queue full; end rather than replay")
	}
	if action.Kind == "input" && action.Input.Action == "move" && len(v.pointerActions) > 0 {
		last := &v.pointerActions[len(v.pointerActions)-1]
		if last.Kind == "input" && last.Input.Action == "move" && last.Grant == action.Grant {
			*last = action
			return nil
		}
	}
	v.pointerActions = append(v.pointerActions, action)
	return nil
}
func (v *xDesktopView) pointerPosition(x, y int) (int, int, bool) {
	if !v.pointer || v.image == nil || (v.pointerGrant.State != "allowed" || v.pointerGrant.Scope != session.ControlScopePointer) {
		return 0, 0, false
	}
	b := v.image.Bounds()
	region := v.viewport.region(b.Dx(), b.Dy(), v.width, v.height-viewerHeader)
	y -= viewerHeader
	if x < region.target.Min.X || y < region.target.Min.Y || x >= region.target.Max.X || y >= region.target.Max.Y || region.target.Empty() {
		return 0, 0, false
	}
	return region.source.Min.X + (x-region.target.Min.X)*region.source.Dx()/region.target.Dx(), region.source.Min.Y + (y-region.target.Min.Y)*region.source.Dy()/region.target.Dy(), true
}
func (v *xDesktopView) pointerMotion(x, y int) error {
	if v.pointerPressed != 0 {
		if abs(x-v.pointerPressX) > 4 || abs(y-v.pointerPressY) > 4 {
			v.pointerPressed = 0
		}
		return nil
	} // No drag, no remote button hold.
	px, py, ok := v.pointerPosition(x, y)
	if !ok {
		return nil
	}
	return v.queuePointer(desktopPointerAction{Kind: "input", Grant: v.pointerGrant, Input: session.InputEvent{Kind: "pointer", Action: "move", X: px, Y: py}})
}
func (v *xDesktopView) pointerButton(button byte, down bool, x, y int) error {
	px, py, ok := v.pointerPosition(x, y)
	if down {
		if !ok {
			return nil
		}
		if button >= 1 && button <= 3 {
			v.pointerPressed = button
			v.pointerPressX, v.pointerPressY = x, y
			v.pointerPressGrant = v.pointerGrant
			return nil
		}
		if button == 4 || button == 5 {
			if err := v.queuePointer(desktopPointerAction{Kind: "input", Grant: v.pointerGrant, Input: session.InputEvent{Kind: "pointer", Action: "move", X: px, Y: py}}); err != nil {
				return err
			}
			dy := 1
			if button == 4 {
				dy = -1
			}
			return v.queuePointer(desktopPointerAction{Kind: "input", Grant: v.pointerGrant, Input: session.InputEvent{Kind: "pointer", Action: "scroll", DY: dy}})
		}
		return nil
	}
	pressed, grant := v.pointerPressed, v.pointerPressGrant
	v.pointerPressed = 0
	if !ok || pressed != button || grant != v.pointerGrant || abs(x-v.pointerPressX) > 4 || abs(y-v.pointerPressY) > 4 {
		return nil
	}
	name := map[byte]string{1: "left", 2: "middle", 3: "right"}[button]
	if name == "" {
		return nil
	}
	return v.queuePointer(desktopPointerAction{Kind: "input", Grant: grant, Input: session.InputEvent{Kind: "pointer", Action: "click", Button: name, X: px, Y: py}})
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
