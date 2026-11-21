package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/netmap"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Called only after actual normal-TLS own exchange and actual durable key proof
// plus native confirmation in TestNativeWorkflowActualDurableKeyTLSChild.
func testActualNativeFinalization(t *testing.T, d *Daemon, policyPath string, policyRaw, before []byte, stage string, install func(http.HandlerFunc)) {
	t.Helper()
	dir := d.stateOwner.lease.Parent()
	oldFS := d.stateOwner.fs
	originalKey := d.stateOwner.saved.PublicKey
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if d.finalizeNativeEnrollment(nil) == nil || d.finalizeNativeEnrollment(canceled) == nil {
		t.Fatal("nil/canceled finalization admitted")
	}
	if raw, _ := os.ReadFile(d.stateOwner.path); !bytes.Equal(raw, before) {
		t.Fatal("canceled admission changed sole State")
	}
	if stage == "policy-before" {
		os.WriteFile(policyPath, append(policyRaw, ' '), 0600)
	}
	switch stage {
	case "create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync":
		d.stateOwner.fs.create = func(dir string) (stateTemp, error) {
			if stage == "create" {
				return nil, errNativeInjected
			}
			f, err := oldFS.create(dir)
			if err != nil {
				return nil, err
			}
			return &nativeFaultTemp{f, stage}, nil
		}
		if stage == "rename" {
			d.stateOwner.fs.rename = func(string, string) error { return errNativeInjected }
		}
		if stage == "after-rename" {
			d.stateOwner.fs.rename = func(a, b string) error {
				if err := oldFS.rename(a, b); err != nil {
					return err
				}
				return errNativeInjected
			}
		}
		if stage == "dirsync" {
			d.stateOwner.fs.syncDir = func(string) error { return errNativeInjected }
		}
	}
	if stage == "policy-during" {
		real := d.stateOwner.fs.create
		d.stateOwner.fs.create = func(dir string) (stateTemp, error) {
			f, err := real(dir)
			os.WriteFile(policyPath, append(policyRaw, ' '), 0600)
			return f, err
		}
	}
	if stage == "policy-after" {
		d.stateOwner.fs.rename = func(a, b string) error {
			err := oldFS.rename(a, b)
			if err == nil {
				os.WriteFile(policyPath, append(policyRaw, ' '), 0600)
			}
			return err
		}
	}

	if stage == "expired-during" {
		real := d.stateOwner.fs.create
		d.stateOwner.fs.create = func(dir string) (stateTemp, error) {
			f, err := real(dir)
			time.Sleep(5100 * time.Millisecond)
			return f, err
		}
	}
	if stage == "expired-after" {
		d.stateOwner.fs.rename = func(a, b string) error {
			err := oldFS.rename(a, b)
			if err == nil {
				time.Sleep(5100 * time.Millisecond)
			}
			return err
		}
	}
	err := d.finalizeNativeEnrollment(context.Background())
	good := stage == "success"
	if good && err != nil {
		t.Fatal("actual finalization", err)
	}
	if !good && err == nil {
		t.Fatal("failed finalization reported success", stage)
	}
	published := good || stage == "after-rename" || stage == "dirsync" || stage == "policy-after" || stage == "expired-after"
	uncertain := stage == "after-rename" || stage == "dirsync" || stage == "policy-after" || stage == "expired-after"
	if uncertain && !errors.Is(err, ErrStateUncertain) {
		t.Fatal("lost native publication uncertainty", err)
	}
	after, _ := os.ReadFile(d.stateOwner.path)
	if !published && !bytes.Equal(before, after) {
		t.Fatal("rollback-safe finalization changed native State")
	}
	if published && bytes.Equal(before, after) {
		t.Fatal("expected whole published native image")
	}
	if d.cc != nil || d.dev != nil || d.state.NodeToken != "" || d.state.NodeID != "" || d.state.AuthKey != "" {
		t.Fatal("native finalization used legacy authority or tunnel")
	}
	if _, err := d.nativeCurrentMap(context.Background(), controlclient.RegistrarLimits{Timeout: 1, MaxConcurrent: 0, MaxRequests: 1}); err == nil {
		t.Fatal("invalid limits/uncertain state made request")
	}

	if !good {
		d.stateOwner.fs = oldFS
		safeRetry := stage == "create" || stage == "write" || stage == "short" || stage == "sync" || stage == "close" || stage == "rename"
		if safeRetry {
			pendingPath := dir + "/enrollment-pending/enrollment-pending-v1.json"
			marker, _ := os.ReadFile(pendingPath)
			if err := d.finalizeNativeEnrollment(context.Background()); err != nil {
				t.Fatal("same live original confirmed intent retry", err)
			}
			markerAfter, _ := os.ReadFile(pendingPath)
			var old, new map[string]json.RawMessage
			json.Unmarshal(marker, &old)
			json.Unmarshal(markerAfter, &new)
			if !bytes.Equal(old["finalization_json"], new["finalization_json"]) {
				t.Fatal("rollback-safe retry changed finalization identity")
			}
			published = true
			after, _ = os.ReadFile(d.stateOwner.path)
		} else if err := d.finalizeNativeEnrollment(context.Background()); err == nil {
			t.Fatal("uncertain/stale finalization replayed")
		}
	}
	d.Close()
	if stage == "policy-after" || stage == "policy-before" || stage == "policy-during" {
		os.WriteFile(policyPath, policyRaw, 0600)
	}
	loaded, loadErr := LoadState(dir)
	if loadErr != nil {
		if !published && stage != "policy-before" {
			return
		} // native-prepared + original legacy State is held for explicit history recovery
		t.Fatal("whole owning state reopen", loadErr)
	}
	if loaded.PublicKey != originalKey {
		t.Fatal("finalization changed actual native key")
	}
	if published {
		if !loaded.nativeDetached || loaded.native != nil {
			t.Fatal("LoadState exposed native credential")
		}
		if err := loaded.Save(dir); err == nil {
			t.Fatal("detached native snapshot rewrote credential")
		}
	}
	reopened, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !published {
		if err := reopened.reconcileNativeFinalization(); err == nil {
			t.Fatal("pending-only recreated authority")
		}
		return
	}
	n := reopened.stateOwner.saved.native
	if n == nil || n.Document.Secret == "" {
		t.Fatal("sole private native credential not loaded")
	}
	public := reopened.State()
	raw, _ := json.Marshal(public)
	if public.native != nil || strings.Contains(string(raw), n.Document.Secret) || strings.Contains(string(raw), "native_enrollment_v1") {
		t.Fatal("State snapshot serialized native credential")
	}
	if err := reopened.reconcileNativeFinalization(); err != nil {
		t.Fatal("exact existing State/pending recovery", err)
	}
	// Multiple historical completion callers cannot publish State or rotate IDs.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := reopened.reconcileNativeFinalization(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	stable, _ := os.ReadFile(reopened.stateOwner.path)
	if !bytes.Equal(stable, after) {
		t.Fatal("reconciliation rewrote sole State")
	}
	if err := reopened.Up("", "", "", ""); err == nil {
		t.Fatal("native fell into legacy startup")
	}
	if _, err := reopened.Prepare("", "", "", ""); err == nil {
		t.Fatal("native fell into legacy mobile prepare")
	}
	if err := reopened.join(context.Background(), "key"); err == nil {
		t.Fatal("native fell into legacy join")
	}
	if err := reopened.SetRouting(true, nil, ""); err == nil {
		t.Fatal("native acquired legacy route capability")
	}
	if raw, _ := os.ReadFile(reopened.stateOwner.path); !bytes.Equal(raw, stable) {
		t.Fatal("refused legacy action changed State")
	}

	if stage == "success" {
		testNativeCurrentClient(t, reopened, policyPath, policyRaw, install)
	}
	// Missing/mismatched pending is not replaced from the document in sole State.
	pendingPath := dir + "/enrollment-pending/enrollment-pending-v1.json"
	pendingRaw, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatal(err)
	}
	os.Rename(pendingPath, pendingPath+".held")
	if err := reopened.reconcileNativeFinalization(); err == nil {
		t.Fatal("missing pending accepted")
	}
	os.Rename(pendingPath+".held", pendingPath)
	if raw, _ := os.ReadFile(pendingPath); !bytes.Equal(raw, pendingRaw) {
		t.Fatal("missing pending check rewrote history")
	}
	if stage == "success" {
		reopened.Close()
		// Completed pending with absent/legacy State must fail ALL owning loaders,
		// not create a replacement native key or offer a legacy fallback.
		os.WriteFile(statePath(dir), before, 0600)
		if _, err := LoadState(dir); err == nil {
			t.Fatal("completed pending accepted old legacy State")
		}
		if bad, err := New(Config{StateDir: dir}); err == nil {
			bad.Close()
			t.Fatal("completed pending started legacy owner")
		}
		os.Remove(statePath(dir))
		if bad, err := New(Config{StateDir: dir}); err == nil {
			bad.Close()
			t.Fatal("missing sole State generated alternate owner")
		}
		if _, err := os.Stat(statePath(dir)); !os.IsNotExist(err) {
			t.Fatal("missing native State reconstructed")
		}
		os.WriteFile(statePath(dir), stable, 0600)
		testNativeCredentialSchema(t, dir, stable)
		testNativeCurrentClose(t, dir, install)
	}
}

