package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Called only after actual native durable-key proof + confirmed normal-TLS
// response. Full actual Multies/Store composition is separate cross-app evidence.
func testPreparedNativeRestart(t *testing.T, d *Daemon, policyPath string, policyRaw, before []byte, result contract.WorkflowReceiptWire, stage string, seven bool, install func(http.HandlerFunc)) {
	t.Helper()
	dir := d.stateOwner.lease.Parent()
	pendingPath := filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json")
	key := d.stateOwner.saved.PublicKey
	if seven {
		d.Close()
		var err error
		d, err = New(Config{StateDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		install(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != registrar.HistoryPath {
				t.Error("record7 setup wrong channel")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version: 1, Receipt: result.Receipt, ReceiptDigest: result.ReceiptDigest})
		}))
		if err := d.recoverNativeHistoryOwned(context.Background(), time.Second, 2); err != nil {
			t.Fatal("record7 setup owned history", err)
		}
	}
	real := d.stateOwner.fs.create
	d.stateOwner.fs.create = func(string) (stateTemp, error) { return nil, errNativeInjected }
	if err := d.finalizeNativeEnrollment(context.Background()); !errors.Is(err, errNativeInjected) {
		t.Fatal("fixture did not stop AFTER durable prepared marker", err)
	}
	d.stateOwner.fs.create = real
	raw, _ := os.ReadFile(pendingPath)
	var prepared struct {
		Version          int
		Phase            string
		FinalizationJSON string `json:"finalization_json"`
	}
	json.Unmarshal(raw, &prepared)
	expectedVersion := 5
	if seven {
		expectedVersion = 7
	}
	if prepared.Version != expectedVersion || prepared.Phase != "native-finalization-prepared" {
		t.Fatal("not original prepared record5")
	}
	if after, _ := os.ReadFile(d.stateOwner.path); !bytes.Equal(before, after) {
		t.Fatal("failed native publication changed State")
	}
	d.Close()
	if _, err := LoadState(dir); err == nil {
		t.Fatal("ordinary detached loader relaxed")
	}
	if owner, err := New(Config{StateDir: dir}); err == nil {
		owner.Close()
		t.Fatal("ordinary daemon loader relaxed")
	}
	var calls atomic.Int32
	receipt := contract.HistoryReceiptWire{Version: 1, Receipt: result.Receipt, ReceiptDigest: result.ReceiptDigest}
	var recovery *nativeFinalizationRecovery
	install(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != registrar.HistoryPath {
			t.Error("recovery used source/proof channel")
		}
		switch stage {
		case "denied":
			w.WriteHeader(403)
			return
		case "lost":
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		case "close-inflight":
			recovery.Close()
			return
		case "freeze-inflight":
			recovery.daemon.mu.Lock()
			recovery.daemon.freezeStateLocked()
			recovery.daemon.mu.Unlock()
			return
		case "policy-inflight":
			os.WriteFile(policyPath, append(policyRaw, ' '), 0600)
		}
		got := receipt
		if stage == "conflict" {
			got.Receipt.NodeID = "wrong-history-node"
			got.ReceiptDigest = contract.ReceiptV2Digest(got.Receipt)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(got)
	}))
	invalid := true
	switch stage {
	case "missing-state":
		os.Remove(statePath(dir))
	case "changed-state":
		var fields map[string]json.RawMessage
		json.Unmarshal(before, &fields)
		fields["node_name"] = json.RawMessage(`"different native image"`)
		bad, _ := json.Marshal(fields)
		os.WriteFile(statePath(dir), bad, 0600)
	case "wrong-key-state":
		other, _, _ := enrollmentContextFixture(t)
		otherRaw, _ := os.ReadFile(other.stateOwner.path)
		other.Close()
		os.WriteFile(statePath(dir), otherRaw, 0600)
	case "missing-policy":
		os.Remove(policyPath)
	case "changed-policy":
		os.WriteFile(policyPath, append(policyRaw, ' '), 0600)
	case "malformed-pending":
		os.WriteFile(pendingPath, []byte(`{}`), 0600)
	case "completed":
		var fields map[string]json.RawMessage
		json.Unmarshal(raw, &fields)
		fields["phase"] = json.RawMessage(`"native-finalization-completed"`)
		bad, _ := json.Marshal(fields)
		os.WriteFile(pendingPath, bad, 0600)
	case "missing-pending":
		os.Remove(pendingPath)
	case "legacy-state":
		var fields map[string]json.RawMessage
		json.Unmarshal(before, &fields)
		fields["auth_key"] = json.RawMessage(`"legacy authority cannot be inherited"`)
		bad, _ := json.Marshal(fields)
		os.WriteFile(statePath(dir), bad, 0600)
	default:
		invalid = false
	}
	stateBeforeOpen, _ := os.ReadFile(statePath(dir))
	pendingBeforeOpen, _ := os.ReadFile(pendingPath)
	var err error
	recovery, err = openNativeFinalizationRecovery(dir)
	if invalid {
		if err == nil {
			recovery.Close()
			t.Fatal("narrow owner accepted invalid prepared context", stage)
		}
		stateAfter, _ := os.ReadFile(statePath(dir))
		pendingAfter, _ := os.ReadFile(pendingPath)
		if !bytes.Equal(stateBeforeOpen, stateAfter) || !bytes.Equal(pendingBeforeOpen, pendingAfter) || calls.Load() != 0 {
			t.Fatal("failed narrow open changed identity/evidence or made request")
		}
		os.WriteFile(statePath(dir), before, 0600)
		os.WriteFile(pendingPath, raw, 0600)
		os.WriteFile(policyPath, policyRaw, 0600)
		recovery, err = openNativeFinalizationRecovery(dir)
		if err != nil {
			t.Fatal("failed narrow opens leaked owner", err)
		}
		recovery.Close()
		return
	}
	if err != nil {
		t.Fatal("exact prepared recovery owner", err)
	}
	defer recovery.Close()
	if calls.Load() != 0 {
		t.Fatal("recovery constructor started network")
	}
	// Owning native lease is retained and no competing normal owner can start.
	if owner, err := New(Config{StateDir: dir}); err == nil {
		owner.Close()
		t.Fatal("narrow owner released native lease")
	}
	if stage == "expired-original-access" {
		// The exact original prepared receipt expires naturally, not by editing
		// its projection or supplying a caller-selected current time.
		wait := time.Until(time.UnixMilli(result.Receipt.AccessExpiresAtMS)) + 10*time.Millisecond
		if wait > 0 {
			time.Sleep(wait)
		}
		if time.Now().UnixMilli() < result.Receipt.AccessExpiresAtMS {
			t.Fatal("expiry fixture still live")
		}
		stateBefore, _ := os.ReadFile(statePath(dir))
		if err := recovery.resume(context.Background(), time.Second, 2); err == nil {
			t.Fatal("expired prepared original access finalized")
		}
		stateAfter, _ := os.ReadFile(statePath(dir))
		if !bytes.Equal(stateBefore, stateAfter) || recovery.daemon.stateOwner.saved.native != nil || recovery.daemon.enrollmentPreparation != nil {
			t.Fatal("expired history published State/readiness")
		}
		if calls.Load() != 1 {
			t.Fatal("expired recovery did not perform exactly original historical read")
		}
		afterPending, _ := os.ReadFile(pendingPath)
		var root map[string]json.RawMessage
		json.Unmarshal(afterPending, &root)
		var finalization string
		json.Unmarshal(root["finalization_json"], &finalization)
		if finalization != prepared.FinalizationJSON {
			t.Fatal("expired recovery changed finalization identity")
		}
		var h struct {
			ReceiptJSON string `json:"receipt_json"`
			Attempts    int    `json:"attempts"`
		}
		var hs string
		json.Unmarshal(root["history_json"], &hs)
		json.Unmarshal([]byte(hs), &h)
		got, err := contract.DecodeHistoryReceiptWire([]byte(h.ReceiptJSON), preparedChallengeForTest(t, pendingPath))
		if err != nil || got != receipt || got.Receipt.AccessExpiresAtMS != result.Receipt.AccessExpiresAtMS {
			t.Fatal("expired history renewed original receipt", err)
		}
		recovery.Close()
		if owner, err := New(Config{StateDir: dir}); err == nil {
			owner.Close()
			t.Fatal("expired prepared wrapper relaxed normal loader")
		}
		if _, err := LoadState(dir); err == nil {
			t.Fatal("expired prepared wrapper relaxed detached loader")
		}
		again, err := openNativeFinalizationRecovery(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer again.Close()
		// Reopen cannot refund a slot or renew the original expiry.
		if err := again.resume(context.Background(), time.Second, 2); err == nil {
			t.Fatal("reopen renewed expired original access")
		}
		stateFinal, _ := os.ReadFile(statePath(dir))
		if !bytes.Equal(stateBefore, stateFinal) || again.daemon.stateOwner.saved.native != nil {
			t.Fatal("expired reopen changed sole State")
		}
		expected := int32(2)
		if seven {
			expected = 1
		}
		if calls.Load() != expected {
			t.Fatal("expired reopen changed fixed history budget", calls.Load(), expected)
		}
		return
	}
	err = recovery.resume(context.Background(), time.Second, 2)
	if stage != "success" {
		if err == nil {
			t.Fatal("failed prepared history finalized", stage)
		}
		after, _ := os.ReadFile(statePath(dir))
		if !bytes.Equal(before, after) {
			t.Fatal("failed fresh history changed native State")
		}
		recovery.Close()
		os.WriteFile(policyPath, policyRaw, 0600)
		if owner, err := New(Config{StateDir: dir}); err == nil {
			owner.Close()
			t.Fatal("record8 wrapper relaxed normal loader")
		}
		again, err := openNativeFinalizationRecovery(dir)
		if err != nil {
			t.Fatal("strict prepared retry wrapper blocked explicit owner", err)
		}
		again.Close()
		return
	}
	if err != nil {
		t.Fatal("same-attempt prepared recovery", err)
	}
	saved := recovery.daemon.stateOwner.saved
	if saved.native == nil || saved.PublicKey != key || saved.native.Document.Receipt != result.Receipt || saved.native.Document.ReceiptDigest != result.ReceiptDigest || saved.native.Document.Secret == "" {
		t.Fatal("recovery changed original identity/projection")
	}
	if recovery.daemon.enrollmentPreparation.confirmed != nil || !recovery.daemon.enrollmentPreparation.historical {
		t.Fatal("recovery invented confirmation")
	}
	afterPending, _ := os.ReadFile(pendingPath)
	var completed struct {
		Version          int
		Phase            string
		FinalizationJSON string `json:"finalization_json"`
	}
	json.Unmarshal(afterPending, &completed)
	if completed.Version != 8 || completed.Phase != "native-finalization-completed" || completed.FinalizationJSON != prepared.FinalizationJSON || calls.Load() != 1 {
		t.Fatal("prepared recovery replaced ID/bindings or made extra sends")
	}
	recovery.Close()
	if stale, err := openNativeFinalizationRecovery(dir); err == nil {
		stale.Close()
		t.Fatal("completed native State admitted as pending recovery")
	}
	reopened, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("record8 existing native State reopen", err)
	}
	defer reopened.Close()
	if err := reopened.reconcileNativeFinalization(); err != nil {
		t.Fatal(err)
	}
	testNativeCurrentClient(t, reopened, policyPath, policyRaw, install)
	if _, err := os.Stat(statePath(dir)); err != nil {
		t.Fatal(err)
	}
}

func preparedChallengeForTest(t *testing.T, path string) contract.Challenge {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Workflow string `json:"workflow_json"`
	}
	if json.Unmarshal(raw, &root) != nil {
		t.Fatal("pending root")
	}
	var workflow struct {
		Challenge string `json:"challenge_json"`
	}
	if json.Unmarshal([]byte(root.Workflow), &workflow) != nil {
		t.Fatal("pending workflow")
	}
	c, err := contract.DecodeChallengeWire([]byte(workflow.Challenge))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
