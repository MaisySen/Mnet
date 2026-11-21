package control

// Actual client/owned Store integration through an explicitly TEST-ONLY adapter.
// This adapter is not registered by Server and establishes NO ingress/parser,
// source eligibility, real enrollment/commit or native finalization acceptance.
import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	client "fintex.au/mnet/internal/controlclient"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/netmap"
)

func TestRegistrarActualClientStoreChild(t *testing.T) {
	path := os.Getenv("MNET_CURRENT_CLIENT_STORE_FIXTURE")
	if path == "" {
		t.Skip("fresh child only; fixture trust explicitly supplied")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Document contract.NativeCredentialDocument
		Mode     string
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	doc := input.Document
	c, err := client.NewRegistrarClient(doc.Receipt.RegistrarOrigin, doc, client.RegistrarLimits{Timeout: time.Second * 3, MaxConcurrent: 1, MaxRequests: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	first, err := c.CurrentMap(context.Background())
	if err != nil || first.Self.ID != doc.Receipt.NodeID {
		t.Fatal("actual first map", err)
	}
	err = c.RegistrarEndpoints(context.Background(), []string{"192.0.2.77:51820"})
	if input.Mode == "lost" {
		if err != client.ErrRegistrarUnavailable {
			t.Fatal("lost endpoint acknowledgement not unknown", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	next, err := c.CurrentMap(context.Background())
	if err != nil || len(next.Self.Endpoints) != 1 || next.Self.Endpoints[0] != "192.0.2.77:51820" {
		t.Fatal("actual published endpoint not observed", err)
	}
	if input.Mode == "denied" {
		if got, err := c.CurrentMap(context.Background()); err != client.ErrRegistrarDenied || got != nil || errors.Is(err, client.ErrUnauthorized) {
			t.Fatal("actual Store revocation not permanent nonlegacy denial", err)
		}
		if err := c.RegistrarEndpoints(context.Background(), nil); err != client.ErrRegistrarDenied {
			t.Fatal("denied owner retried endpoint update", err)
		}
	}
}
func TestRegistrarActualClientToOwnedStoreViaTestOnlyTLSAdapter(t *testing.T) {
	for _, mode := range []string{"denied", "lost"} {
		t.Run(mode, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			var sends atomic.Int64
			var doc contract.NativeCredentialDocument
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls := sends.Add(1)
				if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Mnet-Node-Credential-V1 "+secret || r.Header.Get("X-Node-ID") != "" || r.Header.Get("X-Node-Token") != "" {
					t.Error("actual client used wrong dedicated channel")
					http.Error(w, "refused", 401)
					return
				}
				// Test glue only, deliberately not a production parser/owner-authenticator.
				body, err := io.ReadAll(io.LimitReader(r.Body, 16385))
				if err != nil || len(body) > 16384 || bytes.Contains(body, []byte(secret)) {
					t.Error("secret/body bound")
					return
				}
				var request struct {
					Version       int                `json:"version"`
					Receipt       contract.ReceiptV2 `json:"receipt"`
					ReceiptDigest string             `json:"receipt_digest"`
					Endpoints     []string           `json:"endpoints"`
				}
				if json.Unmarshal(body, &request) != nil || request.Version != 1 || request.Receipt != doc.Receipt || request.ReceiptDigest != doc.ReceiptDigest {
					t.Error("actual client qualified record mismatch")
					http.Error(w, "refused", 401)
					return
				}
				if mode == "denied" && calls == 4 {
					if err := s.ExpireNode(ref.NodeID); err != nil {
						t.Error(err)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				switch r.URL.Path {
				case "/api/mnet-node-experimental-v1/map":
					m, err := s.registrarCredentialMap(ref, strings.TrimPrefix(r.Header.Get("Authorization"), "Mnet-Node-Credential-V1 "))
					if err != nil {
						http.Error(w, "current access refused", 401)
						return
					}
					json.NewEncoder(w).Encode(struct {
						Version       int         `json:"version"`
						ReceiptDigest string      `json:"receipt_digest"`
						Map           *netmap.Map `json:"map"`
					}{1, doc.ReceiptDigest, m})
				case "/api/mnet-node-experimental-v1/endpoints":
					if err := s.setRegistrarCredentialEndpoints(ref, strings.TrimPrefix(r.Header.Get("Authorization"), "Mnet-Node-Credential-V1 "), request.Endpoints); err != nil {
						http.Error(w, "current access refused", 401)
						return
					}
					if mode == "lost" {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
						} else {
							conn.Close()
						}
						return
					}
					json.NewEncoder(w).Encode(struct {
						Version       int    `json:"version"`
						ReceiptDigest string `json:"receipt_digest"`
						Updated       bool   `json:"updated"`
					}{1, doc.ReceiptDigest, true})
				default:
					t.Error("legacy/unregistered path selected")
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			// Update ONLY test-owned existing history before its actual strict save/reopen.
			s.mu.Lock()
			r := registrarFixtureRecord(s)
			c := &r.Consumption.Challenge
			now := time.Now()
			c.Binding.RegistrarOrigin = server.URL
			r.Scope.RegistrarOrigin = server.URL
			c.Binding.CheckedAtMS = now.Add(-time.Second).UnixMilli()
			c.Binding.ReserveDeadlineMS = now.Add(2 * time.Minute).UnixMilli()
			c.Binding.AccessUntilMS = now.Add(time.Minute).UnixMilli()
			c.ExpiresAtMS = now.Add(30 * time.Second).UnixMilli()
			r.CommittedAtMS = now.UnixMilli()
			r.AccessExpiresAtMS = c.Binding.AccessUntilMS
			end := time.UnixMilli(r.AccessExpiresAtMS).UTC()
			s.data.Nodes[r.NodeID].KeyExpiry = &end
			r.Consumption.TranscriptDigest = registrarTranscriptDigest(*c)
			r.ReceiptDigest = registrarReceiptDigest(*r)
			// Origin changed historical proof index, so explicit fixture reconstructs it.
			s.data.Registrar.ConsumedProofs = map[string]string{registrarProofKey(*c): registrarClaimKey(r.NetworkID, r.DevicePublicKey)}
			receipt, err := contract.ReceiptV2ForChallenge(*c, r.NodeID, r.ReceiptID, r.CommittedAtMS)
			if err != nil {
				t.Fatal(err)
			}
			doc = contract.NativeCredentialDocument{Version: 1, Kind: contract.NativeCredentialKind, Secret: secret, FinalizationID: c.ID, Challenge: *c, Receipt: receipt, ReceiptDigest: r.ReceiptDigest}
			if contract.CheckNativeCredentialDocument(doc) != nil {
				t.Fatal("test document/actual record recipe diverge")
			}
			ref.Scope = r.Scope
			ref.ReceiptDigest = r.ReceiptDigest
			ref.TranscriptDigest = r.Consumption.TranscriptDigest
			if err := s.saveLocked(); err != nil {
				t.Fatal(err)
			}
			s.mu.Unlock()
			s = reopenRegistrarFixture(t, s)
			beforeHistory, _ := json.Marshal(s.data.Registrar)
			beforeCount := len(s.data.Nodes)
			dir := t.TempDir()
			root := filepath.Join(dir, "roots.pem")
			if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			input, _ := json.Marshal(struct {
				Document contract.NativeCredentialDocument
				Mode     string
			}{doc, mode})
			path := filepath.Join(dir, "input.json")
			if err := os.WriteFile(path, input, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestRegistrarActualClientStoreChild$", "-test.v", "-test.timeout=15s")
			for _, v := range os.Environ() {
				if !strings.HasPrefix(v, "SSL_CERT_FILE=") && !strings.HasPrefix(v, "SSL_CERT_DIR=") && !strings.HasPrefix(v, "MNET_CURRENT_CLIENT_STORE_FIXTURE=") {
					cmd.Env = append(cmd.Env, v)
				}
			}
			cmd.Env = append(cmd.Env, "SSL_CERT_FILE="+root, "SSL_CERT_DIR="+dir, "MNET_CURRENT_CLIENT_STORE_FIXTURE="+path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("actual client child failed: %v\n%s", err, output)
			}
			if bytes.Contains(output, []byte(secret)) {
				t.Fatal("child log exposed private fixture credential")
			}
			expected := int64(3)
			if mode == "denied" {
				expected = 4
			}
			if sends.Load() != expected {
				t.Fatal("actual client unexpectedly retried/rejoined", sends.Load())
			}
			s = reopenRegistrarFixture(t, s)
			if len(s.data.Nodes) != beforeCount || len(s.data.NodeTokens) != 0 {
				t.Fatal("adapter created node/legacy credential")
			}
			n := s.data.Nodes[ref.NodeID]
			if len(n.Endpoints) != 1 || n.Endpoints[0] != "192.0.2.77:51820" {
				t.Fatal("actual durable endpoint outcome absent")
			}
			r = registrarFixtureRecord(s)
			if mode == "lost" {
				afterHistory, _ := json.Marshal(s.data.Registrar)
				if !bytes.Equal(beforeHistory, afterHistory) {
					t.Fatal("endpoint update altered receipt/history")
				}
			} else if r.Phase != "revoked" || r.ReceiptDigest != doc.ReceiptDigest {
				t.Fatal("revocation altered historical receipt")
			}
			t.Log("actual production-constructor client, verified local TLS, actual retained Store map/endpoint publication/reopen; test-only unregistered adapter, no node/credential creation")
		})
	}
}
