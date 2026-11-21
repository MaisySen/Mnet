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
