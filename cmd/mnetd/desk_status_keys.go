package main

// Local keyboard withdrawal only. This never approves or reconnects a run.
type deskStopKeys struct {
	focused bool
	pressed uint32
}

func (k *deskStopKeys) clear() { k.focused = false; k.pressed = 0 }
func (k *deskStopKeys) press(sym uint32) {
	if sym == 0xff09 || sym == 0xfe20 { // Tab / ISO_Left_Tab
		k.focused = true
		k.pressed = 0
		return
	}
	if k.focused && (sym == 0xff0d || sym == 0x20) {
		if k.pressed == 0 {
			k.pressed = sym
		}
		return
	}
	k.pressed = 0
}
func (k *deskStopKeys) release(sym uint32) bool {
	// Releasing a modifier or Tab must not activate or rebind another key.
	if sym != 0xff0d && sym != 0x20 {
		return false
	}
	stop := k.focused && k.pressed == sym
	k.pressed = 0
	return stop
}
