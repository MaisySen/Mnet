package control

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestApprovalIdentityPersistsWithoutPeerDisclosure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateNetwork("fixture"); err != nil {
		t.Fatal(err)
	}
	joined, err := store.Join(netmap.JoinRequest{Network: "fixture", Name: "one", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.NodeApprovalIdentity(joined.NodeID); ok {
		t.Fatal("unverified identity inferred")
	}
	for _, identity := range []ApprovalIdentity{{Issuer: "https://one.invalid", Subject: "stable-subject"}, {Issuer: "https://two.invalid", Subject: "stable-subject"}} {
		if err := store.ApproveNodeIdentity(joined.NodeID, "same-display@example.invalid", identity.Issuer, identity.Subject); err != nil {
			t.Fatal(err)
		}
		store, err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := store.NodeApprovalIdentity(joined.NodeID)
		if !ok || got != identity {
			t.Fatalf("issuer-bound provenance lost: %+v", got)
		}
		node := store.Snapshot().Nodes[0]
		raw, err := json.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), identity.Issuer) || strings.Contains(string(raw), identity.Subject) {
			t.Fatal("private principal added to peer-facing node")
		}
	}
	if err := store.ApproveNode(joined.NodeID); err != nil {
		t.Fatal(err)
	}
	if identity, ok := store.NodeApprovalIdentity(joined.NodeID); !ok || identity.Issuer != "https://two.invalid" {
		t.Fatal("manual approval changed verified provenance")
	}
	if err := store.DeleteNode(joined.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.NodeApprovalIdentity(joined.NodeID); ok {
		t.Fatal("deleted node retained principal")
	}
}

func TestApprovalIdentityDeleteRollbackAndLegacyUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateNetwork("fixture"); err != nil {
		t.Fatal(err)
	}
	joined, err := store.Join(netmap.JoinRequest{Network: "fixture", Name: "one", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveNodeAs(joined.NodeID, "legacy-label@example.invalid"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.NodeApprovalIdentity(joined.NodeID); ok {
		t.Fatal("legacy email upgraded into principal")
	}
	want := ApprovalIdentity{Issuer: "https://issuer.invalid", Subject: "verified-subject"}
	if err := store.ApproveNodeIdentity(joined.NodeID, "display", want.Issuer, want.Subject); err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(path, "not-a-directory", "state.json")
	if err := store.DeleteNode(joined.NodeID); err == nil {
		t.Fatal("failed persistence accepted")
	}
	if got, ok := store.NodeApprovalIdentity(joined.NodeID); !ok || got != want {
		t.Fatal("delete rollback lost principal")
	}
	if err := store.ApproveNodeIdentity(joined.NodeID, "changed", "https://other.invalid", "other"); err == nil {
		t.Fatal("identity update persisted unexpectedly")
	}
	if got, ok := store.NodeApprovalIdentity(joined.NodeID); !ok || got != want {
		t.Fatal("update rollback changed principal")
	}
}
