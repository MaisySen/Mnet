package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

// Test-only fixture data in the real Store. Not a permit constructor; no
// production code can invoke this function or create an authenticated grant.
func registrarStorageFixture(t *testing.T) *Store {
	t.Helper()
	s, err := openTestStore(t, filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := s.CreateNetwork("fixture")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Millisecond)
	expiry := at.Add(time.Hour)
	r := &registrarRecord{NetworkID: network.ID, DevicePublicKey: "CQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQk=", NodeID: "node-registrar-fixture", CredentialDigest: registrarContractDigest(2), Phase: "active", Scope: registrarScope{Issuer: "https://issuer.invalid/path", RegistrarOrigin: "https://registrar.invalid", Audience: "fixture-audience", ServiceID: "fixture-service", WorkspaceID: "fixture-workspace", MappingRevision: "fixture-r1", HandoffID: registrarContractDigest(3), AttemptID: registrarContractDigest(4), ReserveID: registrarContractDigest(5)}, DescriptorDigest: registrarContractDigest(6), DescriptorEncodingVersion: 1, ReceiptEncodingVersion: 1, ReceiptID: registrarContractDigest(7), CommittedAtMS: at.UnixMilli(), AccessExpiresAtMS: expiry.UnixMilli()}
	r.ReceiptDigest = registrarReceiptDigest(*r)
	s.data.Registrar = &registrarState{Version: 1, Records: map[string]*registrarRecord{registrarClaimKey(r.NetworkID, r.DevicePublicKey): r}}
	s.data.Nodes[r.NodeID] = &netmap.Node{ID: r.NodeID, NetworkID: r.NetworkID, Name: "fixture", PubKey: r.DevicePublicKey, IPv4: "100.64.0.1", Approved: true, CreatedAt: at, LastSeen: at, KeyExpiry: &expiry}
	s.data.NextIP[network.ID] = 1
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	return s
}
func registrarFixtureRecord(s *Store) *registrarRecord {
	for _, r := range s.data.Registrar.Records {
		return r
	}
	panic("missing registrar fixture")
}
func reopenRegistrarFixture(t *testing.T, s *Store) *Store {
	t.Helper()
	path := s.path
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestRegistrarStorageClaimedNodeRejectsLegacyEntryPoints(t *testing.T) {
	s := reopenRegistrarFixture(t, registrarStorageFixture(t))
	r := registrarFixtureRecord(s)
	key, err := s.CreateAuthKey(r.NetworkID, "independent reusable", true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"join", "approve", "approve-as", "approve-identity", "legacy-auth"} {
		before, _ := json.Marshal(s.data)
		diskBefore, _ := os.ReadFile(s.path)
		version := s.Version()
		var err error
		switch action {
		case "join":
			_, err = s.Join(netmap.JoinRequest{Network: r.NetworkID, Name: "takeover", PubKey: r.DevicePublicKey, NodeID: r.NodeID, NodeToken: "pretend-correct", AuthKey: key.Key, ExitNode: true, Routes: []string{"0.0.0.0/0"}})
		case "approve":
			err = s.ApproveNode(r.NodeID)
		case "approve-as":
			err = s.ApproveNodeAs(r.NodeID, "pretend-owner")
		case "approve-identity":
			err = s.ApproveNodeIdentity(r.NodeID, "display", "https://issuer.invalid/path", "pretend-subject")
		case "legacy-auth":
			s.data.NodeTokens[r.NodeID] = "pretend-correct"
			_, err = s.AuthorizeNode(r.NodeID, "pretend-correct")
			delete(s.data.NodeTokens, r.NodeID)
		}
		if !errors.Is(err, ErrUnauthorized) {
			t.Errorf("%s did not reject legacy authority: %v", action, err)
		}
		after, _ := json.Marshal(s.data)
		diskAfter, _ := os.ReadFile(s.path)
		if !bytes.Equal(before, after) || !bytes.Equal(diskBefore, diskAfter) || s.Version() != version {
			t.Fatalf("%s mutated claimed identity", action)
		}
	}
}

func TestRegistrarStorageRevocationAtomicityAndRetainedReceipt(t *testing.T) {
	for _, operation := range []string{"expire", "delete"} {
		for _, failure := range []string{"none", "before-publish", "after-publish"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				s := registrarStorageFixture(t)
				r := registrarFixtureRecord(s)
				receipt := r.ReceiptDigest
				network, key := r.NetworkID, r.DevicePublicKey
				before, _ := json.Marshal(s.data)
				version := s.Version()
				real := s.fs
				if failure == "before-publish" {
					s.fs.rename = func(string, string) error { return errInjectedStoreIO }
				}
				if failure == "after-publish" {
					s.fs.syncDir = func(path string) error {
						if err := real.syncDir(path); err != nil {
							return err
						}
						return errInjectedStoreIO
					}
				}
				var err error
				if operation == "expire" {
					err = s.ExpireNode(r.NodeID)
				} else {
					err = s.DeleteNode(r.NodeID)
				}
				if failure == "none" && err != nil {
					t.Fatal(err)
				}
				if failure != "none" && err == nil {
					t.Fatal("injected persistence error accepted")
				}
				if failure == "before-publish" {
					after, _ := json.Marshal(s.data)
					if !bytes.Equal(before, after) || s.Version() != version {
						t.Fatal("failed revoke did not restore node/claim/cursor")
					}
				} else {
					if r.Phase != "revoked" || r.ReceiptDigest != receipt {
						t.Fatal("revocation erased or changed immutable receipt")
					}
					if operation == "delete" {
						if s.data.Nodes[r.NodeID] != nil {
							t.Fatal("deleted node retained")
						}
						if _, ok := s.data.NodeTokens[r.NodeID]; ok {
							t.Fatal("delete fabricated legacy token")
						}
					}
					if failure == "after-publish" {
						if !errors.Is(err, ErrStoreUncertain) || s.Version() != version {
							t.Fatal("uncertain revocation published success")
						}
					} else if s.Version() != version+1 {
						t.Fatal("successful revocation did not publish once")
					}
					if s.RelayAllowed(r.NodeID, key) {
						t.Fatal("revoked relay allowed")
					}
					if _, err := s.Map(r.NodeID); err == nil {
						t.Fatal("revoked map served")
					}
				}
				s = reopenRegistrarFixture(t, s)
				retained := s.registrarClaimLocked(network, key)
				if retained == nil || retained.ReceiptDigest != receipt {
					t.Fatal("reopen lost receipt/claim")
				}
				expected := "revoked"
				if failure == "before-publish" {
					expected = "active"
				}
				if retained.Phase != expected {
					t.Fatal("reopen recovered partial revocation")
				}
			})
		}
	}
}

