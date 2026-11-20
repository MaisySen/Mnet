package main

import (
	"fintex.au/mnet/internal/daemon"
	"strings"
	"testing"
	"time"
)

func TestDeviceSurfaceStatesAndWorstCasePageFits(t *testing.T) {
	for _, state := range []string{"up", "down", "waiting-approval", "key-expired", "joining"} {
		st := &daemon.Status{Status: state, NodeName: strings.Repeat("W", 32), IPv4: "100.127.255.254", Peers: make([]daemon.PeerStatus, 16)}
		for i := range st.Peers {
			st.Peers[i] = daemon.PeerStatus{Name: strings.Repeat("W", 32), IPv4: "100.127.255.254", Online: true, HandshakeState: "recent", LastHandshake: 9223372036854775807}
		}
		for _, page := range []int{0, 1} {
			if _, _, err := renderDevicesSurface(st, page, time.Now(), "Current local evidence unavailable. Refresh to try again.", [4]bool{true, true, true, true}, 0); err != nil {
				t.Fatal(state, err)
			}
		}
	}
	if _, _, err := renderDevicesSurface(nil, 0, time.Time{}, "Local setup is incomplete. Use the same --state directory as mnetd.", [4]bool{true, false, false, true}, -1); err != nil {
		t.Fatal(err)
	}
}
