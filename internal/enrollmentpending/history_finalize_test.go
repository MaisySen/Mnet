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

// Consistency fixture only. Actual native-owned key + Multies source + atomic
// Store commit/lost response/reopen/current maps are separate cross-app evidence.
func historyLiveReady(t *testing.T, origin string) (*Journal, contract.Challenge, SourceConfig) {
	j, _, cfg := bootstrapReady(t, origin)
	r := *j.data
	var own contract.SourceExchange
	json.Unmarshal([]byte(r.ExchangeJSON), &own)
	now := r.CreatedMS + 1
	b, err := registrar.OwnWorkflowBindingDATA(own, r.Policy.ServiceID, code(), now, now+20000, now+5000)
	if err != nil {
		t.Fatal(err)
	}
	c := contract.Challenge{ID: code(), Nonce: code(), EphemeralPublicKey: code(), Binding: b, ExpiresAtMS: now + 5000}
	raw, _ := json.Marshal(contract.ChallengeWire{Version: 4, Challenge: c})
	w := workflowData{ReserveID: b.ReserveID, StartedMS: r.CreatedMS, TimeoutMS: 1000, MaxChallengeMS: 10000, MaxReserveMS: 60000, MaxAgeMS: 20000, MaxLeaseMS: 10000, UncertaintyMS: 1, ChallengeJSON: string(raw), ResponseDigest: registrar.WorkflowResponseDigest([32]byte{1})}
	r.Version = 4
	r.Phase = "workflow-submit-may-have-started"
	r.WorkflowJSON = workflowEncode(w)
	if err := j.commit(r); err != nil {
		t.Fatal(err)
	}
	dir := j.owner.Parent()
	j.Close()
	j, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return j, c, cfg
}
func historyOwnedCases(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, boundary := range []string{"lookup-marker", "lookup-pin", "prepare", "complete"} {
		for _, stage := range []string{"success", "create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync"} {
			t.Run("history-owned-"+boundary+"-"+stage, func(t *testing.T) {
				j, c, cfg := historyLiveReady(t, s.URL)
				defer j.Close()
				dir := j.owner.Parent()
				originalRev, policyRev := code(), code()
				real := j.fs
				inject := func() {
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
				receipt, _ := contract.ReceiptV2ForChallenge(c, "same-original-node", code(), c.Binding.CheckedAtMS+1)
				result := contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)}
				var calls atomic.Int32
				active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if boundary == "lookup-pin" {
						j.mu.Lock()
						inject()
						j.mu.Unlock()
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					json.NewEncoder(w).Encode(result)
				}))
				if boundary == "lookup-marker" {
					inject()
				}
				err := j.RecoverHistoryOwned(context.Background(), c.Binding.DevicePublicKey, func() SourceConfig { return cfg }, time.Second, 2)
				lookupFail := (boundary == "lookup-marker" || boundary == "lookup-pin") && stage != "success"
				if lookupFail {
					if err == nil || j.historyReady != nil {
						t.Fatal("failed lookup acquired readiness", err)
					}
					if err := j.FinalizeHistory(originalRev, policyRev, cfg, func(contract.NativeCredentialDocument) error { t.Error("failed lookup published"); return nil }); err == nil {
						t.Fatal("lookup failure finalized")
					}
					j.Close()
					reopened, err := Open(dir)
					if err != nil {
						t.Fatal(err)
					}
					defer reopened.Close()
					if reopened.historyReady != nil {
						t.Fatal("reopen acquired readiness")
					}
					return
				}
				if err != nil || j.historyReady == nil || j.finalizationReady != nil {
					t.Fatal("owned history readiness category", err)
				}
				if boundary == "prepare" {
					inject()
				}
				var published contract.NativeCredentialDocument
				pubs := 0
				publish := func(doc contract.NativeCredentialDocument) error {
					pubs++
					published = doc
					raw, _ := os.ReadFile(j.path)
					var saved record
					if !strictRecord(raw, &saved) || !validRecord(saved) || saved.Version != 7 || saved.Phase != "native-finalization-prepared" {
						t.Error("publication before valid separate durable marker")
					}
					if doc.Receipt != result.Receipt || doc.Secret != j.data.Credential || doc.Challenge != c {
						t.Error("publication changed original credential")
					}
					if boundary == "complete" {
						inject()
					}
					return nil
				}
				err = j.FinalizeHistory(originalRev, policyRev, cfg, publish)
				good := stage == "success" || boundary == "lookup-marker" || boundary == "lookup-pin"
				if good && err != nil || !good && err == nil {
					t.Fatal("wrong finalization result", err)
				}
				expectedPubs := 1
				if boundary == "prepare" && !good {
					expectedPubs = 0
				}
				if pubs != expectedPubs {
					t.Fatal("wrong publication count", pubs)
				}
				if calls.Load() != 1 {
					t.Fatal("local finalization performed I/O")
				}
				uncertain := !good && (stage == "after-rename" || stage == "dirsync")
				if uncertain && !errors.Is(err, ErrUncertain) {
					t.Fatal("lost uncertainty", err)
				}
				j.Close()
				reopened, err := Open(dir)
				if err != nil {
					t.Fatal("record7 reopen", err)
				}
				defer reopened.Close()
				if reopened.historyReady != nil {
					t.Fatal("reopened file created readiness")
				}
				if err := reopened.FinalizeHistory(originalRev, policyRev, cfg, func(contract.NativeCredentialDocument) error { t.Error("reopened data published"); return nil }); err == nil {
					t.Fatal("reopened prepared history finalized")
				}
				if pubs == 1 {
					if err := reopened.CorrelateFinalization(published, originalRev, policyRev, true); err != nil {
						t.Fatal("existing exact State could not finish correlation", err)
					}
					if err := reopened.CorrelateFinalization(published, originalRev, policyRev, false); err != nil {
						t.Fatal(err)
					}
					bad := published
					bad.FinalizationID = code()
					if reopened.CorrelateFinalization(bad, originalRev, policyRev, true) == nil {
						t.Fatal("mismatched State correlated")
					}
				}
			})
		}
	}
	for _, mode := range []string{"data-only", "expired", "policy-afterpin", "cancel-inflight", "close-inflight", "replaced-before-finalize", "policy-finalize", "callback-failure", "retry-identity", "close-readiness", "uncertainty-readiness"} {
		t.Run("history-owned-"+mode, func(t *testing.T) {
			j, c, cfg := historyLiveReady(t, s.URL)
			defer j.Close()
			if mode == "expired" {
				j.Close()
				j, c, cfg = historyReady(t, s.URL)
				defer j.Close()
			}
			receipt, _ := contract.ReceiptV2ForChallenge(c, "original-node", code(), c.Binding.CheckedAtMS+1)
			result := contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "cancel-inflight" {
					cancel()
				}
				if mode == "close-inflight" {
					j.Close()
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(result)
			}))
			reads := 0
			current := func() SourceConfig {
				reads++
				if mode == "policy-afterpin" && reads >= 4 {
					return SourceConfig{}
				}
				return cfg
			}
			var err error
			if mode == "data-only" {
				_, err = j.RecoverHistoryDATA(ctx, c.Binding.DevicePublicKey, current, time.Second, 2)
			} else {
				err = j.RecoverHistoryOwned(ctx, c.Binding.DevicePublicKey, current, time.Second, 2)
			}
			var publishes int
			publish := func(contract.NativeCredentialDocument) error { publishes++; return nil }
			firstState, firstPolicy := code(), code()
			switch mode {
			case "data-only":
				if err != nil {
					t.Fatal(err)
				}
			case "expired", "policy-afterpin", "cancel-inflight", "close-inflight":
				if err == nil || j.historyReady != nil {
					t.Fatal("invalid readiness", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "replaced-before-finalize":
					os.WriteFile(j.path, []byte("changed"), 0600)
				case "policy-finalize":
					cfg = SourceConfig{}
				case "close-readiness":
					j.Close()
				case "uncertainty-readiness":
					j.mu.Lock()
					j.fault = true
					j.mu.Unlock()
				case "callback-failure", "retry-identity":
					var firstDoc contract.NativeCredentialDocument
					if err := j.FinalizeHistory(firstState, firstPolicy, cfg, func(doc contract.NativeCredentialDocument) error { firstDoc = doc; return injected }); err == nil {
						t.Fatal("failed callback reported success")
					}
					rawBefore, _ := os.ReadFile(j.path)
					if j.FinalizeHistory(hash("different original State"), firstPolicy, cfg, publish) == nil || publishes != 0 {
						t.Fatal("retry changed original State revision")
					}
					if rawAfter, _ := os.ReadFile(j.path); !bytes.Equal(rawBefore, rawAfter) {
						t.Fatal("bad retry changed prepared marker")
					}
					if err := j.FinalizeHistory(firstState, firstPolicy, cfg, func(doc contract.NativeCredentialDocument) error {
						if doc != firstDoc {
							t.Error("retry changed exact finalization identity")
						}
						return nil
					}); err != nil {
						t.Fatal("rollback-safe callback retry", err)
					}
					return
				}
			}
			if err := j.FinalizeHistory(firstState, firstPolicy, cfg, publish); err == nil || publishes != 0 {
				t.Fatal("DATA/stale/failed history became authority", err)
			}
		})
	}
}
