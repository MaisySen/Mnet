package control

import (
	"encoding/json"
	"os"
	"testing"
)

// Real Store disk fixture, not a permit or transaction constructor. Failure
// baseline: state version2 is rejected before implementing strict v2 storage.
func TestRegistrarConsumedProofV2NamespaceRoundtrip(t *testing.T) {
	s := registrarStorageFixture(t)
	raw, err := json.Marshal(s.data)
	if err != nil {
		t.Fatal(err)
	}
	path := s.path
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data["registrar_state"], &state); err != nil {
		t.Fatal(err)
	}
	state["version"] = json.RawMessage(`2`)
	state["consumed_proofs"] = json.RawMessage(`{}`)
	data["registrar_state"], err = json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestStore(t, path)
	if err != nil {
		t.Fatal("strict v2 must preserve legacy receipt with empty consumption index", err)
	}
	if _, err := reopened.CreateNetwork("unrelated"); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]json.RawMessage
	if err := json.Unmarshal(saved, &after); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after["registrar_state"], &state); err != nil {
		t.Fatal(err)
	}
	if string(state["version"]) != "2" || string(state["consumed_proofs"]) != "{}" {
		t.Fatal("versioned consumed-proof history disappeared")
	}
}
