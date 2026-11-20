//go:build linux

package main

import (
	"context"
	"fintex.au/mnet/internal/desk"
	"strings"
	"testing"
)

func TestDeskStatusPhasesNeverImplyApprovalOrCompletion(t *testing.T) {
	for _, tc := range []struct {
		role, from, to string
		allowed        bool
	}{
		{"primary", "armed", "connecting", true}, {"primary", "connecting", "forwarding", true}, {"primary", "forwarding", "forwarding", true},
		{"recipient", "waiting", "ready", true}, {"recipient", "ready", "receiving", true}, {"recipient", "receiving", "receiving", true},
		{"primary", "armed", "forwarding", false}, {"recipient", "waiting", "receiving", false},
		{"primary", "forwarding", "armed", false}, {"recipient", "receiving", "waiting", false},
		{"primary", "armed", "approved", false}, {"recipient", "receiving", "clean-stop", false},
		{"primary", "armed", "receiving", false}, {"recipient", "waiting", "forwarding", false},
	} {
		if got := deskStatusTransition(tc.role, tc.from, tc.to); got != tc.allowed {
			t.Fatal(tc, got)
		}
	}
	for _, role := range []string{"primary", "recipient"} {
		for _, state := range []string{"approved", "clean-stop", "unknown"} {
			if _, err := deskStatusLine(role, state); err == nil {
				t.Fatal("invented authority/completion phase", role, state)
			}
		}
	}
}
func TestDeskStatusRefusesInvalidStartBeforeOpeningDisplay(t *testing.T) {
	good := desk.RunSpec{Role: "primary", Binding: desk.Binding{Run: strings.Repeat("a", 64), Recipient: strings.Repeat("b", 64)}}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := startDeskRunStatus(canceled, good, "armed"); err == nil {
		t.Fatal("retired status owner opened UI")
	}
	if _, err := startDeskRunStatus(context.Background(), good, "forwarding"); err == nil {
		t.Fatal("unstarted run claimed forwarding")
	}
	good.Binding.Run = "remote label"
	if _, err := startDeskRunStatus(context.Background(), good, "armed"); err == nil {
		t.Fatal("unvalidated binding entered local UI")
	}
}