func testNativeCurrentClient(t *testing.T, d *Daemon, policyPath string, policyRaw []byte, install func(http.HandlerFunc)) {
	t.Helper()
	n := *d.stateOwner.saved.native
	doc := n.Document
	r := doc.Receipt
	var calls atomic.Int32
	var mode atomic.Value
	mode.Store("success")
	install(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.URL.Path != "/api/mnet-node-experimental-v1/map" || req.Header.Get("Authorization") != "Mnet-Node-Credential-V1 "+doc.Secret || req.Header.Get("X-Node-ID") != "" || req.Header.Get("X-Node-Token") != "" {
			t.Error("sole State client used legacy or wrong secret/target")
		}
		var input struct {
			Version       int             `json:"version"`
			ReceiptDigest string          `json:"receipt_digest"`
			Receipt       json.RawMessage `json:"receipt"`
			Endpoints     []string        `json:"endpoints"`
		}
		json.NewDecoder(req.Body).Decode(&input)
		if input.Version != 1 || input.ReceiptDigest != doc.ReceiptDigest {
			t.Error("wrong qualified sole State receipt")
		}
		if mode.Load().(string) == "denied" {
			w.WriteHeader(403)
			return
		}
		if mode.Load().(string) == "policy-inflight" {
			os.WriteFile(policyPath, append(policyRaw, ' '), 0600)
		}
		expiry := time.UnixMilli(r.AccessExpiresAtMS)
		m := netmap.Map{Version: 1, Network: netmap.Network{ID: r.NetworkID}, Self: netmap.Node{ID: r.NodeID, NetworkID: r.NetworkID, PubKey: r.DevicePublicKey, Approved: true, KeyExpiry: &expiry}}
		if mode.Load().(string) == "wrong-key" {
			m.Self.PubKey = "not-own-key"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(struct {
			Version       int         `json:"version"`
			ReceiptDigest string      `json:"receipt_digest"`
			Map           *netmap.Map `json:"map"`
		}{1, doc.ReceiptDigest, &m})
	}))
	limits := controlclient.RegistrarLimits{Timeout: time.Second, MaxConcurrent: 1, MaxRequests: 16}
	got, err := d.nativeCurrentMap(context.Background(), limits)
	if err != nil || got.Self.ID != r.NodeID {
		t.Fatal("actual sole State client map", err)
	}
	mode.Store("wrong-key")
	if got, err := d.nativeCurrentMap(context.Background(), limits); err == nil || got != nil {
		t.Fatal("wrong returned own key accepted")
	}
	mode.Store("policy-inflight")
	if got, err := d.nativeCurrentMap(context.Background(), limits); err == nil || got != nil {
		t.Fatal("changed original policy returned native map")
	}
	os.WriteFile(policyPath, policyRaw, 0600)
	mode.Store("denied")
	before := calls.Load()
	if got, err := d.nativeCurrentMap(context.Background(), limits); !errors.Is(err, controlclient.ErrRegistrarDenied) || got != nil {
		t.Fatal("dedicated native denial", err)
	}
	if calls.Load() != before+1 {
		t.Fatal("denial request count")
	}
	if got, err := d.nativeCurrentMap(context.Background(), limits); !errors.Is(err, controlclient.ErrRegistrarDenied) || got != nil || calls.Load() != before+1 {
		t.Fatal("denied native owner retried or fell back", err)
	}
	if d.cc != nil || d.dev != nil || d.state.NodeToken != "" {
		t.Fatal("current denial triggered legacy rejoin/tunnel")
	}
}

