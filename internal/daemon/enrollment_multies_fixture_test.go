//go:build multies_native_fixture

package daemon

// Opt-in cross-repository disposable test. The Multies Vitest coordinator owns
// actual app/DB consent and verified TLS bridge. Files are private fixture IPC,
// not native API or production onboarding. No live account/service is used.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/controlclient"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	source "fintex.au/mnet/internal/enrollmentsource"
)

func TestActualMultiesNativeFixture(t *testing.T) {
	dir := os.Getenv("MNET_NATIVE_FIXTURE_DIR")
	if dir == "" {
		t.Skip("explicit disposable cross-repository coordinator required")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Mode   string               `json:"mode"`
		Policy config.LocalConfigV2 `json:"policy"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal("invalid private fixture")
	}
	if _, err := config.ValidateLocalV2(fixture.Policy); err != nil {
		t.Fatal("invalid independent fixture policy")
	}
	d, err := New(Config{StateDir: filepath.Join(dir, "native")})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.PrepareIdentity(); err != nil {
		t.Fatal(err)
	}
	policyRaw, _ := json.Marshal(fixture.Policy)
	if err := os.WriteFile(filepath.Join(dir, "native", "enrollment-policy-v2.json"), policyRaw, 0600); err != nil {
		t.Fatal(err)
	}
	stateBefore, err := os.ReadFile(d.stateOwner.path)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	launchRaw, _ := json.Marshal(launch)
	if err := writeFixtureAtomic(dir, "launch.json", launchRaw); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "continue")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("coordinator callback deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
	result, exchangeErr := d.exchangeNativeEnrollment(context.Background(), source.Limits{Timeout: 3 * time.Second, MaxConnections: 1})
	phase, err := d.enrollmentPreparation.journal.Status()
	if err != nil {
		t.Fatal(err)
	}
	switch fixture.Mode {
	case "success":
		if exchangeErr != nil || phase != "exchanged-data" || result.Launch != launch {
			t.Fatal("actual handler exchange failed", exchangeErr)
		}
		if !d.enrollmentPreparation.context.policy.AllowsObservedDATA(result.Mapping, result.Provenance.PrincipalAuthority, result.Provenance.PrincipalNamespace, result.Provenance.PrincipalID, fixture.Policy.ServiceID) {
			t.Fatal("observed source outside independent fixture policy")
		}
	case "decline":
		if exchangeErr == nil || phase != "declined-data" {
			t.Fatal("declined callback reached source exchange", exchangeErr)
		}
	case "revoked", "lost-response":
		if !errors.Is(exchangeErr, pending.ErrRemoteUnknown) || phase != "exchange-may-have-started" {
			t.Fatal("source loss/revocation not durable unknown", exchangeErr)
		}
	default:
		t.Fatal("unsupported fixture mode")
	}
	if _, err := d.exchangeNativeEnrollment(context.Background(), source.Limits{Timeout: 3 * time.Second, MaxConnections: 1}); err == nil {
		t.Fatal("second one-shot exchange admitted")
	}
	stateAfter, _ := os.ReadFile(d.stateOwner.path)
	if !bytes.Equal(stateBefore, stateAfter) || d.state.NodeID != "" || d.state.NodeToken != "" || d.cc != nil || d.dev != nil {
		t.Fatal("source DATA altered native authority")
	}
	// Publish only nonsecret outcome metadata, never ticket/code/verifier/session.
	outcome, _ := json.Marshal(map[string]any{"phase": phase, "stateUnchanged": true, "nodeCreated": false, "inputAuthorized": false})
	if err := writeFixtureAtomic(dir, "outcome.json", outcome); err != nil {
		t.Fatal(err)
	}
}
func writeFixtureAtomic(dir, name string, raw []byte) error {
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

// A separate disposable process owns the ACTUAL native State key and callback.
// Control fixture receives only launch + receipt outcome, never the private key
// or presaved credential. Its real TLS service sees the expected bearer header.
var developmentRecoveryFixtureSetup func(*testing.T, string, config.LocalConfigV2, registrar.WorkflowLimits) string
var developmentRecoveryFixtureRun func(*testing.T, string, string, []byte) bool
var developmentPreparedRecoveryFixture func(*testing.T, string)
var developmentBookkeepingFixture func(*testing.T, *Daemon) *Daemon

func TestActualMultiesNativeWorkflowFixture(t *testing.T) {
	dir := os.Getenv("MNET_NATIVE_OWNED_WORKFLOW_DIR")
	if dir == "" {
		t.Skip("explicit disposable control/source coordinator required")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "native-workflow-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Policy                  config.LocalConfigV2
		Limits                  registrar.WorkflowLimits
		Recover                 bool
		FinalizeHistory         bool
		PreparedFinalize        bool
		PreparedHistoryFinalize bool
		Transport               bool
		RelayTransport          bool
	}
	if json.Unmarshal(raw, &f) != nil || !registrar.ValidWorkflowLimits(f.Limits) {
		t.Fatal("invalid independent fixture config")
	}
	if _, err := config.ValidateLocalV2(f.Policy); err != nil {
		t.Fatal(err)
	}
	nativeDir := filepath.Join(dir, "owned-native")
	if os.Getenv("MNET_DEVELOPMENT_RECOVERY_FIXTURE") == "1" {
		if developmentRecoveryFixtureSetup == nil || developmentRecoveryFixtureRun == nil {
			t.Fatal("tagged development recovery fixture missing")
		}
		nativeDir = developmentRecoveryFixtureSetup(t, dir, f.Policy, f.Limits)
	}
	d, err := New(Config{StateDir: nativeDir})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.PrepareIdentity(); err != nil {
		t.Fatal(err)
	}
	policyRaw, _ := json.Marshal(f.Policy)
	if err := os.WriteFile(filepath.Join(nativeDir, "enrollment-policy-v2.json"), policyRaw, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(d.stateOwner.path)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(launch)
	if err := writeFixtureAtomic(dir, "native-launch.json", public); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "native-continue")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native own callback timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	own, err := d.exchangeNativeEnrollment(context.Background(), source.Limits{Timeout: 3 * time.Second, MaxConnections: 1})
	if err != nil || own.Launch != launch {
		t.Fatal("actual native/source exchange", err)
	}
	result, err := d.proveNativeEnrollment(context.Background(), f.Limits)

	if f.Recover {
		if err == nil {
			t.Fatal("lost submit unexpectedly returned receipt")
		}
		phase, phaseErr := d.enrollmentPreparation.journal.Status()
		if phaseErr != nil || phase != "workflow-submit-may-have-started" {
			t.Fatal("submit loss not retained", phase, phaseErr)
		}
		actualDir := d.stateOwner.lease.Parent()
		d.Close()
		if os.Getenv("MNET_DEVELOPMENT_RECOVERY_FIXTURE") == "1" && !f.FinalizeHistory {
			if developmentRecoveryFixtureRun(t, dir, actualDir, before) {
				return
			}
			t.Fatal("development recovery fixture did not complete")
		}
		reopened, err := New(Config{StateDir: actualDir})
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if f.FinalizeHistory {
			testActualMultiesHistoryFinalization(t, reopened, before, actualDir, false, false)
			return
		}
		// Past explicit registrar fixture access/challenge lifetime. History must be
		// returned even after deletion by the source-of-truth test bridge, not renewed.
		time.Sleep(16 * time.Second)
		history, err := reopened.recoverNativeHistoryDATA(context.Background(), 3*time.Second, 2)
		if err != nil {
			t.Fatal("actual reopened native immutable history", err)
		}
		after, err := os.ReadFile(reopened.stateOwner.path)
		if err != nil || !bytes.Equal(before, after) || reopened.state.native != nil || reopened.cc != nil || reopened.dev != nil {
			t.Fatal("history DATA installed authority")
		}
		if err := reopened.finalizeNativeEnrollment(context.Background()); err == nil {
			t.Fatal("history DATA manufactured confirmation")
		}
		outcome, _ := json.Marshal(struct {
			History                   contract.HistoryReceiptWire
			NativeHistoryRecovered    bool
			NativeProofStateUnchanged bool
		}{history, true, true})
		if err := writeFixtureAtomic(dir, "native-workflow-outcome.json", outcome); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil {
		t.Fatal("actual owned native key workflow", err)
	}
	phase, err := d.enrollmentPreparation.journal.Status()
	if err != nil || phase != "workflow-receipt-data" {
		t.Fatal("actual native receipt not pinned", err)
	}
	if _, err := d.proveNativeEnrollment(context.Background(), f.Limits); err == nil {
		t.Fatal("native proof replay")
	}
	after, err := os.ReadFile(d.stateOwner.path)
	if err != nil || !bytes.Equal(before, after) || d.state.NodeID != "" || d.state.NodeToken != "" || d.cc != nil || d.dev != nil {
		t.Fatal("proof DATA installed native State")
	}

	if f.PreparedFinalize || f.PreparedHistoryFinalize {
		testActualMultiesHistoryFinalization(t, d, before, d.stateOwner.lease.Parent(), true, f.PreparedHistoryFinalize)
		return
	}
	// Actual private sole-State finalization from SAME confirmed runtime, not
	// a returned-document installer. Then owner Close/reopen and dedicated map.
	if os.Getenv("MNET_DEVELOPMENT_BOOKKEEPING_FIXTURE") == "1" {
		if developmentBookkeepingFixture == nil {
			t.Fatal("bookkeeping hook missing")
		}
		d = developmentBookkeepingFixture(t, d)
		defer d.Close()
	} else if err := d.finalizeNativeEnrollment(context.Background()); err != nil {
		t.Fatal("actual native finalization", err)
	}
	phase, err = func() (string, error) {
		if d.enrollmentPreparation == nil {
			raw, e := os.ReadFile(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending", "enrollment-pending-v1.json"))
			var p struct{ Phase string }
			if e == nil {
				e = json.Unmarshal(raw, &p)
			}
			return p.Phase, e
		}
		return d.enrollmentPreparation.journal.Status()
	}()
	if err != nil || phase != "native-finalization-completed" {
		t.Fatal("native finalization correlation", err)
	}
	actualDir := d.stateOwner.lease.Parent()
	finalized, err := os.ReadFile(d.stateOwner.path)
	if err != nil || bytes.Equal(before, finalized) {
		t.Fatal("sole State not published")
	}
	snapshot := d.State()
	snapshotRaw, _ := json.Marshal(snapshot)
	if snapshot.native != nil || bytes.Contains(snapshotRaw, []byte(d.stateOwner.saved.native.Document.Secret)) {
		t.Fatal("snapshot leaked registrar credential")
	}
	d.Close()
	reopened, err := New(Config{StateDir: actualDir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.reconcileNativeFinalization(); err != nil {
		t.Fatal("actual owning native finalization reopen", err)
	}

	if os.Getenv("MNET_BOOKKEEPING_EXPIRED") == "1" && os.Getenv("MNET_DEVELOPMENT_BOOKKEEPING_FIXTURE") == "1" {
		if time.Now().UnixMilli() < result.Receipt.AccessExpiresAtMS {
			t.Fatal("expired bookkeeping fixture still live")
		}
		if reopened.cc != nil || reopened.dev != nil {
			t.Fatal("bookkeeping activated network")
		}
		raw, _ := json.Marshal(map[string]any{"Receipt": result, "BookkeepingExpired": true, "NativeOwnedKeyConfirmed": true, "NativeProofStateUnchanged": true, "NativeStateFinalized": true})
		if e := writeFixtureAtomic(dir, "native-workflow-outcome.json", raw); e != nil {
			t.Fatal(e)
		}
		return
	}
	currentLimits := controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 8}
	if os.Getenv("MNET_DEVELOPMENT_NETWORK_SESSION") == "1" {
		currentLimits.MaxRequests = 16
	}
	if f.RelayTransport {
		currentLimits.MaxConcurrent = 2
		currentLimits.MaxRequests = 16
	}
	if err := reopened.nativeCurrentEndpoints(context.Background(), []string{"192.0.2.79:51820"}, currentLimits); err != nil {
		t.Fatal("sole State actual endpoint publication", err)
	}
	current, err := reopened.nativeCurrentMap(context.Background(), currentLimits)
	if err != nil || current.Self.ID != result.Receipt.NodeID || len(current.Peers) != 2 || len(current.Self.Endpoints) != 1 || current.Self.Endpoints[0] != "192.0.2.79:51820" {
		t.Fatal("sole State dedicated client actual peers", err)
	}
	if reopened.cc != nil || reopened.dev != nil {
		t.Fatal("native current map activated legacy/tunnel")
	}
	if f.Transport {
		testActualEnrolledNativeTransport(t, reopened, dir, f.RelayTransport)
	}
	afterCurrent, _ := os.ReadFile(reopened.stateOwner.path)
	if !bytes.Equal(finalized, afterCurrent) {
		t.Fatal("native current map rewrote sole State")
	}
	out, _ := json.Marshal(struct {
		Receipt                   contract.WorkflowReceiptWire
		NativeOwnedKeyConfirmed   bool
		NativeProofStateUnchanged bool
		NativeStateFinalized      bool
		NativeCurrentPeers        int
		NativeEncryptedDirect     bool
		NativeEncryptedRelay      bool
	}{result, true, true, true, len(current.Peers), f.Transport && !f.RelayTransport, f.RelayTransport})
	if err := writeFixtureAtomic(dir, "native-workflow-outcome.json", out); err != nil {
		t.Fatal(err)
	}
}

// Actual lost-submit daemon owning reopen -> fresh authenticated history delivery
// -> distinct original-expiry finalization -> owning reopen -> current maps.
func testActualMultiesHistoryFinalization(t *testing.T, d *Daemon, before []byte, dir string, prepared, seven bool) {
	t.Helper()
	originalPending := []byte(nil)
	if prepared {
		if seven {
			d.Close()
			var err error
			d, err = New(Config{StateDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.recoverNativeHistoryOwned(context.Background(), 3*time.Second, 2); err != nil {
				t.Fatal("actual record7 setup owned delivery", err)
			}
		}
		real := d.stateOwner.fs.create
		d.stateOwner.fs.create = func(string) (stateTemp, error) { return nil, errNativeInjected }
		if d.finalizeNativeEnrollment(context.Background()) == nil {
			t.Fatal("failed publication fixture succeeded")
		}
		d.stateOwner.fs.create = real
		originalPending, _ = os.ReadFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"))
		if raw, _ := os.ReadFile(d.stateOwner.path); !bytes.Equal(raw, before) {
			t.Fatal("failed publication changed State")
		}
		d.Close()
		if owner, err := New(Config{StateDir: dir}); err == nil {
			owner.Close()
			t.Fatal("normal loader admitted prepared-only intent")
		}
		if _, err := LoadState(dir); err == nil {
			t.Fatal("detached loader admitted prepared-only intent")
		}
		if os.Getenv("MNET_DEVELOPMENT_RECOVERY_FIXTURE") == "1" {
			if developmentPreparedRecoveryFixture == nil {
				t.Fatal("tagged prepared recovery fixture missing")
			}
			developmentPreparedRecoveryFixture(t, dir)
			var err error
			d, err = New(Config{StateDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
		} else {
			recovery, err := openNativeFinalizationRecovery(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer recovery.Close()
			if err := recovery.resume(context.Background(), 3*time.Second, 2); err != nil {
				t.Fatal("actual prepared restart same-attempt history", err)
			}
			d = recovery.daemon
		}
		var old, new struct {
			FinalizationJSON string `json:"finalization_json"`
		}
		json.Unmarshal(originalPending, &old)
		now, _ := os.ReadFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"))
		json.Unmarshal(now, &new)
		if old.FinalizationJSON == "" || old.FinalizationJSON != new.FinalizationJSON {
			t.Fatal("prepared restart replaced original finalization identity")
		}
	}
	if !prepared {
		if d.finalizeNativeEnrollment(context.Background()) == nil {
			t.Fatal("reopened DATA finalized without delivery")
		}
		if err := d.recoverNativeHistoryOwned(context.Background(), 3*time.Second, 2); err != nil {
			t.Fatal("actual authenticated history delivery", err)
		}
		if d.enrollmentPreparation == nil || !d.enrollmentPreparation.historical || d.enrollmentPreparation.confirmed != nil {
			t.Fatal("history laundered into proof confirmation")
		}
		raw, _ := os.ReadFile(d.stateOwner.path)
		if !bytes.Equal(before, raw) || d.state.native != nil {
			t.Fatal("delivery prematurely changed State")
		}
		if err := d.finalizeNativeEnrollment(context.Background()); err != nil {
			t.Fatal("actual lost-submit finalization", err)
		}
	}
	doc := d.stateOwner.saved.native.Document
	history := contract.HistoryReceiptWire{Version: 1, Receipt: doc.Receipt, ReceiptDigest: doc.ReceiptDigest}
	finalized, _ := os.ReadFile(d.stateOwner.path)
	if bytes.Equal(before, finalized) || d.cc != nil || d.dev != nil {
		t.Fatal("finalization did not use isolated sole State")
	}
	d.Close()
	reopened, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("recovered finalized reopen", err)
	}
	defer reopened.Close()
	if err := reopened.reconcileNativeFinalization(); err != nil {
		t.Fatal(err)
	}
	if err := reopened.nativeCurrentEndpoints(context.Background(), []string{"192.0.2.79:51820"}, controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 8}); err != nil {
		t.Fatal(err)
	}
	current, err := reopened.nativeCurrentMap(context.Background(), controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 8})
	if err != nil || current.Self.ID != history.Receipt.NodeID || len(current.Peers) != 2 || len(current.Self.Endpoints) != 1 {
		t.Fatal("recovered original credential not current with actual approved peers", err)
	}
	after, _ := os.ReadFile(reopened.stateOwner.path)
	if !bytes.Equal(finalized, after) || reopened.cc != nil || reopened.dev != nil {
		t.Fatal("recovered current access changed State or activated tunnel")
	}
	outcome, _ := json.Marshal(struct {
		History                   contract.HistoryReceiptWire
		NativeOwnedKeyConfirmed   bool
		NativeHistoryRecovered    bool
		NativeProofStateUnchanged bool
		NativeStateFinalized      bool
		NativeCurrentPeers        int
	}{history, false, true, true, true, 2})
	outcomeDir := filepath.Dir(dir)
	if os.Getenv("MNET_DEVELOPMENT_RECOVERY_FIXTURE") == "1" {
		outcomeDir = filepath.Dir(outcomeDir)
	}
	if err := writeFixtureAtomic(outcomeDir, "native-workflow-outcome.json", outcome); err != nil {
		t.Fatal(err)
	}
}
