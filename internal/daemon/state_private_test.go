package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRefusesLinkedIdentity(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"node_name":"original"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, statePath(dir)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := LoadState(dir); err == nil {
		t.Fatal("loaded linked identity")
	}
	if err := (&State{NodeName: "overwritten"}).Save(dir); err == nil {
		t.Fatal("saved over linked identity")
	}
	got, _ := os.ReadFile(outside)
	if string(got) != `{"node_name":"original"}` {
		t.Fatal("outside identity changed")
	}
}

func TestPrivateStateAtomicRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s := &State{NodeName: "first"}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	s.NodeName = "second"
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil || got.NodeName != "second" {
		t.Fatalf("roundtrip: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files: %v %v", entries, err)
	}
}
