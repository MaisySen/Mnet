package main

import (
	"net"
	"testing"

	"fintex.au/mnet/internal/daemon"
)

func TestHeadlessViewerRequiresUniqueCurrentKeyedPeer(t *testing.T) {
	const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	source := &net.TCPAddr{IP: net.ParseIP("100.64.0.2"), Port: 1234}
	valid := daemon.SessionPeer{Name: "laptop", IPv4: "100.64.0.2", PublicKey: key}
	for _, tc := range []struct {
		name  string
		peers []daemon.SessionPeer
		claim string
		ok    bool
	}{
		{"valid", []daemon.SessionPeer{valid}, "LAPTOP", true},
		{"unknown", nil, "laptop", false},
		{"wrong-name", []daemon.SessionPeer{valid}, "other", false},
		{"missing-key", []daemon.SessionPeer{{Name: "laptop", IPv4: valid.IPv4}}, "laptop", false},
		{"duplicate-name", []daemon.SessionPeer{valid, {Name: "LAPTOP", IPv4: "100.64.0.3", PublicKey: key}}, "laptop", false},
		{"duplicate-source", []daemon.SessionPeer{valid, {Name: "other", IPv4: valid.IPv4, PublicKey: key}}, "laptop", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := daemon.MatchSessionPeer(tc.peers, tc.claim, source)
			if ok != tc.ok {
				t.Fatalf("accepted=%v", ok)
			}
		})
	}
}

func TestAllowedViewer(t *testing.T) {
	if !allowedViewer(nil, "anyone") {
		t.Fatal("empty list allows every verified peer")
	}
	allow := map[string]bool{"laptop": true}
	if !allowedViewer(allow, "LAPTOP") {
		t.Fatal("allow list is case insensitive")
	}
	if allowedViewer(allow, "phone") {
		t.Fatal("phone is not on the list")
	}
}
