package enrollmentpending

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Consistency-only historical fixture builder. Real native proof/source/atomic
// Store commit+lost response is covered separately by cross-app integration.
func historyReady(t *testing.T, origin string) (*Journal, contract.Challenge, SourceConfig) {
	t.Helper()
	j, _, cfg := bootstrapReady(t, origin)
	r := *j.data
	r.CreatedMS -= 120000
	r.ExpiresMS = r.CreatedMS + 60000
	var own contract.SourceExchange
	json.Unmarshal([]byte(r.ExchangeJSON), &own)
	own.ExpiresAt = time.UnixMilli(r.CreatedMS + 30000).UTC().Format("2006-01-02T15:04:05.000Z")
	raw, _ := json.Marshal(own)
	r.ExchangeJSON = string(raw)
	now := r.CreatedMS + 10
	b, err := registrar.OwnWorkflowBindingDATA(own, r.Policy.ServiceID, code(), now, now+20000, now+5000)
	if err != nil {
		t.Fatal(err)
	}
	c := contract.Challenge{ID: code(), Nonce: code(), EphemeralPublicKey: code(), Binding: b, ExpiresAtMS: now + 5000}
	raw, _ = json.Marshal(contract.ChallengeWire{Version: 4, Challenge: c})
	w := workflowData{ReserveID: code(), StartedMS: r.CreatedMS + 1, TimeoutMS: 1000, MaxChallengeMS: 10000, MaxReserveMS: 60000, MaxAgeMS: 20000, MaxLeaseMS: 10000, UncertaintyMS: 1, ChallengeJSON: string(raw), ResponseDigest: registrar.WorkflowResponseDigest([32]byte{1})}
	r.Version = 4
	r.Phase = "workflow-submit-may-have-started"
	r.WorkflowJSON = workflowEncode(w)
	if err := j.commit(r); err != nil {
		t.Fatal("historical original fixture", err)
	}
	return j, c, cfg
}
func historyJournalCases(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, boundary := range []string{"marker", "receipt"} {
		for _, stage := range []string{"success", "create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync"} {
			t.Run("history-"+boundary+"-"+stage, func(t *testing.T) {
				j, c, cfg := historyReady(t, s.URL)
				dir := j.owner.Parent()
				device := c.Binding.DevicePublicKey
				secret := j.data.Credential
				before, _ := os.ReadFile(j.path)
				var calls int
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
				active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.URL.Path != registrar.HistoryPath || r.Header.Get("Authorization") != registrar.HistoryScheme+secret {
						t.Error("wrong historical channel")
					}
					var req contract.HistoryRequestWire
					json.NewDecoder(r.Body).Decode(&req)
					if req.Challenge != c {
						t.Error("history not same full pinned challenge")
					}
					raw, _ := os.ReadFile(j.path)
					var saved record
					if !strictRecord(raw, &saved) || !validRecord(saved) || saved.Version != 6 || saved.Phase != "history-lookup-may-have-started" {
						t.Error("history disclosure before durable valid marker")
					}
					if boundary == "receipt" {
						j.mu.Lock()
						inject()
						j.mu.Unlock()
					}
					receipt, err := contract.ReceiptV2ForChallenge(c, "old-node", code(), c.Binding.CheckedAtMS+1)
					if err != nil {
						t.Error(err)
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)})
				}))
				if boundary == "marker" {
					inject()
				}
				result, err := j.RecoverHistoryDATA(context.Background(), device, func() SourceConfig { return cfg }, time.Second, 2)
				good := stage == "success"
				uncertain := stage == "after-rename" || stage == "dirsync"
				if good {
					if err != nil || result.Receipt.NodeID != "old-node" {
						t.Fatal("expired historical lookup", err)
					}
				} else if err == nil || result != (contract.HistoryReceiptWire{}) {
					t.Fatal("failed historical result", err)
				}
				if uncertain && !errors.Is(err, ErrUncertain) {
					t.Fatal("lost history local uncertainty", err)
				}
				sends := 1
				if boundary == "marker" && !good {
					sends = 0
				}
				if calls != sends {
					t.Fatal("history send count", calls, sends)
				}
				if j.finalizationReady != nil {
					t.Fatal("history manufactured finalization readiness")
				}
				j.Close()
				reopened, err := Open(dir)
				if err != nil {
					t.Fatal("whole history reopen", err)
				}
				defer reopened.Close()
				phase, _ := reopened.Status()
				expected := "history-receipt-data"
				if !good {
					expected = "history-lookup-may-have-started"
					if boundary == "marker" && !uncertain {
						expected = "workflow-submit-may-have-started"
					}
					if boundary == "receipt" && uncertain {
						expected = "history-receipt-data"
					}
				}
				if phase != expected {
					t.Fatal("wrong whole history phase", phase, expected)
				}
				if expected == "workflow-submit-may-have-started" {
					after, _ := os.ReadFile(reopened.path)
					if !bytes.Equal(before, after) {
						t.Fatal("rollback-safe marker rewrote original intent")
					}
				}
				if expected == "history-receipt-data" {
					raw, _ := os.ReadFile(reopened.path)
					var object map[string]json.RawMessage
					json.Unmarshal(raw, &object)
					object["version"] = json.RawMessage(`4`)
					old, _ := json.Marshal(object)
					var rec record
					if strictRecord(old, &rec) {
						t.Fatal("old record decoder accepted explicit history field")
					}
				}
			})
		}
	}
	for _, mode := range []string{"budget", "lost", "wrong-device", "policy-before", "policy-during-marker", "policy-inflight", "policy-afterpin", "close-inflight", "replaced-file", "concurrent", "begin-unknown", "known-receipt-mismatch"} {
		t.Run("history-"+mode, func(t *testing.T) {
			j, c, cfg := historyReady(t, s.URL)
			defer j.Close()
			dir := j.owner.Parent()
			current := func() SourceConfig { return cfg }
			var calls atomic.Int32
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if mode == "lost" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				if mode == "close-inflight" {
					j.Close()
					return
				}
				if mode == "policy-inflight" {
					j.mu.Lock()
					cfg = SourceConfig{}
					j.mu.Unlock()
				}
				receipt, _ := contract.ReceiptV2ForChallenge(c, "historical-only", code(), c.Binding.CheckedAtMS+1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)})
			}))

			if mode == "known-receipt-mismatch" {
				r := *j.data
				w, _ := decodeWorkflow(r)
				known, _ := contract.ReceiptV2ForChallenge(c, "originally-known-node", code(), c.Binding.CheckedAtMS+1)
				raw, _ := json.Marshal(contract.WorkflowReceiptWire{Version: 1, Receipt: known, ReceiptDigest: contract.ReceiptV2Digest(known), ConfirmationMAC: code()})
				w.ReceiptJSON = string(raw)
				r.Phase = "workflow-receipt-data"
				r.WorkflowJSON = workflowEncode(w)
				if err := j.commit(r); err != nil {
					t.Fatal(err)
				}
			}
			device := c.Binding.DevicePublicKey
			if mode == "wrong-device" {
				device = "wrong"
			}
			if mode == "policy-before" {
				cfg = SourceConfig{}
			}
			if mode == "policy-during-marker" {
				real := j.fs.rename
				j.fs.rename = func(a, b string) error {
					err := real(a, b)
					if err == nil {
						cfg = SourceConfig{}
					}
					return err
				}
			}
			if mode == "replaced-file" {
				os.WriteFile(j.path, []byte("changed"), 0600)
			}
			if mode == "begin-unknown" {
				r := *j.data
				w, _ := decodeWorkflow(r)
				w.ChallengeJSON = ""
				w.ResponseDigest = ""
				r.WorkflowJSON = workflowEncode(w)
				r.Phase = "workflow-begin-may-have-started"
				if err := j.commit(r); err != nil {
					t.Fatal(err)
				}
			}
			var reads int
			current = func() SourceConfig {
				reads++
				if mode == "policy-afterpin" && reads >= 4 {
					return SourceConfig{}
				}
				return cfg
			}
			if mode == "concurrent" {
				// Only one lookup may own this phase at a time, and the finite durable
				// budget counts even later admitted calls. No more than two total sends.
				current = func() SourceConfig { return cfg }
				done := make(chan error, 8)
				for i := 0; i < 8; i++ {
					go func() {
						_, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 2)
						done <- err
					}()
				}
				for i := 0; i < 8; i++ {
					<-done
				}
				if calls.Load() < 1 || calls.Load() > 2 {
					t.Fatal("concurrent durable history budget", calls.Load())
				}
				return
			}
			result, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 2)
			good := mode == "budget"
			if good && err != nil || !good && (err == nil || result != (contract.HistoryReceiptWire{})) {
				t.Fatal("wrong history outcome", mode, err)
			}
			noSend := mode == "wrong-device" || mode == "policy-before" || mode == "policy-during-marker" || mode == "replaced-file" || mode == "begin-unknown"
			if noSend && calls.Load() != 0 {
				t.Fatal("history sent without current owned original context")
			}
			if mode != "budget" && mode != "lost" {
				return
			}
			j.Close()
			var openErr error
			j, openErr = Open(dir)
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer j.Close()
			image, _ := os.ReadFile(j.path)
			for _, params := range []struct {
				timeout time.Duration
				max     int
			}{{2 * time.Second, 2}, {time.Second, 3}} {
				if _, err := j.RecoverHistoryDATA(context.Background(), device, current, params.timeout, params.max); err == nil {
					t.Fatal("restart changed original history bounds")
				}
			}
			if after, _ := os.ReadFile(j.path); !bytes.Equal(image, after) {
				t.Fatal("invalid bounds rewrote history")
			}
			if _, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 2); good && err != nil {
				t.Fatal("explicit same lookup second original slot", err)
			}
			beforeCalls := calls.Load()
			if _, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 2); err == nil || calls.Load() != beforeCalls {
				t.Fatal("history restart budget reset")
			}
			if beforeCalls != 2 {
				t.Fatal("history explicit attempts", beforeCalls)
			}
		})
	}
}
