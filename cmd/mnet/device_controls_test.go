package main

import "testing"

func TestDeviceControlsTraverseEnabledAndActivateExactRelease(t *testing.T) {
	c := newDeviceControls()
	enabled := [4]bool{true, false, true, true}
	for _, want := range []int{0, 2, 3, 0} {
		c.advance(false, enabled)
		if c.focus != want {
			t.Fatal(c.focus, want)
		}
	}
	c.advance(true, enabled)
	if c.focus != 3 {
		t.Fatal("reverse wrap", c.focus)
	}
	c = newDeviceControls()
	c.advance(true, enabled)
	if c.focus != 3 {
		t.Fatal("reverse initial", c.focus)
	}
	for _, key := range []uint32{0xff0d, 0x20} {
		c.focus = 3
		if got := c.releaseKey(key, enabled); got != -1 {
			t.Fatal("unpressed release", got)
		}
		c.pressKey(key, enabled)
		if got := c.releaseKey(key, enabled); got != 3 {
			t.Fatal("exact release", got)
		}
		c.pressKey(key, enabled)
		c.cancel()
		if got := c.releaseKey(key, enabled); got != -1 {
			t.Fatal("focus loss activated", got)
		}
		c.pressKey(key, enabled)
		c.advance(false, enabled)
		if got := c.releaseKey(key, enabled); got != -1 {
			t.Fatal("tab rebound activation", got)
		}
	}
}
func TestDeviceControlsInvalidatePendingRefreshAcrossSnapshotAndAvailability(t *testing.T) {
	c := newDeviceControls()
	enabled := [4]bool{true, false, true, true}
	c.advance(false, enabled)
	c.pressKey(0x20, enabled)
	c.update(enabled, 1)
	if c.releaseKey(0x20, enabled) != -1 {
		t.Fatal("snapshot change retained key")
	}
	c.pressKey(0xff0d, enabled)
	enabled[0] = false
	c.update(enabled, 1)
	if c.focus != -1 || c.releaseKey(0xff0d, enabled) != -1 {
		t.Fatal("disabled refresh active")
	}
	c.advance(false, enabled)
	if c.focus != 2 {
		t.Fatal("disabled not skipped", c.focus)
	}
	if got := deviceControlEnabled(true, "down", 3, 19); got != [4]bool{false, false, false, true} {
		t.Fatal(got)
	}
	if got := deviceControlEnabled(false, "up", 2, 19); got != [4]bool{true, true, false, true} {
		t.Fatal(got)
	}
}

func TestDeviceControlsNeverRebindWrongReleaseOrDisabledFocus(t *testing.T) {
	c := newDeviceControls()
	all := [4]bool{true, true, true, true}
	c.advance(false, all)
	c.pressKey(0x20, all)
	if c.releaseKey(0xff0d, all) != -1 || c.releaseKey(0x20, all) != -1 {
		t.Fatal("different key release accepted")
	}
	c.pressKey(0xff0d, all)
	all[0] = false
	if c.releaseKey(0xff0d, all) != -1 {
		t.Fatal("disabled target activated before redraw")
	}
	c = newDeviceControls()
	c.advance(false, [4]bool{})
	if c.focus != -1 {
		t.Fatal("empty controls focused")
	}
	c.advance(true, [4]bool{})
	if c.focus != -1 {
		t.Fatal("empty reverse focused")
	}
}
