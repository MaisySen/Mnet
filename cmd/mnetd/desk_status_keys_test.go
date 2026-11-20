package main

import "testing"

func TestDeskStopKeyboardExactLocalRelease(t *testing.T) {
	for _, sym := range []uint32{0xff0d, 0x20} {
		for _, tab := range []uint32{0xff09, 0xfe20} {
			var k deskStopKeys
			k.press(sym)
			if k.release(sym) {
				t.Fatal("unfocused activation")
			}
			k.press(tab)
			if !k.focused || k.release(sym) {
				t.Fatal("unpressed activation")
			}
			k.press(sym)
			if !k.release(sym) || k.release(sym) {
				t.Fatal("exact release must be one-shot")
			}
			k.press(sym)
			k.clear()
			if k.release(sym) {
				t.Fatal("focus loss retained activation")
			}
			k.press(tab)
			k.press(sym)
			k.press(tab)
			if k.release(sym) {
				t.Fatal("Tab retained pending activation")
			}
			k.press(sym)
			k.press('a')
			if k.release(sym) {
				t.Fatal("other gesture retained activation")
			}
		}
	}
}
func TestDeskStopKeyboardCannotRebindPendingKey(t *testing.T) {
	var k deskStopKeys
	k.press(0xff09)
	k.press(0xff0d)
	k.press(0x20)
	if k.release(0x20) || k.release(0xff0d) {
		t.Fatal("pending Enter rebound to Space")
	}
	k.press(0x20)
	if k.release(0xff0d) || k.release(0x20) {
		t.Fatal("mismatched release activated")
	}
	k.press(0xff0d)
	k.release(0xff09)
	if !k.release(0xff0d) {
		t.Fatal("Tab keyup removed independently pressed Enter")
	}
}
