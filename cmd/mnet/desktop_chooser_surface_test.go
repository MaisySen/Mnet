package main

import (
	"fintex.au/mnet/internal/daemon"
	"strings"
	"testing"
	"time"
)

func TestDesktopChooserSurfaceFitsExactKeyAndStates(t *testing.T) {
	now := time.Now()
	s := newDesktopPeerSelection()
	s.replace(desktopPeerSnapshot{Sampled: now, Peers: []daemon.SessionPeer{{Name: strings.Repeat("W", 64), IPv4: "100.127.255.254", PublicKey: strings.Repeat("W", 43) + "="}}})
	for _, selected := range []int{-1, 0} {
		s.selected = selected
		for focus := -1; focus < 12; focus++ {
			if _, err := renderDesktopChooser(s, 0, focus, "Snapshot expired. Old peers and selection cleared. Refresh before choosing.", make([]bool, 12), now); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.clear()
	if _, err := renderDesktopChooser(s, 0, -1, "Local setup is incomplete. Use the same --state directory as mnetd.", make([]bool, 12), now); err != nil {
		t.Fatal(err)
	}
}
