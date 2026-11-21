package enrollmentpending

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// The first delivered historical receipt is an immutable anchor even when the
// original workflow never pinned a receipt (the real lost-submit case). All
// subsequent lookup markers and failures must preserve those exact bytes.
func historyImmutableCases(t *testing.T, s *httptest.Server, active *atomic.Value) {
	type scenario struct{ boundary, stage string }
	cases := []scenario{{"response", "same"}, {"response", "different-node"}, {"response", "different-receipt-id"}, {"response", "different-commit"}, {"response", "lost"}, {"response", "policy-inflight"}, {"response", "policy-afterpin"}}
	for _, boundary := range []string{"marker", "receipt"} {
		for _, stage := range []string{"create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync"} {
			cases = append(cases, scenario{boundary, stage})
		}
	}
	for _, item := range cases {
		t.Run("history-immutable-"+item.boundary+"-"+item.stage, func(t *testing.T) {
			j, c, cfg := historyReady(t, s.URL)
			dir := j.owner.Parent()
			defer func() { j.Close() }()
			device := c.Binding.DevicePublicKey
			firstReceipt, err := contract.ReceiptV2ForChallenge(c, "first-immutable-node", code(), c.Binding.CheckedAtMS+1)
			if err != nil {
				t.Fatal(err)
			}
			firstWire := contract.HistoryReceiptWire{Version: 1, Receipt: firstReceipt, ReceiptDigest: contract.ReceiptV2Digest(firstReceipt)}
			mode := "first"
			calls := 0
			nativeReads := 0
			realFS := j.fs
			inject := func() {
				j.fs.create = func(dir string) (tempFile, error) {
					if item.stage == "create" {
						return nil, injected
					}
					f, err := realFS.create(dir)
					if err != nil {
						return nil, err
					}
					return &wrappedTemp{f, item.stage}, nil
				}
				if item.stage == "rename" {
					j.fs.rename = func(string, string) error { return injected }
				}
				if item.stage == "after-rename" {
					j.fs.rename = func(a, b string) error {
						if err := realFS.rename(a, b); err != nil {
							return err
						}
						return injected
					}
				}
				if item.stage == "dirsync" {
					j.fs.syncDir = func(string) error { return injected }
				}
			}
			var originalPin string
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != registrar.HistoryPath {
					t.Error("unexpected channel")
				}
				// Every disclosed retry must ALREADY carry the old immutable receipt in
				// the durable spent marker, not only in a local pre-request variable.
				raw, _ := os.ReadFile(j.path)
				var saved record
				if !strictRecord(raw, &saved) || !validRecord(saved) {
					t.Error("invalid on-disk retry marker")
				}
				h, ok := decodeHistory(saved)
				if !ok {
					t.Error("invalid historical marker")
				}
				if mode != "first" && h.ReceiptJSON != originalPin {
					t.Error("retry marker erased/changed immutable history")
				}
				result := firstWire
				switch mode {
				case "different-node":
					result.Receipt.NodeID = "different-node"
				case "different-receipt-id":
					result.Receipt.ReceiptID = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{17}, 32))
				case "different-commit":
					result.Receipt.CommittedAtMS++
				case "lost":
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				case "policy-inflight":
					cfg = SourceConfig{}
				}
				result.ReceiptDigest = contract.ReceiptV2Digest(result.Receipt)
				if item.boundary == "receipt" && mode == "second" {
					j.mu.Lock()
					realFS = j.fs
					inject()
					j.mu.Unlock()
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(result)
			}))
			originalCfg := cfg
			current := func() SourceConfig {
				nativeReads++
				if mode == "policy-afterpin" && nativeReads >= 4 {
					return SourceConfig{}
				}
				return cfg
			}
			first, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 4)
			if err != nil || first != firstWire {
				t.Fatal("first history pin", err)
			}
			h, _ := decodeHistory(*j.data)
			originalPin = h.ReceiptJSON
			originalWorkflow := j.data.WorkflowJSON
			reopen := func() {
				j.Close()
				var err error
				j, err = Open(dir)
				if err != nil {
					t.Fatal("owning immutable history reopen", err)
				}
			}
			reopen()
			mode = item.stage
			nativeReads = 0
			if item.boundary != "response" {
				mode = "second"
			}
			if item.boundary == "marker" {
				realFS = j.fs
				inject()
			}
			second, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 4)
			if item.boundary == "response" && item.stage == "same" {
				if err != nil || second != first {
					t.Fatal("exact repeat refused", err)
				}
			} else if err == nil || second != (contract.HistoryReceiptWire{}) {
				t.Fatal("failed or conflicting second receipt accepted", err)
			}
			reopen()
			cfg = originalCfg
			nativeReads = 0
			h, ok := decodeHistory(*j.data)
			if !ok || h.ReceiptJSON != originalPin || j.data.WorkflowJSON != originalWorkflow {
				t.Fatal("failure/reopen lost original historical anchor")
			}
			if j.finalizationReady != nil {
				t.Fatal("historical conflict produced authority")
			}
			// After ANY second-result/fault state, a conflicting valid projection must
			// still refuse. This catches failure markers that lost the anchor on reopen.
			mode = "different-node"
			if result, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 4); err == nil || result != (contract.HistoryReceiptWire{}) {
				t.Fatal("conflict after failure/reopen replaced first receipt", err)
			}
			reopen()
			h, _ = decodeHistory(*j.data)
			if h.ReceiptJSON != originalPin {
				t.Fatal("conflict erased first receipt on disk")
			}
			mode = "same"
			exact, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 4)
			if err != nil || exact != first {
				t.Fatal("same immutable receipt after failed conflicting lookup", err)
			}
			reopen()
			h, _ = decodeHistory(*j.data)
			if h.ReceiptJSON != originalPin || j.data.WorkflowJSON != originalWorkflow {
				t.Fatal("successful exact repeat rewrote original pin/context")
			}
			beforeCalls := calls
			if h.Attempts == 4 {
				if _, err := j.RecoverHistoryDATA(context.Background(), device, current, time.Second, 4); err == nil || calls != beforeCalls {
					t.Fatal("immutable retry reset finite budget")
				}
			}
		})
	}
}
