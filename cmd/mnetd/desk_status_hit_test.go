package main

import "testing"

func TestDeskStopTargetHasExact44PixelBand(t *testing.T) {
	for _, x := range []int{18, 360, 701} {
		for y := 240; y < 284; y++ {
			if !deskStopHit(1, x, y) {
				t.Fatal(x, y)
			}
		}
	}
	for _, p := range [][2]int{{17, 250}, {702, 250}, {18, 239}, {18, 284}} {
		if deskStopHit(1, p[0], p[1]) {
			t.Fatal(p)
		}
	}
	for _, b := range []byte{0, 2, 3, 4, 5} {
		if deskStopHit(b, 360, 260) {
			t.Fatal(b)
		}
	}
}
