package main

import (
	"fintex.au/mnet/internal/multies"
	"strings"
	"testing"
)

func TestMultiesSurfaceEveryStateAndDevicePageFits(t *testing.T) {
	for _, phase := range []string{"start", "preparing", "review", "waiting", "done", "stale", "failed"} {
		for _, page := range []int{0, 1} {
			list := multies.ApprovedDeviceList{Truncated: true, Devices: make([]multies.ApprovedDevice, 8)}
			for i := range list.Devices {
				list.Devices[i] = multies.ApprovedDevice{ID: strings.Repeat("A", 43), DeviceKey: strings.Repeat("W", 43), Platform: "windows"}
			}
			image, err := renderMultiesSurface(multiesSurface{phase: phase, key: strings.Repeat("W", 43), origin: "https://" + strings.Repeat("w", 64) + ".au", list: list, page: page, focus: 0, enabled: [4]bool{true, true, true, true}})
			if err != nil {
				t.Fatalf("%s page%d: %v", phase, page, err)
			}
			if image.Bounds().Dx() != 900 || image.Bounds().Dy() != 500 {
				t.Fatal("changed fixed surface")
			}
		}
	}
}
func TestMultiesSurfaceRefusesInvalidStateAndOverflow(t *testing.T) {
	for _, v := range []multiesSurface{{phase: "unknown"}, {phase: "start", origin: strings.Repeat("a", 76)}, {phase: "review", key: strings.Repeat("a", 44)}, {phase: "done", page: 1}} {
		if _, err := renderMultiesSurface(v); err == nil {
			t.Fatal("invalid surface accepted")
		}
	}
}