func TestRegistrarStorageRejectsCorruptSemanticsWithoutRewriting(t *testing.T) {
	for _, kind := range []string{"receipt-digest", "missing-node", "mixed-token", "mixed-provenance", "wrong-key-index", "alternate-node", "wrong-network", "bad-expiry", "duplicate-attempt", "credential-format", "encoding-version"} {
		t.Run(kind, func(t *testing.T) {
			s := registrarStorageFixture(t)
			r := registrarFixtureRecord(s)
			switch kind {
			case "receipt-digest":
				r.ReceiptDigest = registrarContractDigest(1)
			case "missing-node":
				delete(s.data.Nodes, r.NodeID)
			case "mixed-token":
				s.data.NodeTokens[r.NodeID] = "must-not-load"
			case "mixed-provenance":
				s.data.ApprovalIdentities = map[string]ApprovalIdentity{r.NodeID: {Issuer: r.Scope.Issuer, Subject: "not-verified"}}
			case "wrong-key-index":
				delete(s.data.Registrar.Records, registrarClaimKey(r.NetworkID, r.DevicePublicKey))
				s.data.Registrar.Records["not-canonical"] = r
			case "alternate-node":
				n := cloneNode(s.data.Nodes[r.NodeID])
				n.ID = "node-shadow"
				s.data.Nodes[n.ID] = &n
			case "wrong-network":
				s.data.Nodes[r.NodeID].NetworkID = "elsewhere"
			case "bad-expiry":
				r.AccessExpiresAtMS = r.CommittedAtMS
				r.ReceiptDigest = registrarReceiptDigest(*r)
			case "duplicate-attempt":
				copy := *r
				copy.NodeID = "node-other"
				copy.DevicePublicKey = "CgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgo="
				copy.Phase = "revoked"
				copy.ReceiptID = registrarContractDigest(20)
				copy.ReceiptDigest = registrarReceiptDigest(copy)
				s.data.Registrar.Records[registrarClaimKey(copy.NetworkID, copy.DevicePublicKey)] = &copy
			case "credential-format":
				r.CredentialDigest = "not-a-digest"
				r.ReceiptDigest = registrarReceiptDigest(*r)
			case "encoding-version":
				r.ReceiptEncodingVersion = 2
			}
			original, err := json.Marshal(s.data)
			if err != nil {
				t.Fatal(err)
			}
			path := s.path
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(path)
			if reopened != nil {
				reopened.Close()
			}
			if err == nil || reopened != nil {
				t.Fatal("corrupt authority record accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("corrupt authority record rewritten", err)
			}
		})
	}
}
