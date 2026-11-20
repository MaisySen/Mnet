package main

import (
	"bytes"
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
		for focus := -1; focus < 13; focus++ {
			if _, err := renderDesktopChooser(s, 0, focus, "expired", "Snapshot expired. Old peers and selection cleared. Refresh before choosing.", false, make([]bool, 13), now); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.clear()
	if _, err := renderDesktopChooser(s, 0, -1, "unavailable", "Local setup is incomplete. Use the same --state directory as mnetd.", false, make([]bool, 13), now); err != nil {
		t.Fatal(err)
	}
}

// A primary chooser must not expose a raw key or technical status. Disclosure
// shows the exact pin, and never revives one after snapshot expiry.
func TestDesktopChooserDetailsAreSecondaryAndNeverStale(t *testing.T) {
	now := time.Now()
	s := newDesktopPeerSelection()
	s.replace(desktopPeerSnapshot{Sampled: now, Peers: []daemon.SessionPeer{{Name: "My device", IPv4: "100.64.0.1", PublicKey: strings.Repeat("W", 43) + "="}}})
	s.selected = 0
	render := func(details bool, at time.Time) []byte {
		t.Helper()
		im, err := renderDesktopChooser(s, 0, -1, "ready", "Technical read detail", details, make([]bool, 13), at)
		if err != nil {
			t.Fatal(err)
		}
		return im.Pix
	}
	primary := render(false, now)
	detail := render(true, now)
	s.snapshot.Peers[0].PublicKey = strings.Repeat("B", 43) + "="
	if !bytes.Equal(primary, render(false, now)) {
		t.Fatal("key affected primary screen")
	}
	if bytes.Equal(detail, render(true, now)) {
		t.Fatal("exact key absent from details")
	}
	stale := render(true, now.Add(time.Minute))
	s.snapshot.Peers[0].PublicKey = strings.Repeat("C", 43) + "="
	if !bytes.Equal(stale, render(true, now.Add(time.Minute))) {
		t.Fatal("expired details retained key or disclosure")
	}
}
