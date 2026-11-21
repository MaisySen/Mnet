package enrollmentpending

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Journal-only tests use real crypto engines and test-owned keys. Actual daemon
// retained-key/Store composition is separately exercised in cross-app tests.
func workflowFinalizeJournalCases(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, boundary := range []string{"prepare", "complete"} {
		for _, stage := range []string{"success", "data-only", "create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync"} {
			t.Run("finalization-"+boundary+"-"+stage, func(t *testing.T) {
				private, err := ecdh.X25519().GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				var key [32]byte
				copy(key[:], private.Bytes())
				defer clear(key[:])
				j, _, cfg := bootstrapReady(t, s.URL, base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()))
				defer j.Close()
				dir := j.owner.Parent()
				var challenge contract.Challenge
				var server *proof.Server
				var calls int
				active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					if r.URL.Path == registrar.WorkflowPrefix+"begin" {
						var req contract.WorkflowBeginWire
						json.NewDecoder(r.Body).Decode(&req)
						raw, _ := os.ReadFile(j.path)
						var saved record
						json.Unmarshal(raw, &saved)
						var own contract.SourceExchange
						json.Unmarshal([]byte(saved.ExchangeJSON), &own)
						now := time.Now()
						b, err := registrar.OwnWorkflowBindingDATA(own, saved.Policy.ServiceID, req.Request.ReserveID, now.UnixMilli(), now.Add(20*time.Second).UnixMilli(), now.Add(5*time.Second).UnixMilli())
						if err != nil {
							t.Error(err)
						}
						server, challenge, err = proof.NewServer(b, now, now.Add(5*time.Second), 10*time.Second)
						if err != nil {
							t.Error(err)
						}
						json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: challenge})
						return
					}

					var req contract.WorkflowSubmitWire
					json.NewDecoder(r.Body).Decode(&req)
					raw, _ := base64.RawURLEncoding.DecodeString(req.ResponseMAC)
					var response [32]byte
					copy(response[:], raw)
					confirmation, verifyErr := server.Verify(req.ChallengeID, response, time.Now())
					if verifyErr != nil && stage != "data-only" {
						t.Error(verifyErr)
					}
					receipt, err := contract.ReceiptV2ForChallenge(challenge, "journal-fixture-node", code(), time.Now().UnixMilli())
					if err != nil {
						t.Error(err)
					}
					json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt), ConfirmationMAC: base64.RawURLEncoding.EncodeToString(confirmation[:])})
				}))
				current := func() SourceConfig { return cfg }
				c, err := j.BeginWorkflowDATA(context.Background(), current, workflowLimits())
				if err != nil {
					t.Fatal(err)
				}

				var wire contract.WorkflowReceiptWire
				if stage == "data-only" {
					// Actual proof MAC + authenticated receipt DATA but no retained outstanding
					// engine confirmation in the Journal must NOT enable credential transfer.
					native, err := proof.Respond(key, c, c.Binding, time.Now(), workflowLimits().MaxChallengeLifetime)
					if err != nil {
						t.Fatal(err)
					}
					defer native.Close()
					mac, _ := native.Response(time.Now())
					wire, err = j.SubmitWorkflowDATA(context.Background(), c.ID, mac, current)
					if err != nil {
						t.Fatal(err)
					}
					if err := j.FinalizeWorkflow(wire, code(), code(), cfg, func(contract.NativeCredentialDocument) error { t.Error("DATA-only path exposed secret"); return nil }); err == nil {
						t.Fatal("DATA-only receipt created finalization readiness")
					}
					server.Close()
					return
				}
				wire, err = j.ProveWorkflowOwned(context.Background(), current, func(a *WorkflowKeyAdmission) (*proof.Native, error) {
					return a.RespondProof(func(got contract.Challenge, expected contract.Binding) (*proof.Native, error) {
						return proof.Respond(key, got, expected, time.Now(), workflowLimits().MaxChallengeLifetime)
					})
				})
				server.Close()
				if err != nil {
					t.Fatal(err)
				}
				before, _ := os.ReadFile(j.path)
				inject := func() {
					real := j.fs
					j.fs.create = func(dir string) (tempFile, error) {
						if stage == "create" {
							return nil, injected
						}
						f, err := real.create(dir)
						if err != nil {
							return nil, err
						}
						return &wrappedTemp{f, stage}, nil
					}
					if stage == "rename" {
						j.fs.rename = func(string, string) error { return injected }
					}
					if stage == "after-rename" {
						j.fs.rename = func(a, b string) error {
							if err := real.rename(a, b); err != nil {
								return err
							}
							return injected
						}
					}
					if stage == "dirsync" {
						j.fs.syncDir = func(string) error { return injected }
					}
				}
				if boundary == "prepare" {
					inject()
				}
				stateRev, policyRev := code(), code()
				var published contract.NativeCredentialDocument
				publications := 0
				err = j.FinalizeWorkflow(wire, stateRev, policyRev, cfg, func(doc contract.NativeCredentialDocument) error {
					publications++
					raw, _ := os.ReadFile(j.path)
					var saved record
					if !strictRecord(raw, &saved) || !validRecord(saved) || saved.Version != 5 || saved.Phase != "native-finalization-prepared" {
						t.Error("native publication before valid durable marker")
					}
					if doc.Challenge != c || doc.Receipt != wire.Receipt || doc.ReceiptDigest != wire.ReceiptDigest || contract.CheckNativeCredentialDocument(doc) != nil {
						t.Error("changed qualified document")
					}
					published = doc
					if boundary == "complete" {
						inject()
					}
					return nil
				})
				good := stage == "success"
				uncertain := stage == "after-rename" || stage == "dirsync"
				if good && err != nil || !good && err == nil {
					t.Fatal("wrong finalization result", err)
				}
				if uncertain && !errors.Is(err, ErrUncertain) {
					t.Fatal("lost pending uncertainty", err)
				}
				wantPub := 0
				if good || boundary == "complete" {
					wantPub = 1
				}
				if publications != wantPub {
					t.Fatal("State callback crossed failed prepare", publications, wantPub)
				}
				if calls != 2 {
					t.Fatal("local finalization performed network I/O")
				}
				if _, err := j.Status(); uncertain && err == nil {
					t.Fatal("uncertain journal not frozen")
				}
				j.Close()
				reopened, err := Open(dir)
				if err != nil {
					t.Fatal("whole pending reopen", err)
				}
				defer reopened.Close()
				phase, _ := reopened.Status()
				expected := "native-finalization-completed"
				if !good {
					expected = "native-finalization-prepared"
					if boundary == "complete" && uncertain {
						expected = "native-finalization-completed"
					}
					if boundary == "prepare" && !uncertain {
						expected = "workflow-receipt-data"
					}
				}
				if phase != expected {
					t.Fatal("wrong owning reopen phase", phase, expected)
				}
				if expected == "workflow-receipt-data" {
					after, _ := os.ReadFile(reopened.path)
					if !bytes.Equal(before, after) {
						t.Fatal("rollback-safe prepare rewrote receipt DATA")
					}
				}
				if err := reopened.FinalizeWorkflow(wire, stateRev, policyRev, cfg, func(contract.NativeCredentialDocument) error {
					t.Error("restart reconstructed missing native authority")
					return nil
				}); err == nil {
					t.Fatal("reopen replayed native publication")
				}
				if publications == 0 {
					return
				}
				for _, mode := range []string{"wrong-original", "wrong-policy", "wrong-id", "wrong-secret", "wrong-receipt"} {
					doc := published
					original, policy := stateRev, policyRev
					switch mode {
					case "wrong-original":
						original = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32))
					case "wrong-policy":
						policy = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32))
					case "wrong-id":
						doc.FinalizationID = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32))
					case "wrong-secret":
						doc.Secret = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32))
					case "wrong-receipt":
						doc.Receipt.ReceiptID = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32))
					}
					if err := reopened.CorrelateFinalization(doc, original, policy, true); err == nil {
						t.Fatal("mismatched native State completed pending", mode)
					}
				}
				if expected == "native-finalization-prepared" {
					if err := reopened.CorrelateFinalization(published, stateRev, policyRev, false); err == nil {
						t.Fatal("unfinished correlation allowed current operations")
					}
				}
				if err := reopened.CorrelateFinalization(published, stateRev, policyRev, true); err != nil {
					t.Fatal("exact existing native State not recovered", err)
				}
				if err := reopened.CorrelateFinalization(published, stateRev, policyRev, false); err != nil {
					t.Fatal("completed correlation not stable", err)
				}
				raw, _ := os.ReadFile(reopened.path)
				var fields map[string]json.RawMessage
				json.Unmarshal(raw, &fields)
				for _, field := range []string{"finalization_json", "workflow_json"} {
					old := fields[field]
					fields[field] = json.RawMessage(`""`)
					broken, _ := json.Marshal(fields)
					var out record
					if strictRecord(broken, &out) && validRecord(out) {
						t.Fatal("missing finalization evidence accepted", field)
					}
					fields[field] = old
				}
			})
		}
	}
}
