package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"fintex.au/mnet/internal/enrollmentcontract"
)

// Creates DATA fixture only; not compiled into production and never verifies a
// principal, consent or proof. All mutations still save/reopen the actual Store.
func registrarConsumptionFixture(t *testing.T) *Store {
	t.Helper()
	s := registrarStorageFixture(t)
	r := registrarFixtureRecord(s)
	now := r.CommittedAtMS
	c := enrollmentcontract.Challenge{ID: registrarContractDigest(21), Nonce: registrarContractDigest(22), EphemeralPublicKey: registrarContractDigest(23), ExpiresAtMS: now + 20000, Binding: enrollmentcontract.Binding{PrincipalAuthority: "https://multies.invalid", PrincipalNamespace: enrollmentcontract.PrincipalNamespace, PrincipalID: "local-person", SessionBinding: registrarContractDigest(24), IssuerLabel: r.Scope.Issuer, RegistrarOrigin: r.Scope.RegistrarOrigin, Audience: r.Scope.Audience, ServiceID: r.Scope.ServiceID, WorkspaceID: r.Scope.WorkspaceID, NetworkID: r.NetworkID, MappingRevision: r.Scope.MappingRevision, HandoffID: r.Scope.HandoffID, AttemptID: r.Scope.AttemptID, ReserveID: r.Scope.ReserveID, DescriptorDigest: r.DescriptorDigest, DescriptorEncodingVersion: 1, CredentialDigest: r.CredentialDigest, CheckedAtMS: now - 1000, ReserveDeadlineMS: now + 60000, AccessUntilMS: now + 30000}}
	raw, err := base64.StdEncoding.DecodeString(r.DevicePublicKey)
	if err != nil {
		t.Fatal(err)
	}
	c.Binding.DevicePublicKey = base64.RawURLEncoding.EncodeToString(raw)
	r.AccessExpiresAtMS = c.Binding.AccessUntilMS
	expiry := time.UnixMilli(r.AccessExpiresAtMS).UTC()
	s.data.Nodes[r.NodeID].KeyExpiry = &expiry
	r.ReceiptEncodingVersion = 2
	r.Consumption = &registrarConsumption{Challenge: c, TranscriptDigest: registrarTranscriptDigest(c)}
	r.ReceiptDigest = registrarReceiptDigest(*r)
	s.data.Registrar.Version = 2
	s.data.Registrar.ConsumedProofs = map[string]string{registrarProofKey(c): registrarClaimKey(r.NetworkID, r.DevicePublicKey)}
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestRegistrarConsumedHistorySurvivesRevocationAndReopen(t *testing.T) {
	for _, action := range []string{"unrelated", "expire", "delete"} {
		t.Run(action, func(t *testing.T) {
			s := reopenRegistrarFixture(t, registrarConsumptionFixture(t))
			r := registrarFixtureRecord(s)
			consumption, _ := json.Marshal(r.Consumption)
			receipt := r.ReceiptDigest
			proofKey := registrarProofKey(r.Consumption.Challenge)
			claimKey := registrarClaimKey(r.NetworkID, r.DevicePublicKey)
			var err error
			switch action {
			case "expire":
				err = s.ExpireNode(r.NodeID)
			case "delete":
				err = s.DeleteNode(r.NodeID)
			default:
				_, err = s.CreateNetwork("unrelated")
			}
			if err != nil {
				t.Fatal(err)
			}
			s = reopenRegistrarFixture(t, s)
			r = registrarFixtureRecord(s)
			after, _ := json.Marshal(r.Consumption)
			if s.data.Registrar.Version != 2 || s.data.Registrar.ConsumedProofs[proofKey] != claimKey || !bytes.Equal(consumption, after) || r.ReceiptDigest != receipt {
				t.Fatal("consumed proof/history erased or changed")
			}
		})
	}
}
func TestRegistrarConsumedHistoryRejectsIndexOrBindingCorruption(t *testing.T) {
	for _, kind := range []string{"missing-index", "null-index", "dangling-index", "wrong-index", "duplicate-proof", "changed-transcript", "changed-principal", "changed-session", "changed-deadline", "downgrade", "v1-receipt-with-consumption"} {
		t.Run(kind, func(t *testing.T) {
			s := registrarConsumptionFixture(t)
			r := registrarFixtureRecord(s)
			state := s.data.Registrar
			proofKey := registrarProofKey(r.Consumption.Challenge)
			switch kind {
			case "missing-index":
				delete(state.ConsumedProofs, proofKey)
			case "null-index":
				state.ConsumedProofs = nil
			case "dangling-index":
				state.ConsumedProofs["unexpected"] = "absent"
			case "wrong-index":
				state.ConsumedProofs[proofKey] = "absent"
			case "duplicate-proof":
				copy := *r
				copy.NodeID = "node-other"
				copy.DevicePublicKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{25}, 32))
				copy.Phase = "revoked"
				copy.ReceiptID = registrarContractDigest(26)
				copy.ReceiptDigest = registrarReceiptDigest(copy)
				state.Records[registrarClaimKey(copy.NetworkID, copy.DevicePublicKey)] = &copy
			case "changed-transcript":
				r.Consumption.TranscriptDigest = registrarContractDigest(27)
			case "changed-principal":
				r.Consumption.Challenge.Binding.PrincipalID = "other"
			case "changed-session":
				r.Consumption.Challenge.Binding.SessionBinding = registrarContractDigest(28)
			case "changed-deadline":
				r.Consumption.Challenge.Binding.AccessUntilMS++
			case "downgrade":
				state.Version = 1
			case "v1-receipt-with-consumption":
				r.ReceiptEncodingVersion = 1
				r.ReceiptDigest = registrarReceiptDigest(*r)
			}
			// MarshalJSON deliberately cannot make a supported v1 image by dropping
			// fields: v1 record still contains unsupported consumption and refuses.
			original, err := json.Marshal(s.data)
			if err != nil {
				t.Fatal(err)
			}
			path := s.path
			s.Close()
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := Open(path)
			if opened != nil {
				opened.Close()
			}
			if err == nil || opened != nil {
				t.Fatal("consumption corruption/downgrade accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("invalid consumption history rewritten", err)
			}
		})
	}
}

