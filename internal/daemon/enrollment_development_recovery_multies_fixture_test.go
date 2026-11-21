//go:build mnet_enrollment_development && multies_native_fixture

package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func init() {
	developmentPreparedRecoveryFixture = func(t *testing.T, dir string) {
		t.Helper()
		root := filepath.Dir(dir)
		checkDevelopmentRecoveryCLI(t, root)
		before, err := InspectDisposableEnrollment(root)
		if err != nil || before.Recovery != "prepared-original-state" || before.JoiningSaved || before.NetworkVerified {
			t.Fatal("actual prepared inspection", before, err)
		}
		var recovered DevelopmentRecoveryResult
		if binary := os.Getenv("MNET_RETAINED_CLI_BINARY"); binary != "" {
			raw, e := exec.Command(binary, "--disposable-root", root, "--recover-retained").Output()
			err = e
			if e == nil {
				err = json.Unmarshal(raw, &recovered)
			}
		} else {
			recovered, err = RecoverDisposableEnrollment(context.Background(), root)
		}
		if binary := os.Getenv("MNET_RETAINED_CLI_BINARY"); binary != "" {
			// Reset is forbidden: a second saved invocation must be an idempotent no-lookup inspection.
			raw, e := exec.Command(binary, "--disposable-root", root, "--recover-retained").Output()
			if e != nil {
				t.Fatal("saved CLI", e)
			}
			var repeat DevelopmentRecoveryResult
			if json.Unmarshal(raw, &repeat) != nil || !repeat.Inspection.JoiningSaved || repeat.NetworkVerified || repeat.JoiningFinalized || repeat.HistoricalRecovered {
				t.Fatal("saved CLI misrepresented outcome")
			}
		}
		if err != nil || !recovered.HistoricalRecovered || !recovered.JoiningFinalized || recovered.NetworkVerified {
			t.Fatal("actual prepared same-attempt recovery", recovered, err)
		}
		saved, err := InspectDisposableEnrollment(root)
		if err != nil || !saved.JoiningSaved || saved.NetworkVerified || saved.Recovery != "none-saved" {
			t.Fatal("actual saved historical inspection", saved, err)
		}

		// A well-shaped saved credential with a different original-State revision
		// cannot inherit the journal's finalization, even though both parse alone.
		statePath := filepath.Join(dir, "state.json")
		stateRaw, e := os.ReadFile(statePath)
		if e != nil {
			t.Fatal(e)
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(stateRaw, &fields)
		var nativeText string
		json.Unmarshal(fields["native_enrollment_v1"], &nativeText)
		var native nativeSavedCredential
		json.Unmarshal([]byte(nativeText), &native)
		native.OriginalStateRevision = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
		wrong, _ := json.Marshal(native)
		fields["native_enrollment_v1"], _ = json.Marshal(string(wrong))
		tampered, _ := json.Marshal(fields)
		os.WriteFile(statePath, tampered, 0600)
		if _, e := InspectDisposableEnrollment(root); e == nil {
			t.Fatal("saved original-State mismatch accepted")
		}
		if _, e := RecoverDisposableEnrollment(context.Background(), root); e == nil {
			t.Fatal("saved original-State mismatch recovered")
		}
		after, _ := os.ReadFile(statePath)
		if !bytes.Equal(after, tampered) {
			t.Fatal("tampered saved State repaired")
		}
		os.WriteFile(statePath, stateRaw, 0600)
		t.Log("actual prepared original-State recovery finalized under unchanged receipt; saved joining inspected, network remains unverified")
	}

	developmentRecoveryFixtureSetup = func(t *testing.T, dir string, p config.LocalConfigV2, l registrar.WorkflowLimits) string {
		t.Helper()
		root := filepath.Join(dir, "recovery-setup")
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		policy, _ := json.Marshal(p)
		limits, _ := json.Marshal(l)
		for name, raw := range map[string][]byte{"DISPOSABLE-MNET-SETUP": []byte("mnet-disposable-enrollment-v1\n"), "enrollment-policy-v2.json": policy, "workflow-limits.json": limits} {
			if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
		return filepath.Join(root, "native")
	}
	developmentRecoveryFixtureRun = func(t *testing.T, dir, actualDir string, before []byte) bool {
		t.Helper()
		root := filepath.Dir(actualDir)
		checkDevelopmentRecoveryCLI(t, root)
		status, err := InspectDisposableEnrollment(root)
		if err != nil || status.Recovery != "authenticated-history" || status.JoiningSaved || status.NetworkVerified {
			t.Fatal("actual submit-unknown local inspection", status, err)
		}
		// Real original15s fixture interval ends; no renewal/newapproval allowed.
		time.Sleep(16 * time.Second)
		var history contract.HistoryReceiptWire
		result, err := recoverDisposableEnrollment(context.Background(), root, &history)
		if err != nil || !result.HistoricalRecovered || result.JoiningFinalized || result.NetworkVerified || !result.Inspection.OriginalLeaseEnded {
			t.Fatal("actual same-attempt expired recovery", result, err)
		}
		after, err := os.ReadFile(filepath.Join(actualDir, "state.json"))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("historical recovery changed original State")
		}
		if _, err := os.Stat(filepath.Join(root, "network-result.html")); !os.IsNotExist(err) {
			t.Fatal("recovery created network result")
		}
		outcome, _ := json.Marshal(struct {
			History                                           contract.HistoryReceiptWire
			NativeHistoryRecovered, NativeProofStateUnchanged bool
		}{history, true, true})
		if err := writeFixtureAtomic(dir, "native-workflow-outcome.json", outcome); err != nil {
			t.Fatal(err)
		}
		t.Log("actual expired submit-unknown development inspection/recovery: original State/key unchanged, authenticated same-attempt history only, no finalization/network")
		return true
	}
}

func checkDevelopmentRecoveryCLI(t *testing.T, root string) {
	t.Helper()
	binary := os.Getenv("MNET_RETAINED_CLI_BINARY")
	if binary == "" {
		return
	}
	raw, err := exec.Command(binary, "--disposable-root", root, "--inspect-retained").Output()
	if err != nil {
		t.Fatal("actual inspect CLI", err)
	}
	var status DevelopmentRecoveryInspection
	if json.Unmarshal(raw, &status) != nil || status.NetworkVerified || status.JoiningSaved {
		t.Fatal("actual retained inspection CLI misrepresented State")
	}
}

func init() {
	developmentBookkeepingFixture = func(t *testing.T, d *Daemon) *Daemon {
		t.Helper()
		dir := d.stateOwner.lease.Parent()
		root := filepath.Dir(dir)
		originalRename := d.stateOwner.fs.rename
		d.stateOwner.fs.rename = func(a, b string) error {
			if e := originalRename(a, b); e != nil {
				return e
			}
			return errNativeInjected
		}
		if e := d.finalizeNativeEnrollment(context.Background()); e == nil {
			t.Fatal("post-rename fault did not interrupt finalization")
		}
		stateBefore, e := os.ReadFile(d.stateOwner.path)
		if e != nil {
			t.Fatal(e)
		}
		var image State
		if unmarshalOwnedState(stateBefore, &image) != nil || image.native == nil {
			t.Fatal("actual post-rename sole State missing")
		}
		originalDoc := image.native.Document
		stateInfoBefore, _ := os.Stat(d.stateOwner.path)
		d.Close()
		window := filepath.Join(os.Getenv("MNET_WORKFLOW_FIXTURE_DIR"), "bookkeeping-only-window")
		if e := os.WriteFile(window, []byte("explicit local inspect/recover window"), 0600); e != nil {
			t.Fatal(e)
		}
		defer os.Remove(window)
		pendingBefore, e := os.ReadFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"))
		if e != nil {
			t.Fatal(e)
		}
		status, e := InspectDisposableEnrollment(root)
		if e != nil || !status.JoiningSaved || !status.BookkeepingPending || status.NetworkVerified || status.Recovery != "saved-bookkeeping-pending" {
			t.Fatal("post-rename inspection", status, e)
		}
		stateAfter, _ := os.ReadFile(filepath.Join(dir, "state.json"))
		pendingAfter, _ := os.ReadFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"))
		if !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(pendingBefore, pendingAfter) {
			t.Fatal("post-rename inspection mutated images")
		}

		// Invalid variants of the actual crash image must fail without repair.
		for _, name := range []string{"state.json", "enrollment-pending/enrollment-pending-v1.json"} {
			path := filepath.Join(dir, name)
			original, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			for _, remove := range []bool{false, true} {
				if remove {
					os.Remove(path)
				} else {
					os.WriteFile(path, []byte("{}"), 0600)
				}
				if _, e := InspectDisposableEnrollment(root); e == nil {
					t.Fatal("invalid crash image inspected", name, remove)
				}
				if _, e := RecoverDisposableEnrollment(context.Background(), root); e == nil {
					t.Fatal("invalid crash image repaired", name, remove)
				}
				if remove {
					if _, e := os.Stat(path); !os.IsNotExist(e) {
						t.Fatal("missing crash image recreated")
					}
				} else {
					raw, _ := os.ReadFile(path)
					if !bytes.Equal(raw, []byte("{}")) {
						t.Fatal("invalid crash image changed")
					}
				}
				os.WriteFile(path, original, 0600)
			}
		}

		originalRevision := image.native.OriginalStateRevision
		image.native.OriginalStateRevision = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
		bad, _ := marshalOwnedState(&image)
		os.WriteFile(filepath.Join(dir, "state.json"), bad, 0600)
		if _, e := InspectDisposableEnrollment(root); e == nil {
			t.Fatal("mismatched saved-prepared revision accepted")
		}
		if _, e := RecoverDisposableEnrollment(context.Background(), root); e == nil {
			t.Fatal("mismatched saved-prepared revision repaired")
		}
		observed, _ := os.ReadFile(filepath.Join(dir, "state.json"))
		if !bytes.Equal(observed, bad) {
			t.Fatal("mismatched revision mutated")
		}
		image.native.OriginalStateRevision = originalRevision
		os.WriteFile(filepath.Join(dir, "state.json"), stateBefore, 0600)
		// Capture metadata after test-only deliberate invalid-image edits/restoration.
		stateInfoBefore, _ = os.Stat(filepath.Join(dir, "state.json"))
		if os.Getenv("MNET_BOOKKEEPING_EXPIRED") == "1" {
			wait := time.Until(time.UnixMilli(originalDoc.Receipt.AccessExpiresAtMS)) + 10*time.Millisecond
			if wait > 0 {
				time.Sleep(wait)
			}
		}
		if os.Getenv("MNET_BOOKKEEPING_BROWSER") == "1" {
			binary := os.Getenv("MNET_RETAINED_CLI_BINARY")
			if _, e := exec.Command(binary, "--disposable-root", root, "--inspect-retained", "--open-browser").Output(); e != nil {
				t.Fatal("bookkeeping pending CLI page", e)
			}
		}
		var result DevelopmentRecoveryResult
		if binary := os.Getenv("MNET_RETAINED_CLI_BINARY"); binary != "" {
			args := []string{"--disposable-root", root, "--recover-retained"}
			if os.Getenv("MNET_BOOKKEEPING_BROWSER") == "1" {
				args = append(args, "--open-browser")
			}
			raw, e := exec.Command(binary, args...).Output()
			if e != nil {
				t.Fatal("bookkeeping CLI", e)
			}
			if json.Unmarshal(raw, &result) != nil {
				t.Fatal("bookkeeping CLI output")
			}
		} else {
			result, e = RecoverDisposableEnrollment(context.Background(), root)
			if e != nil {
				t.Fatal(e)
			}
		}
		if os.Getenv("MNET_BOOKKEEPING_EXPIRED") == "1" && !result.Inspection.OriginalLeaseEnded {
			t.Fatal("expired bookkeeping renewed interval")
		}
		if !result.BookkeepingCompleted || result.HistoricalRecovered || result.JoiningFinalized || result.NetworkVerified || result.Inspection.BookkeepingPending || !result.Inspection.JoiningSaved {
			t.Fatal("bookkeeping result misrepresented", result)
		}
		stateAfter, _ = os.ReadFile(filepath.Join(dir, "state.json"))
		if !bytes.Equal(stateBefore, stateAfter) {
			t.Fatal("bookkeeping rewrote State")
		}

		stateInfoAfter, _ := os.Stat(filepath.Join(dir, "state.json"))
		if !os.SameFile(stateInfoBefore, stateInfoAfter) || stateInfoBefore.Mode() != stateInfoAfter.Mode() || !stateInfoBefore.ModTime().Equal(stateInfoAfter.ModTime()) {
			t.Fatal("bookkeeping changed sole-State metadata")
		}
		pendingFinal, _ := os.ReadFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"))
		var beforeFields, afterFields map[string]json.RawMessage
		if json.Unmarshal(pendingBefore, &beforeFields) != nil || json.Unmarshal(pendingFinal, &afterFields) != nil {
			t.Fatal("bookkeeping journal shape")
		}
		if string(afterFields["phase"]) != `"native-finalization-completed"` {
			t.Fatal("bookkeeping did not complete phase")
		}
		delete(beforeFields, "phase")
		delete(afterFields, "phase")
		if !reflect.DeepEqual(beforeFields, afterFields) {
			t.Fatal("bookkeeping changed more than completion phase")
		}
		var after State
		if unmarshalOwnedState(stateAfter, &after) != nil || after.native == nil || after.native.Document != originalDoc {
			t.Fatal("bookkeeping replaced receipt/key/expiry")
		}
		repeat, e := RecoverDisposableEnrollment(context.Background(), root)
		if e != nil || repeat.BookkeepingCompleted || repeat.HistoricalRecovered || repeat.JoiningFinalized || repeat.NetworkVerified {
			t.Fatal("bookkeeping repeat", repeat, e)
		}
		reopened, e := New(Config{StateDir: dir})
		if e != nil {
			t.Fatal(e)
		}
		t.Log("actual post-rename sole State preserved; explicit bookkeeping-only CLI completed exact journal, no lookup/new authority")
		return reopened
	}
}
