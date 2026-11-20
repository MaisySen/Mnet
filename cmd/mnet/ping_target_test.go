package main

import (
	"fintex.au/mnet/internal/daemon"
	"testing"
)

func TestPingTargetRefusesAmbiguousMapSelector(t *testing.T) {
	st := &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{{Name: "same", IPv4: "100.64.0.2"}, {Name: "same", IPv4: "100.64.0.3"}}}
	if _, err := selectPingTarget("same", st); err == nil {
		t.Fatal("ambiguous selector silently chose a peer")
	}
}

func TestPingTargetCurrentUniqueAddressAndNoFlagText(t *testing.T) {
	valid := daemon.PeerStatus{Name: "peer", FQDN: "peer.home.mnet", IPv4: "100.64.0.2"}
	for _, selector := range []string{valid.Name, valid.FQDN, valid.IPv4} {
		got, err := selectPingTarget(selector, &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{valid}})
		if err != nil || got != valid.IPv4 {
			t.Fatal(selector, got, err)
		}
	}
	for _, state := range []string{"down", "preparing", "waiting-approval", "key-expired"} {
		if _, err := selectPingTarget(valid.Name, &daemon.Status{Status: state, Peers: []daemon.PeerStatus{valid}}); err == nil {
			t.Fatal("stale map allowed ping while", state)
		}
	}
	if _, err := selectPingTarget(valid.Name, nil); err == nil {
		t.Fatal("nil status")
	}
	for _, selector := range []string{"", " peer", "peer ", "missing", "--help"} {
		if _, err := selectPingTarget(selector, &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{valid}}); err == nil {
			t.Fatal("unmatched/ambiguous text accepted", selector)
		}
	}
	for _, address := range []string{"--help", "host.example", "100.064.0.2", "0.0.0.0", "255.255.255.255", "224.0.0.1", "::1", "::ffff:100.64.0.2", "100.64.0.2\n"} {
		p := valid
		p.IPv4 = address
		if _, err := selectPingTarget(p.Name, &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{p}}); err == nil {
			t.Fatal("invalid executable target accepted", address)
		}
	}
}
func TestPingTargetAliasAndAddressCollisionsRefused(t *testing.T) {
	a := daemon.PeerStatus{Name: "one", FQDN: "one.home.mnet", IPv4: "100.64.0.2"}
	b := daemon.PeerStatus{Name: "two", FQDN: "two.home.mnet", IPv4: "100.64.0.3"}
	for _, other := range []daemon.PeerStatus{
		{Name: "one", IPv4: b.IPv4},
		{Name: b.Name, FQDN: "one", IPv4: b.IPv4},
		{Name: b.Name, IPv4: a.IPv4},
		a,
	} {
		if _, err := selectPingTarget("one", &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{a, other}}); err == nil {
			t.Fatal("ambiguous target accepted", other)
		}
	}
	// Offline/unknown peer telemetry is a reason to diagnose, not a reason to
	// refuse an explicit ping. Only the local node's current Up state is required.
	if got, err := selectPingTarget("one", &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{a, b}}); err != nil || got != a.IPv4 {
		t.Fatal(got, err)
	}
}
