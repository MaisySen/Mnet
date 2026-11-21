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

func TestChooserSetupGuidanceStates(t *testing.T) {
	cases := []struct {
		message, phase string
		peers          int
		wantTitle      string
	}{
		{errLocalDaemon.Error(), "unavailable", 0, "Mnet is not running"},
		{errLocalNetworkDown.Error(), "unavailable", 0, "This computer is not connected"},
		{errLocalSetup.Error(), "unavailable", 0, "Local setup is incomplete"},
		{errLocalAccess.Error(), "unavailable", 0, "Local access was refused"},
		{"", "ready", 0, "No devices yet"},
		{"", "ready", 3, "Local setup details"},
		{"local evidence unavailable", "unavailable", 0, "Local setup details"},
	}
	for _, c := range cases {
		title, line := chooserSetupGuidance(c.message, c.phase, c.peers)
		if title != c.wantTitle {
			t.Fatalf("guidance(%q, %q, %d) title = %q, want %q", c.message, c.phase, c.peers, title, c.wantTitle)
		}
		if line == "" || len(line) > 120 {
			t.Fatalf("guidance(%q) line missing or unbounded: %q", c.wantTitle, line)
		}
	}
	// Every line must fit the presentation bound so rendering never fails.
	for _, c := range cases {
		_, line := chooserSetupGuidance(c.message, c.phase, c.peers)
		if len(line) > 110 {
			t.Fatalf("line too long for the panel: %q", line)
		}
	}
}
