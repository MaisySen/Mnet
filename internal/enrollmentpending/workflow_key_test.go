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

// Uses the same fresh-process normal TLS roots as the original journal cases.
func workflowKeyJournalCases(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, mode := range []string{"success", "zero-admission", "no-consume", "callback-error", "copy-and-repeat", "close-before-use", "cancel-before-use", "policy-after-key", "policy-during-marker", "expired-during-marker", "no-channel", "concurrent", "reopen", "create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync"} {
		t.Run("key-"+mode, func(t *testing.T) {
			j, _, cfg := bootstrapReady(t, s.URL)
			defer j.Close()
			dir := j.owner.Parent()
			var sends, uses int
			var c contract.Challenge
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends++
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				if r.URL.Path == registrar.WorkflowPrefix+"begin" {
					var in contract.WorkflowBeginWire
					json.NewDecoder(r.Body).Decode(&in)
					raw, _ := os.ReadFile(j.path)
					var saved record
					json.Unmarshal(raw, &saved)
					var own contract.SourceExchange
					json.Unmarshal([]byte(saved.ExchangeJSON), &own)
					now := time.Now()
					b, err := registrar.OwnWorkflowBindingDATA(own, saved.Policy.ServiceID, in.Request.ReserveID, now.UnixMilli(), now.Add(20*time.Second).UnixMilli(), now.Add(5*time.Second).UnixMilli())
					if err != nil {
						t.Error(err)
					}
					c = contract.Challenge{Binding: b, ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: now.Add(5 * time.Second).UnixMilli()}
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
					return
				}
				raw, _ := os.ReadFile(j.path)
				var saved record
				if !strictRecord(raw, &saved) || saved.Phase != "workflow-submit-may-have-started" {
					t.Error("submit without durable marker")
				}
				receipt, err := contract.ReceiptV2ForChallenge(c, "fixture-node", code(), time.Now().UnixMilli())
				if err != nil {
					t.Error(err)
				}
				json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt), ConfirmationMAC: code()})
			}))
			current := func() SourceConfig { return cfg }
			if _, err := j.BeginWorkflowDATA(context.Background(), current, workflowLimits()); err != nil {
				t.Fatal(err)
			}
			original, _ := os.ReadFile(j.path)
			stage := mode
			switch stage {
			case "create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync":
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
			if mode == "policy-during-marker" || mode == "expired-during-marker" {
				real := j.fs.rename
				j.fs.rename = func(a, b string) error {
					err := real(a, b)
					if err == nil {
						if mode == "policy-during-marker" {
							cfg = SourceConfig{}
						} else {
							time.Sleep(5100 * time.Millisecond)
						}
					}
					return err
				}
			}
			if mode == "no-channel" {
				j.workflow.Close()
				j.workflow = nil
			}
			if mode == "reopen" {
				j.Close()
				var err error
				j, err = Open(dir)
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var retained *WorkflowKeyAdmission
			use := func(got contract.Challenge, expected contract.Binding) ([32]byte, error) {
				uses++
				if got != c || expected != c.Binding {
					t.Error("wrong independently owned context")
				}
				raw, _ := os.ReadFile(j.path)
				var saved record
				if !strictRecord(raw, &saved) || !validRecord(saved) || saved.Phase != "workflow-key-use-may-have-started" {
					t.Error("key before durable marker")
				}
				if mode == "policy-after-key" {
					cfg = SourceConfig{}
				}
				var response [32]byte
				response[0] = 1
				return response, nil
			}
			invoke := func() (contract.WorkflowReceiptWire, error) {
				return j.ProveWorkflowDATA(ctx, current, func(a *WorkflowKeyAdmission) ([32]byte, error) {
					retained = a
					switch mode {
					case "no-consume":
						return [32]byte{1}, nil
					case "zero-admission":
						var zero WorkflowKeyAdmission
						return zero.Respond(use)
					case "callback-error":
						return [32]byte{}, injected
					case "close-before-use":
						j.Close()
					case "cancel-before-use":
						cancel()
					}
					copy := *a
					response, err := a.Respond(use)
					if _, again := copy.Respond(use); again == nil {
						t.Error("copied admission reused key")
					}
					return response, err
				})
			}
			result, err := contract.WorkflowReceiptWire{}, error(nil)
			if mode == "concurrent" {
				type outcome struct {
					result contract.WorkflowReceiptWire
					err    error
				}
				outcomes := make(chan outcome, 8)
				for i := 0; i < 8; i++ {
					go func() { r, e := invoke(); outcomes <- outcome{r, e} }()
				}
				successes := 0
				for i := 0; i < 8; i++ {
					out := <-outcomes
					if out.err == nil {
						successes++
						result = out.result
					}
				}
				if successes != 1 {
					t.Fatal("concurrent admission winners", successes)
				}
			} else {
				result, err = invoke()
			}
			good := mode == "success" || mode == "copy-and-repeat" || mode == "concurrent"
			if good {
				if err != nil || result.Receipt.NodeID != "fixture-node" || uses != 1 || sends != 2 {
					t.Fatal("key continuation failed", err, uses, sends)
				}
			} else {
				if err == nil || result != (contract.WorkflowReceiptWire{}) {
					t.Fatal("bad key continuation accepted", mode, err)
				}
				if sends != 1 {
					t.Fatal("failure submitted", sends)
				}
			}
			if mode != "success" && mode != "copy-and-repeat" && mode != "concurrent" && mode != "policy-after-key" && uses != 0 {
				t.Fatal("key used on failure", mode, uses)
			}
			if retained != nil {
				if _, err := retained.Respond(use); err == nil {
					t.Fatal("retained admission survived return")
				}
			}
			if _, err := j.ProveWorkflowDATA(context.Background(), current, func(a *WorkflowKeyAdmission) ([32]byte, error) { return a.Respond(use) }); err == nil && stage != "create" && stage != "write" && stage != "short" && stage != "sync" && stage != "close" && stage != "rename" {
				t.Fatal("replayed key marker")
			}
			uncertain := stage == "after-rename" || stage == "dirsync"
			if uncertain && !errors.Is(err, ErrUncertain) {
				t.Fatal("lost uncertainty", err)
			}
			j.Close()
			reopened, openErr := Open(dir)
			if openErr != nil {
				t.Fatal("owning reopen", openErr)
			}
			defer reopened.Close()
			phase, _ := reopened.Status()
			expectedPhase := "workflow-key-use-may-have-started"
			switch mode {
			case "success", "copy-and-repeat", "concurrent":
				expectedPhase = "workflow-receipt-data"
			case "reopen", "no-channel", "create", "write", "short", "sync", "close", "rename":
				expectedPhase = "workflow-challenge-data"
			}
			if phase != expectedPhase {
				t.Fatal("wrong whole reopen phase", mode, phase, expectedPhase)
			}
			if expectedPhase == "workflow-challenge-data" {
				after, _ := os.ReadFile(reopened.path)
				if !bytes.Equal(original, after) {
					t.Fatal("rollback-safe failure changed original")
				}
			}
			callsBefore := uses
			if _, err := reopened.ProveWorkflowDATA(context.Background(), current, func(a *WorkflowKeyAdmission) ([32]byte, error) { return a.Respond(use) }); err == nil || uses != callsBefore {
				t.Fatal("reopen recreated key admission")
			}
		})
	}
}
