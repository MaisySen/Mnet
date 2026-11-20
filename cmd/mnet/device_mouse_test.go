package main

import "testing"

func TestDeviceMouseRequiresExactEnabledSnapshotGesture(t *testing.T) {
	all := [4]bool{true, true, true, true}
	for i, x := range []int{20, 240, 460, 680} {
		c := newDeviceControls()
		if c.releaseMouse(1, x, 448, all) != -1 {
			t.Fatal("unpressed release")
		}
		c.pressMouse(1, x, 448, all)
		if c.releaseMouse(1, x+199, 491, all) != i {
			t.Fatal("exact band")
		}
		for _, point := range [][2]int{{x - 1, 448}, {x + 200, 448}, {x, 447}, {x, 492}} {
			c.pressMouse(1, x, 448, all)
			if c.releaseMouse(1, point[0], point[1], all) != -1 {
				t.Fatal("mismatched target")
			}
			c.pressMouse(1, point[0], point[1], all)
			if c.releaseMouse(1, x, 448, all) != -1 {
				t.Fatal("outside press rebound")
			}
		}
		for _, button := range []byte{0, 2, 3, 4, 5} {
			c.pressMouse(button, x, 448, all)
			if c.releaseMouse(1, x, 448, all) != -1 {
				t.Fatal("wrong button")
			}
		}
		c.pressMouse(1, x, 448, all)
		c.update(all, 1)
		if c.releaseMouse(1, x, 448, all) != -1 {
			t.Fatal("new snapshot rebound")
		}
		c.pressMouse(1, x, 448, all)
		c.cancel()
		if c.releaseMouse(1, x, 448, all) != -1 {
			t.Fatal("focus loss rebound")
		}
		c.pressMouse(1, x, 448, all)
		disabled := all
		disabled[i] = false
		c.update(disabled, 1)
		if c.releaseMouse(1, x, 448, all) != -1 {
			t.Fatal("disabled then enabled rebound")
		}
		c.pressMouse(1, x, 448, all)
		c.advance(false, all)
		if c.releaseMouse(1, x, 448, all) != -1 {
			t.Fatal("Tab rebound")
		}
		c.pressMouse(1, x, 448, all)
		c.pressKey(0x20, all)
		if c.releaseMouse(1, x, 448, all) != -1 {
			t.Fatal("key gesture rebound")
		}
	}
}
