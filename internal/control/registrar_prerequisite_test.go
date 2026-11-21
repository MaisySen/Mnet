package control

import (
	"path/filepath"
	"testing"
)

// Characterization, NOT desired registrar acceptance: Open currently has no
// cross-instance ownership lease. A future enrollment consumer must not claim
// the per-instance mutex serializes another Store or process on the same path.
func TestRegistrarPrerequisiteCharacterizesUnownedStoreLostUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.CreateNetwork("base"); err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.CreateNetwork("first-only"); err != nil {
		t.Fatal(err)
	}
	if _, err = second.CreateNetwork("second-only"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, network := range reopened.data.Networks {
		names[network.Name] = true
	}
	if !names["base"] || !names["second-only"] || names["first-only"] {
		t.Fatalf("ownership behavior changed; revisit registrar prerequisite: %v", names)
	}
	// The first instance still believes its now-lost update exists. This is why
	// idempotent receipts cannot rely only on commitLocked plus a local mutex.
	if first.findNetworkLocked("first-only") == nil {
		t.Fatal("first instance unexpectedly refreshed disk state")
	}
}
