//go:build linux

package main

func (v *xDesktopView) cancelToolKey() { v.toolKey = 0; v.toolKeyTarget = "" }
func (v *xDesktopView) focusTool(reverse bool) {
	v.cancelToolKey()
	v.toolPressed = ""
	v.pointerPressed = 0
	tools := v.tools()
	index := -1
	for i, b := range tools {
		if b.kind == v.toolFocus {
			index = i
		}
	}
	step := 1
	if reverse {
		step = -1
		if index < 0 {
			index = 0
		}
	}
	for n := 0; n < len(tools); n++ {
		index = (index + step + len(tools)) % len(tools)
		if tools[index].enabled {
			v.toolFocus = tools[index].kind
			return
		}
	}
	v.toolFocus = ""
}
func (v *xDesktopView) toolKeyPress(key uint32) {
	v.cancelToolKey()
	v.toolPressed = ""
	v.pointerPressed = 0
	if key != 0xff0d && key != 0x20 {
		return
	}
	for _, b := range v.tools() {
		if b.enabled && b.kind == v.toolFocus {
			v.toolKey = key
			v.toolKeyTarget = b.kind
			v.toolKeyGrant = v.pointerGrant
			return
		}
	}
}
func (v *xDesktopView) toolKeyRelease(key uint32) (bool, error) {
	target, held, grant := v.toolKeyTarget, v.toolKey, v.toolKeyGrant
	v.cancelToolKey()
	if key != held || target == "" || target != v.toolFocus {
		return false, nil
	}
	for _, b := range v.tools() {
		if b.enabled && b.kind == target {
			if target == "end" {
				return true, nil
			}
			if grant != v.pointerGrant {
				return false, nil
			}
			return false, v.queuePointer(desktopPointerAction{Kind: target, Grant: grant})
		}
	}
	return false, nil
}
