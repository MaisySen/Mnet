package control

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestFailedPersistenceDoesNotChangeLiveState(t *testing.T) {
	for _, operation := range []string{"network", "join", "approve", "approve-as", "approve-identity", "expire", "endpoints", "acl", "delete-node", "create-key", "delete-key"} {
		t.Run(operation, func(t *testing.T) {
			s, err := openTestStore(t, filepath.Join(t.TempDir(), "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			network, err := s.CreateNetwork("home")
			if err != nil {
				t.Fatal(err)
			}
			first, err := s.Join(netmap.JoinRequest{Network: "home", Name: "owner", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
			if err != nil {
				t.Fatal(err)
			}
			key, err := s.CreateAuthKey(network.ID, "fixture", true, 0)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(s.data)
			version := s.Version()
			// An existing regular file cannot also become the parent directory.
			s.path = filepath.Join(s.path, "not-a-directory", "state.json")
			switch operation {
			case "network":
				_, err = s.CreateNetwork("second")
			case "join":
				pk := make([]byte, 32)
				pk[0] = 1
				_, err = s.Join(netmap.JoinRequest{Network: "home", Name: "second", PubKey: base64.StdEncoding.EncodeToString(pk), AuthKey: key.Key})
			case "approve":
				err = s.ApproveNode(first.NodeID)
			case "approve-as":
				err = s.ApproveNodeAs(first.NodeID, "fixture-user")
			case "approve-identity":
				err = s.ApproveNodeIdentity(first.NodeID, "display@example.invalid", "https://issuer.invalid", "verified-subject")
			case "expire":
				err = s.ExpireNode(first.NodeID)
			case "endpoints":
				err = s.SetEndpoints(first.NodeID, []string{"192.0.2.1:12345"})
			case "acl":
				err = s.SetACL(network.ID, &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "*", Dst: "*"}}})
			case "delete-node":
				err = s.DeleteNode(first.NodeID)
			case "create-key":
				_, err = s.CreateAuthKey(network.ID, "second", true, 0)
			case "delete-key":
				err = s.DeleteAuthKey(key.ID)
			}
			if err == nil {
				t.Fatal("save unexpectedly succeeded")
			}
			after, _ := json.Marshal(s.data)
			if string(before) != string(after) {
				t.Fatal("failed operation changed live authorization or state")
			}
			if s.Version() != version {
				t.Fatal("failed operation published map change")
			}
		})
	}
}

func TestFailedExistingRejoinRestoresNodeAndKeyUse(t *testing.T) {
	s, err := openTestStore(t, filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := s.CreateNetwork("home")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Join(netmap.JoinRequest{Network: "home", Name: "owner", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAuthKey(network.ID, "fixture", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(s.data)
	s.path = filepath.Join(s.path, "bad-parent")
	_, err = s.Join(netmap.JoinRequest{Network: "home", Name: "owner", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), NodeID: first.NodeID, NodeToken: first.NodeToken, AuthKey: key.Key, Routes: []string{"10.0.0.0/24"}, ExitNode: true})
	if err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	after, _ := json.Marshal(s.data)
	if string(before) != string(after) {
		t.Fatal("failed rejoin changed state")
	}
	if s.data.AuthKeys[key.ID].Uses != 0 {
		t.Fatal("failed rejoin consumed one-time enrollment key")
	}
}

func TestPersistencePrivateAtomicFileAndFailedRenameCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateNetwork("home"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions %v", info.Mode())
	}
	// A stale/symlinked predictable .tmp file must not be followed.
	sentinel := filepath.Join(dir, "sentinel")
	if err = os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(sentinel, path+".tmp"); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err = s.CreateNetwork("second"); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(sentinel)
	if err != nil || string(bytes) != "unchanged" {
		t.Fatal("predictable temporary symlink target changed")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := openTestStore(t, path)
	if err != nil || len(loaded.Networks()) != 2 {
		t.Fatal("persisted state could not be reopened")
	}
	s = loaded
	// Force rename failure after write+sync by making destination a directory.
	destination := filepath.Join(dir, "directory")
	if err = os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	s.path = destination
	if _, err = s.CreateNetwork("failed"); err == nil {
		t.Fatal("rename to directory succeeded")
	}
	if len(s.Networks()) != 2 {
		t.Fatal("rename failure changed live state")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".mnet-state-*"))
	if err != nil || len(matches) != 0 {
		t.Fatal("private temporary file leaked on success/failure")
	}
}

func TestApprovalIdentityAndVersionPersistTogether(t *testing.T) {
	s, err := openTestStore(t, filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateNetwork("home"); err != nil {
		t.Fatal(err)
	}
	first, err := s.Join(netmap.JoinRequest{Network: "home", Name: "owner", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	before := s.Version()
	if err = s.ApproveNodeAs(first.NodeID, "verified-subject"); err != nil {
		t.Fatal(err)
	}
	if s.Version() != before+1 {
		t.Fatal("approval did not publish exactly one change")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := openTestStore(t, s.path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := loaded.AuthorizeNode(first.NodeID, first.NodeToken)
	if err != nil || !n.Approved || n.User != "verified-subject" {
		t.Fatal("approval and identity did not persist together")
	}
}

func TestAdministrativeResultsOwnMutableState(t *testing.T) {
	s, err := openTestStore(t, filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.CreateNetwork("home")
	if err != nil {
		t.Fatal(err)
	}
	originalID := n.ID
	n.Name = "changed-without-save"
	if s.Network(originalID).Name != "home" {
		t.Fatal("CreateNetwork returned mutable store pointer")
	}
	if err = s.SetACL(originalID, &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "*", Dst: "*"}}}); err != nil {
		t.Fatal(err)
	}
	networks := s.Networks()
	networks[0].ACL.Rules[0].Src = "changed"
	if s.Network(originalID).ACL.Rules[0].Src != "*" {
		t.Fatal("Networks returned mutable policy")
	}
	key, err := s.CreateAuthKey(originalID, "fixture", true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	key.Note = "changed"
	*key.ExpiresAt = time.Time{}
	snap := s.Snapshot()
	if snap.AuthKeys[0].Note != "fixture" || snap.AuthKeys[0].ExpiresAt.IsZero() {
		t.Fatal("CreateAuthKey returned mutable state")
	}
	*snap.AuthKeys[0].ExpiresAt = time.Time{}
	if s.Snapshot().AuthKeys[0].ExpiresAt.IsZero() {
		t.Fatal("Snapshot returned mutable key expiry")
	}
}
