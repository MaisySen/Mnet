package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Local actual-native lifecycle test; the actual committed-then-lost submit and
// deleted/expired Store history are exercised in the opt-in cross-app fixture.
func testNativeHistoryOwner(t *testing.T, d *Daemon, policyPath string, policyRaw, before []byte, c contract.Challenge, install func(http.HandlerFunc)) {
	t.Helper()
	dir := d.stateOwner.lease.Parent()
	originalKey := d.stateOwner.saved.PublicKey
	d.Close()
	reopened, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var saved struct{ Credential string }
	raw, _ := os.ReadFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"))
	json.Unmarshal(raw, &saved)
	var calls atomic.Int32
	install(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != registrar.HistoryPath || r.Header.Get("Authorization") != registrar.HistoryScheme+saved.Credential {
			t.Error("actual native history wrong channel")
		}
		var req contract.HistoryRequestWire
		json.NewDecoder(r.Body).Decode(&req)
		if req.Challenge != c {
			t.Error("native history changed original challenge")
		}
		receipt, _ := contract.ReceiptV2ForChallenge(c, "historical-node", c.Nonce, c.Binding.CheckedAtMS+1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)})
	}))
	// Policy and detached state mutation must stop disclosure before even marker.
	// A differently formatted valid policy is captured by a new native context,
	// but Journal compares its original exact semantic policy. An actual semantic
	// change is required here, whitespace is not a new recorded policy grant.
	os.WriteFile(policyPath, []byte(`{}`), 0600)
	if _, err := reopened.recoverNativeHistoryDATA(context.Background(), time.Second, 2); err == nil || calls.Load() != 0 {
		t.Fatal("invalid policy disclosed recovery credential")
	}
	os.WriteFile(policyPath, policyRaw, 0600)
	reopened.mu.Lock()
	reopened.state.NodeName = "detached"
	reopened.mu.Unlock()
	if _, err := reopened.recoverNativeHistoryDATA(context.Background(), time.Second, 2); err == nil || calls.Load() != 0 {
		t.Fatal("mutated native image disclosed credential")
	}
	reopened.mu.Lock()
	*reopened.state = cloneState(reopened.stateOwner.saved)
	reopened.mu.Unlock()
	result, err := reopened.recoverNativeHistoryDATA(context.Background(), time.Second, 2)
	if err != nil || result.Receipt.NodeID != "historical-node" || calls.Load() != 1 {
		t.Fatal("native reopened history", err)
	}
	if reopened.stateOwner.saved.PublicKey != originalKey || reopened.state.native != nil || reopened.cc != nil || reopened.dev != nil {
		t.Fatal("history changed actual native authority")
	}
	if err := reopened.finalizeNativeEnrollment(context.Background()); err == nil {
		t.Fatal("history DATA became native finalization")
	}
	after, _ := os.ReadFile(reopened.stateOwner.path)
	if !bytes.Equal(after, before) {
		t.Fatal("history rewrote sole State")
	}
	reopened.Close()
	again, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("history record prevented owning reopen", err)
	}
	defer again.Close()
	if _, err := again.recoverNativeHistoryDATA(context.Background(), time.Second, 2); err != nil || calls.Load() != 2 {
		t.Fatal("second explicit persistent history slot", err)
	}
	if _, err := again.recoverNativeHistoryDATA(context.Background(), time.Second, 2); err == nil || calls.Load() != 2 {
		t.Fatal("daemon restart reset durable history budget")
	}
	again.Close()
	pendingPath := filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json")
	historyRaw, _ := os.ReadFile(pendingPath)
	var root map[string]json.RawMessage
	json.Unmarshal(historyRaw, &root)
	for _, field := range []string{"history_json", "workflow_json"} {
		old := root[field]
		root[field] = json.RawMessage(`""`)
		bad, _ := json.Marshal(root)
		os.WriteFile(pendingPath, bad, 0600)
		if owner, err := New(Config{StateDir: dir}); err == nil {
			owner.Close()
			t.Fatal("native loader accepted malformed new history namespace", field)
		}
		if _, err := LoadState(dir); err == nil {
			t.Fatal("detached loader accepted malformed history namespace", field)
		}
		if after, _ := os.ReadFile(pendingPath); !bytes.Equal(after, bad) {
			t.Fatal("malformed history safety gate rewrote evidence")
		}
		root[field] = old
	}
	os.WriteFile(pendingPath, historyRaw, 0600)
	final, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("failed history safety opens leaked owner", err)
	}
	final.Close()
}
