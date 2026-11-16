package control

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestDeviceNameCollisionKeepsValidatedBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	network, err := st.CreateNetwork("shared")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNetwork("separate"); err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("a", 29) + "-ab" // truncation can expose a trailing hyphen
	seen := map[string]bool{}
	var first *netmap.JoinResponse
	for i := 0; i < 15; i++ {
		raw := make([]byte, 32)
		raw[0] = byte(i)
		joined, err := st.Join(netmap.JoinRequest{Network: "shared", Name: want, PubKey: base64.StdEncoding.EncodeToString(raw)})
		if err != nil {
			t.Fatal(err)
		}
		n := st.data.Nodes[joined.NodeID]
		if len(n.Name) > 32 || !nameRE.MatchString(n.Name) || seen[n.Name] || n.NetworkID != network.ID {
			t.Fatalf("invalid collision name: %q", n.Name)
		}
		seen[n.Name] = true
		if i == 0 {
			first = joined
		}
	}
	// Network scope is independent, and a saved identity rejoin keeps its name.
	raw := make([]byte, 32)
	separate, err := st.Join(netmap.JoinRequest{Network: "separate", Name: want, PubKey: base64.StdEncoding.EncodeToString(raw)})
	if err != nil || st.data.Nodes[separate.NodeID].Name != want {
		t.Fatal("name collision crossed networks", err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := st.Join(netmap.JoinRequest{Network: "shared", Name: "different", PubKey: base64.StdEncoding.EncodeToString(raw), NodeID: first.NodeID, NodeToken: first.NodeToken})
	if err != nil || joined.NodeID != first.NodeID || st.data.Nodes[joined.NodeID].Name != want {
		t.Fatal("rejoin changed saved name", err)
	}
	// A failed collision enrollment must leave names, identity and allocation intact.
	before, _ := json.Marshal(st.data)
	st.path = filepath.Join(path, "invalid")
	raw[0] = 99
	if _, err := st.Join(netmap.JoinRequest{Network: "shared", Name: want, PubKey: base64.StdEncoding.EncodeToString(raw)}); err == nil {
		t.Fatal("failed persistence accepted")
	}
	after, _ := json.Marshal(st.data)
	if string(before) != string(after) {
		t.Fatal("failed collision enrollment mutated state")
	}
}
