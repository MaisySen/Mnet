package main

import (
	"testing"

	"fintex.au/mnet/internal/daemon"
)

func TestBindViewer(t *testing.T) {
	peers := []daemon.PeerStatus{
		{Name: "laptop", IPv4: "100.64.0.2"},
		{Name: "phone", IPv4: "100.64.0.3"},
	}
	if err := bindViewer(peers, "laptop", "100.64.0.2"); err != nil {
		t.Fatalf("right name and IP: %v", err)
	}
	if err := bindViewer(peers, "LAPTOP", "100.64.0.2"); err != nil {
		t.Fatalf("case insensitive: %v", err)
	}
	if err := bindViewer(peers, "laptop", "100.64.0.3"); err == nil {
		t.Fatal("name claimed from another peer's IP must fail")
	}
	if err := bindViewer(peers, "stranger", "100.64.0.9"); err == nil {
		t.Fatal("unknown name must fail")
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