func TestRegistrarConsumedHistoryPersistenceFaults(t *testing.T) {
	for _, operation := range []string{"expire", "delete"} {
		for _, stage := range []string{"prepublish", "uncertain"} {
			t.Run(operation+"/"+stage, func(t *testing.T) {
				s := registrarConsumptionFixture(t)
				r := registrarFixtureRecord(s)
				before, _ := json.Marshal(s.data)
				receipt := r.ReceiptDigest
				proof := registrarProofKey(r.Consumption.Challenge)
				claim := registrarClaimKey(r.NetworkID, r.DevicePublicKey)
				version := s.Version()
				real := s.fs
				if stage == "prepublish" {
					s.fs.rename = func(string, string) error { return errInjectedStoreIO }
				} else {
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
				if err == nil || s.Version() != version {
					t.Fatal("injected failure claimed success")
				}
				if stage == "prepublish" {
					after, _ := json.Marshal(s.data)
					if !bytes.Equal(before, after) {
						t.Fatal("rollback changed consumed history")
					}
				} else if s.Health() == nil {
					t.Fatal("uncertain consumption image kept authority")
				}
				s = reopenRegistrarFixture(t, s)
				r = registrarFixtureRecord(s)
				expected := "revoked"
				if stage == "prepublish" {
					expected = "active"
				}
				if r.Phase != expected || r.ReceiptDigest != receipt || s.data.Registrar.ConsumedProofs[proof] != claim {
					t.Fatal("reopen lost atomic phase/receipt/consumption")
				}
			})
		}
	}
}

func TestRegistrarConsumedHistoryStrictNestedFields(t *testing.T) {
	s := registrarConsumptionFixture(t)
	valid, err := json.Marshal(s.data)
	if err != nil {
		t.Fatal(err)
	}
	path := s.path
	s.Close()
	for _, data := range [][]byte{
		bytes.Replace(valid, []byte(`"session_binding":`), []byte(`"session_id":`), 1),
		bytes.Replace(valid, []byte(`"principal_namespace":`), []byte(`"Principal_Namespace":`), 1),
		bytes.Replace(valid, []byte(`"transcript_digest":`), []byte(`"unexpected":true,"transcript_digest":`), 1),
		bytes.Replace(valid, []byte(`"consumed_proofs":`), []byte(`"consumed_proofs":{},"consumed_proofs":`), 1),
	} {
		if bytes.Equal(data, valid) {
			t.Fatal("test replacement missed")
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		opened, err := Open(path)
		if opened != nil {
			opened.Close()
		}
		if err == nil || opened != nil {
			t.Fatal("nested alias/unknown/duplicate accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, data) {
			t.Fatal("rejected file changed", err)
		}
	}
}
