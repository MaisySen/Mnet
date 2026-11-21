package control

// Deterministic legacy-image witness; this exact test can be copied unchanged
// to the accepted baseline. No tagged codec symbol is referenced.
import (
	"fintex.au/mnet/internal/netmap"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistrarNilCodecLegacyExactImage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	seed := []byte(`{"networks":{"0123456789abcdef":{"id":"0123456789abcdef","name":"legacy-golden","created_at":"2026-01-01T00:00:00Z"}},"nodes":{},"auth_keys":{},"node_tokens":{},"next_ip":{}}`)
	if e := os.WriteFile(path, seed, 0600); e != nil {
		t.Fatal(e)
	}
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SetACL("0123456789abcdef", &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "*", Dst: "*", Comment: "fixed witness"}}}); e != nil {
		t.Fatal(e)
	}
	s.Close()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if output := os.Getenv("MNET_TEST_LEGACY_GOLDEN_OUTPUT"); output != "" {
		if e = os.WriteFile(output, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
