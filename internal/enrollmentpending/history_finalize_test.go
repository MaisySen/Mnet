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
	historyOwnedModeCases(t, s, active, false)
	t.Run("prepared-restart", func(t *testing.T) { historyOwnedModeCases(t, s, active, true) })
	t.Run("prepared-history7-restart", func(t *testing.T) { historyPreparedSevenCases(t, s, active) })
	preparedRecoveryCases(t, s, active)
}
func historyOwnedModeCases(t *testing.T, s *httptest.Server, active *atomic.Value, prepared bool) {
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
				if prepared {
					installPreparedFixture(t, j, result, originalRev, policyRev)
				}
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
				var err error
				if prepared {
					err = j.RecoverPreparedHistoryOwned(context.Background(), c.Binding.DevicePublicKey, originalRev, policyRev, func() SourceConfig { return cfg }, time.Second, 2)
				} else {
					err = j.RecoverHistoryOwned(context.Background(), c.Binding.DevicePublicKey, func() SourceConfig { return cfg }, time.Second, 2)
				}
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
					if prepared && doc.FinalizationID != hash("preserved original prepared identity") {
						t.Error("recovery changed original finalization ID")
					}
					published = doc
					raw, _ := os.ReadFile(j.path)
					var saved record
					if !strictRecord(raw, &saved) || !validRecord(saved) || (saved.Version != 7 && (!prepared || saved.Version != 8)) || saved.Phase != "native-finalization-prepared" {
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
	if prepared {
		return
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

func installPreparedFixture(t *testing.T, j *Journal, result contract.HistoryReceiptWire, stateRev, policyRev string) {
	t.Helper()
	r := *j.data
	w, _ := decodeWorkflow(r)
	raw, _ := json.Marshal(contract.WorkflowReceiptWire{Version: 1, Receipt: result.Receipt, ReceiptDigest: result.ReceiptDigest, ConfirmationMAC: code()})
	w.ReceiptJSON = string(raw)
	r.WorkflowJSON = workflowEncode(w)
	r.Version = 5
	r.Phase = "native-finalization-prepared"
	raw, _ = json.Marshal(finalizationData{ID: hash("preserved original prepared identity"), OriginalStateRevision: stateRev, PolicyRevision: policyRev, ReceiptDigest: result.ReceiptDigest})
	r.FinalizationJSON = string(raw)
	if err := j.commit(r); err != nil {
		t.Fatal(err)
	}
}
func preparedRecoveryCases(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, mode := range []string{"success", "wrong-state", "wrong-policy-revision", "wrong-device", "policy-semantic", "data-path", "ordinary-owned-path", "completed", "lost-budget", "conflicting-history", "reopen-marker-budget"} {
		t.Run("prepared-recovery-"+mode, func(t *testing.T) {
			j, c, cfg := historyLiveReady(t, s.URL)
			defer func() { j.Close() }()
			dir := j.owner.Parent()
			receipt, _ := contract.ReceiptV2ForChallenge(c, "original-node", code(), c.Binding.CheckedAtMS+1)
			result := contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)}
			stateRev, policyRev := code(), code()
			installPreparedFixture(t, j, result, stateRev, policyRev)
			originalID := hash("preserved original prepared identity")
			if mode == "completed" {
				r := *j.data
				r.Phase = "native-finalization-completed"
				if err := j.commit(r); err != nil {
					t.Fatal(err)
				}
			}
			original, _ := os.ReadFile(j.path)
			var calls atomic.Int32
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if mode == "lost-budget" || mode == "reopen-marker-budget" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				got := result
				if mode == "conflicting-history" {
					got.Receipt.NodeID = "conflict"
					got.ReceiptDigest = contract.ReceiptV2Digest(got.Receipt)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(got)
			}))
			device := c.Binding.DevicePublicKey
			switch mode {
			case "wrong-state":
				stateRev = hash("another State revision")
			case "wrong-policy-revision":
				policyRev = hash("another policy byte revision")
			case "wrong-device":
				device = hash("other public key")
			case "policy-semantic":
				cfg = SourceConfig{}
			}
			matches := CheckPreparedRecoveryStateDATA(original, stateRev, policyRev, device, cfg)
			invalid := mode == "wrong-state" || mode == "wrong-policy-revision" || mode == "wrong-device" || mode == "policy-semantic" || mode == "completed"
			if matches == invalid {
				t.Fatal("prepared safety match", matches)
			}
			var err error
			switch mode {
			case "data-path":
				_, err = j.RecoverHistoryDATA(context.Background(), device, func() SourceConfig { return cfg }, time.Second, 2)
			case "ordinary-owned-path":
				err = j.RecoverHistoryOwned(context.Background(), device, func() SourceConfig { return cfg }, time.Second, 2)
			default:
				err = j.RecoverPreparedHistoryOwned(context.Background(), device, stateRev, policyRev, func() SourceConfig { return cfg }, time.Second, 2)
			}
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				if err := j.FinalizeHistory(stateRev, policyRev, cfg, func(doc contract.NativeCredentialDocument) error {
					if doc.FinalizationID != originalID {
						t.Error("original ID replaced")
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || j.historyReady != nil {
				t.Fatal("failed prepared lookup created readiness", err)
			}
			noSend := invalid || mode == "data-path" || mode == "ordinary-owned-path"
			if noSend {
				after, _ := os.ReadFile(j.path)
				if calls.Load() != 0 || !bytes.Equal(after, original) {
					t.Fatal("invalid prepared attempt changed intent/disclosed")
				}
				return
			}
			j.Close()
			var openErr error
			j, openErr = Open(dir)
			if openErr != nil {
				t.Fatal(openErr)
			}
			if !CheckPreparedRecoveryStateDATA(mustReadPrepared(t, j.path), stateRev, policyRev, device, cfg) {
				t.Fatal("strict record8 reopen check")
			}
			if mode == "conflicting-history" {
				return
			}
			if j.RecoverPreparedHistoryOwned(context.Background(), device, stateRev, policyRev, func() SourceConfig { return cfg }, 2*time.Second, 2) == nil {
				t.Fatal("reopen changed timeout budget")
			}
			if j.RecoverPreparedHistoryOwned(context.Background(), device, stateRev, policyRev, func() SourceConfig { return cfg }, time.Second, 3) == nil {
				t.Fatal("reopen changed attempt budget")
			}
			if err := j.RecoverPreparedHistoryOwned(context.Background(), device, stateRev, policyRev, func() SourceConfig { return cfg }, time.Second, 2); err == nil {
				t.Fatal("lost lookup reported success")
			}
			j.Close()
			j, openErr = Open(dir)
			if openErr != nil {
				t.Fatal(openErr)
			}
			if j.RecoverPreparedHistoryOwned(context.Background(), device, stateRev, policyRev, func() SourceConfig { return cfg }, time.Second, 2) == nil || calls.Load() != 2 {
				t.Fatal("prepared reopen refunded spent budget")
			}
		})
	}
}
func mustReadPrepared(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func historyPreparedSevenCases(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, mode := range []string{"success", "lost", "conflict", "exhausted", "changed-budget", "marker-uncertain", "pin-uncertain", "prepared-uncertain", "completed-uncertain", "wrapper-tamper", "same-runtime-retry"} {
		t.Run(mode, func(t *testing.T) {
			j, c, cfg := historyLiveReady(t, s.URL)
			defer func() { j.Close() }()
			dir := j.owner.Parent()
			receipt, _ := contract.ReceiptV2ForChallenge(c, "history-seven-node", code(), c.Binding.CheckedAtMS+1)
			wire := contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)}
			current := func() SourceConfig { return cfg }
			stage := "initial"
			var calls atomic.Int32
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if stage == "lost" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				got := wire
				if stage == "conflict" {
					got.Receipt.NodeID = "conflicting-seven-node"
					got.ReceiptDigest = contract.ReceiptV2Digest(got.Receipt)
				}
				if stage == "pin-uncertain" {
					j.mu.Lock()
					j.fs.syncDir = func(string) error { return injected }
					j.mu.Unlock()
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(got)
			}))
			max := 4
			if mode == "exhausted" {
				max = 1
			}
			if err := j.RecoverHistoryOwned(context.Background(), c.Binding.DevicePublicKey, current, time.Second, max); err != nil {
				t.Fatal(err)
			}
			stateRev, policyRev := code(), code()
			var originalDoc contract.NativeCredentialDocument
			if err := j.FinalizeHistory(stateRev, policyRev, cfg, func(doc contract.NativeCredentialDocument) error { originalDoc = doc; return injected }); err == nil {
				t.Fatal("fixture must stop prepared")
			}
			original := *j.data
			originalHistory := original.HistoryJSON
			j.Close()
			var err error
			j, err = Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			rawBefore, _ := os.ReadFile(j.path)
			if !CheckPreparedRecoveryStateDATA(rawBefore, stateRev, policyRev, c.Binding.DevicePublicKey, cfg) {
				t.Fatal("strict original7 admission")
			}
			stage = mode
			if mode == "marker-uncertain" {
				j.fs.syncDir = func(string) error { return injected }
			}
			timeout := time.Second
			if mode == "changed-budget" {
				timeout = 2 * time.Second
			}
			err = j.RecoverPreparedHistoryOwned(context.Background(), c.Binding.DevicePublicKey, stateRev, policyRev, current, timeout, max)
			bad := mode == "lost" || mode == "conflict" || mode == "exhausted" || mode == "changed-budget" || mode == "marker-uncertain" || mode == "pin-uncertain"
			if bad {
				if err == nil || j.historyReady != nil {
					t.Fatal("failed prepared7 lookup acquired readiness", err)
				}
				if (mode == "exhausted" || mode == "changed-budget") && calls.Load() != 1 {
					t.Fatal("prepared7 reset original budget")
				}
			} else if err != nil {
				t.Fatal("prepared7 exact history", err)
			}
			if mode == "prepared-uncertain" {
				j.fs.syncDir = func(string) error { return injected }
			}
			if !bad {
				if mode == "wrapper-tamper" {
					rec := *j.data
					h, _ := decodeHistory(rec)
					h.MaxAttempts++
					data, _ := json.Marshal(h)
					rec.HistoryJSON = string(data)
					if validRecord(rec) {
						t.Fatal("wrapper expanded original7 budget")
					}
					h, _ = decodeHistory(*j.data)
					changed := wire
					changed.Receipt.NodeID = "replaced-anchor"
					changed.ReceiptDigest = contract.ReceiptV2Digest(changed.Receipt)
					data, _ = json.Marshal(changed)
					h.ReceiptJSON = string(data)
					data, _ = json.Marshal(h)
					rec.HistoryJSON = string(data)
					if validRecord(rec) {
						t.Fatal("wrapper replaced original7 first anchor")
					}
				}
				err = j.FinalizeHistory(stateRev, policyRev, cfg, func(doc contract.NativeCredentialDocument) error {
					if doc != originalDoc {
						t.Error("prepared7 changed exact original credential/finalization identity")
					}
					if mode == "completed-uncertain" {
						j.fs.syncDir = func(string) error { return injected }
					}
					if mode == "same-runtime-retry" {
						return injected
					}
					return nil
				})
				expectedFail := mode == "prepared-uncertain" || mode == "completed-uncertain" || mode == "same-runtime-retry"
				if expectedFail && err == nil || !expectedFail && err != nil {
					t.Fatal("prepared7 finalization", err)
				}
				if mode == "same-runtime-retry" {
					if err := j.FinalizeHistory(stateRev, policyRev, cfg, func(doc contract.NativeCredentialDocument) error {
						if doc != originalDoc {
							t.Error("retry changed ID")
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
				}
			}
			j.Close()
			j, err = Open(dir)
			if err != nil {
				t.Fatal("whole wrapper reopen", err)
			}
			if j.data.Version == 8 {
				h, _ := decodeHistory(*j.data)
				if h.PreparedHistoryJSON != originalHistory {
					t.Fatal("wrapper lost exact original7 bytes")
				}
				old, _ := decodeHistory(original)
				if h.ReceiptJSON != old.ReceiptJSON || h.Attempts != old.Attempts+1 || h.MaxAttempts != old.MaxAttempts || h.TimeoutMS != old.TimeoutMS {
					t.Fatal("wrapper reset pin/count/budget")
				}
			}
			if j.historyReady != nil {
				t.Fatal("wrapper reopen created runtime authority")
			}
		})
	}
}
