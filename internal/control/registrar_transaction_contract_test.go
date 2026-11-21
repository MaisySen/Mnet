//go:build registrar_transaction_contract

package control

// Inactive desired storage/legacy-isolation contract, not transaction or proof
// implementation. Fixtures modify the REAL owning Store's disk image between
// Close and Open. No fake registrar, enrollment endpoint or credential issuer.
import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func registrarContractDigest(b byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32))
}

func TestRegistrarTransactionRejectsMalformedPersistedNamespace(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"version":0,"records":{}}`, `{"version":2,"records":{}}`,
		`{"version":1,"records":null}`, `{"version":1,"records":[]}`,
		`{"version":1,"records":{"invalid":null}}`,
		`{"version":1,"records":{},"unexpected":true}`,
		`{"version":1,"version":2,"records":{}}`,
		`{"version":1,"records":{},"records":null}`,
	} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			original := []byte(`{"registrar_state":` + raw + `}`)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(path)
			if s != nil {
				s.Close()
			}
			if err == nil || s != nil {
				t.Error("malformed authority namespace silently ignored")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatal("rejected namespace changed source bytes", err)
			}
		})
	}
}

func TestRegistrarTransactionTombstoneBlocksLegacyRejoinAfterRestart(t *testing.T) {
	for _, phase := range []string{"revoked"} {
		for _, mode := range []string{"valid-reusable", "consumed-once", "expired-reusable", "removed-reusable", "credential-only"} {
			t.Run(phase+"/"+mode, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "state.json")
				s, err := openTestStore(t, path)
				if err != nil {
					t.Fatal(err)
				}
				network, err := s.CreateNetwork("fixture")
				if err != nil {
					t.Fatal(err)
				}
				key, err := s.CreateAuthKey(network.ID, "independent legacy authority", mode != "consumed-once", time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "consumed-once":
					s.data.AuthKeys[key.ID].Uses = 1
				case "expired-reusable":
					past := time.Now().Add(-time.Minute)
					s.data.AuthKeys[key.ID].ExpiresAt = &past
				case "removed-reusable":
					delete(s.data.AuthKeys, key.ID)
				}
				if err := s.saveLocked(); err != nil {
					t.Fatal(err)
				}
				rawKey := bytes.Repeat([]byte{9}, 32)
				pubKey := base64.StdEncoding.EncodeToString(rawKey)
				// A revoked claimed identity outlives its deleted node; strict load
				// must retain that history rather than permitting a fresh legacy join.
				record := map[string]any{
					"network_id": network.ID, "device_public_key": pubKey, "node_id": "node-retired-fixture",
					"credential_digest": registrarContractDigest(2), "phase": phase,
					"scope":             map[string]any{"issuer": "https://issuer.invalid", "registrar_origin": "https://registrar.invalid", "audience": "mnet-enrollment", "service_id": "fixture-service", "workspace_id": "fixture-workspace", "mapping_revision": "fixture-r1", "handoff_id": registrarContractDigest(3), "attempt_id": registrarContractDigest(4), "reserve_id": registrarContractDigest(5)},
					"descriptor_digest": registrarContractDigest(6), "descriptor_encoding_version": 1,
					"receipt_id": registrarContractDigest(7), "receipt_digest": registrarContractDigest(8),
					"committed_at_ms": int64(1000), "access_expires_at_ms": int64(2000),
				}
				tuple, _ := json.Marshal([]string{network.ID, pubKey})
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				disk, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var data map[string]json.RawMessage
				if err := json.Unmarshal(disk, &data); err != nil {
					t.Fatal(err)
				}
				data["registrar_state"], err = json.Marshal(map[string]any{"version": 1, "records": map[string]any{string(tuple): record}})
				if err != nil {
					t.Fatal(err)
				}
				before, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, before, 0600); err != nil {
					t.Fatal(err)
				}
				s, err = openTestStore(t, path)
				if err != nil {
					t.Fatal("claimed-history fixture must load without erasure", err)
				}
				version := s.Version()
				cursor := s.data.NextIP[network.ID]
				uses := 0
				if k := s.data.AuthKeys[key.ID]; k != nil {
					uses = k.Uses
				}
				authKey := key.Key
				if mode == "credential-only" {
					authKey = ""
				}
				result, joinErr := s.Join(netmap.JoinRequest{Network: network.ID, Name: "bypass", PubKey: pubKey, NodeID: "node-retired-fixture", NodeToken: "old-legacy-token", AuthKey: authKey})
				if joinErr == nil || result != nil {
					t.Error("standing registrar claim entered legacy rejoin")
				}
				if len(s.data.Nodes) != 0 || s.data.NextIP[network.ID] != cursor || s.Version() != version {
					t.Error("denied claimed rejoin allocated identity/address or published map")
				}
				if k := s.data.AuthKeys[key.ID]; k != nil && k.Uses != uses {
					t.Error("denied claimed rejoin consumed legacy key")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Error("denied claimed rejoin rewrote disk/history", err)
				}
			})
		}
	}
}