func testNativeCredentialSchema(t *testing.T, dir string, original []byte) {
	t.Helper()
	var root map[string]json.RawMessage
	if json.Unmarshal(original, &root) != nil {
		t.Fatal("own native image")
	}
	var encoded string
	json.Unmarshal(root["native_enrollment_v1"], &encoded)
	for _, mode := range []string{"unknown-envelope", "duplicate-envelope", "missing-envelope", "bad-kind", "unknown-document", "duplicate-document", "wrong-secret", "wrong-finalization", "mixed-token", "mixed-auth", "mixed-server", "mixed-routes", "wrong-own-key", "null-native"} {
		var fields map[string]json.RawMessage
		json.Unmarshal(original, &fields)
		var envelope map[string]json.RawMessage
		json.Unmarshal([]byte(encoded), &envelope)
		var document map[string]json.RawMessage
		json.Unmarshal(envelope["document"], &document)
		switch mode {
		case "unknown-envelope":
			envelope["extra"] = json.RawMessage(`"x"`)
		case "missing-envelope":
			delete(envelope, "policy_revision")
		case "bad-kind":
			document["kind"] = json.RawMessage(`"legacy"`)
		case "unknown-document":
			document["extra"] = json.RawMessage(`"x"`)
		case "wrong-secret":
			document["secret"] = json.RawMessage(`"invalid"`)
		case "wrong-finalization":
			document["finalization_id"] = json.RawMessage(`"invalid"`)
		case "mixed-token":
			fields["node_token"] = json.RawMessage(`"legacy"`)
		case "mixed-auth":
			fields["auth_key"] = json.RawMessage(`"legacy"`)
		case "mixed-server":
			fields["server"] = json.RawMessage(`"https://other.test"`)
		case "mixed-routes":
			fields["advertise_routes"] = json.RawMessage(`["10.0.0.0/24"]`)
		case "wrong-own-key":
			fields["public_key"] = json.RawMessage(`"not-key"`)
		}
		docRaw, _ := json.Marshal(document)
		if mode == "duplicate-document" {
			docRaw = append([]byte(`{"kind":"duplicate",`), docRaw[1:]...)
		}
		envelope["document"] = docRaw
		envRaw, _ := json.Marshal(envelope)
		if mode == "duplicate-envelope" {
			envRaw = append([]byte(`{"policy_revision":"duplicate",`), envRaw[1:]...)
		}
		if mode == "null-native" {
			envRaw = []byte(`null`)
		}
		nativeRaw, _ := json.Marshal(string(envRaw))
		fields["native_enrollment_v1"] = nativeRaw
		broken, _ := json.Marshal(fields)
		if err := os.WriteFile(statePath(dir), broken, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadState(dir); err == nil {
			t.Fatal("corrupt native credential loaded", mode)
		}
		if d, err := New(Config{StateDir: dir}); err == nil {
			d.Close()
			t.Fatal("corrupt native owner created", mode)
		}
		after, _ := os.ReadFile(statePath(dir))
		if !bytes.Equal(after, broken) {
			t.Fatal("corrupt native state repaired", mode)
		}
	}
	if err := os.WriteFile(statePath(dir), original, 0600); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("failed opens leaked owner", err)
	}
	d.Close()
}

func testNativeCurrentClose(t *testing.T, dir string, install func(http.HandlerFunc)) {
	t.Helper()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	started := make(chan struct{})
	canceled := make(chan struct{})
	install(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		close(started)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(3 * time.Second):
		}
	}))
	done := make(chan error, 1)
	go func() {
		got, err := d.nativeCurrentMap(context.Background(), controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 4})
		if got != nil {
			done <- errors.New("Close returned native map")
			return
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("native current request did not start")
	}
	d.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Close accepted native map")
		}
	case <-time.After(time.Second):
		t.Fatal("native Close did not cancel I/O")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("native Close did not reach actual TLS server")
	}
}
