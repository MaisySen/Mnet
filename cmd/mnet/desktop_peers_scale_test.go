package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
)

func desktopScaleFixture(count int) (*daemon.Status, []daemon.SessionPeer) {
	st := &daemon.Status{Status: "up"}
	peers := make([]daemon.SessionPeer, count)
	for i := range peers {
		name, ip := fmt.Sprintf("fixture-%04d", i), fmt.Sprintf("100.64.%d.%d", i/256, i%256)
		st.Peers = append(st.Peers, daemon.PeerStatus{Name: name, IPv4: ip})
		peers[i] = daemon.SessionPeer{Name: name, IPv4: ip, PublicKey: strings.Repeat("A", 43) + "="}
	}
	return st, peers
}
func BenchmarkDesktopEligiblePeers4096(b *testing.B) {
	st, peers := desktopScaleFixture(4096)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		got, err := desktopEligiblePeers(st, peers)
		if err != nil || len(got) != len(peers) {
			b.Fatal(len(got), err)
		}
	}
}

// A deterministic owner lets cancellation land after some valid rows have
// been accepted, without scheduler/sleep guesses in the cooperative-loop test.
type desktopSelectionOwner struct{ checks, stop int }

func (c *desktopSelectionOwner) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *desktopSelectionOwner) Done() <-chan struct{}       { return nil }
func (c *desktopSelectionOwner) Value(any) any               { return nil }
func (c *desktopSelectionOwner) Err() error {
	c.checks++
	if c.checks >= c.stop {
		return context.Canceled
	}
	return nil
}
func TestDesktopEligibleLargeSnapshotCancellationReturnsNoPartialRows(t *testing.T) {
	st, peers := desktopScaleFixture(4096)
	for _, stop := range []int{1, 2, 3, 20} {
		owner := &desktopSelectionOwner{stop: stop}
		got, err := desktopEligiblePeersContext(owner, st, peers)
		if err == nil || got != nil || owner.checks != stop {
			t.Fatal("cancellation did not stop before next row", stop, len(got), err, owner.checks)
		}
	}
	if got, err := desktopEligiblePeersContext(nil, st, peers); err == nil || got != nil {
		t.Fatal("nil owner accepted")
	}
	small, smallPeers := desktopScaleFixture(13)
	got, err := desktopEligiblePeersContext(context.Background(), small, smallPeers)
	if err != nil || len(got) != 13 {
		t.Fatal("valid snapshot refused", len(got), err)
	}
	for i, p := range got {
		if p != smallPeers[i] {
			t.Fatal("identity/order changed", i, p, smallPeers[i])
		}
	}
}
