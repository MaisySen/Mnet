//go:build mnet_enrollment_development

package daemon

import (
	"context"
	"encoding/json"
	"errors"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"fintex.au/mnet/internal/stateowner"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDisposableInspectionPreparedIsReadOnlyNotLive(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	policy := config.LocalConfigV2{Version: 2, Revision: "fixture", SourceOrigin: "https://127.0.0.1:30001", PrincipalAuthority: "https://principal.test", PrincipalNamespace: contract.PrincipalNamespace, IssuerLabel: "https://issuer.test/path", RegistrarOrigin: "https://127.0.0.1:30002", Audience: "fixture", ServiceID: "fixture-service", Bindings: []config.LocalBindingV2{{WorkspaceID: "workspace", NetworkID: "network", PrincipalIDs: []string{"fixture-person"}}}}
	p, _ := json.Marshal(policy)
	limits := registrar.WorkflowLimits{Timeout: time.Second, MaxChallengeLifetime: time.Second, MaxReserveLifetime: time.Second, Timing: contract.FixtureTiming{MaxObservationAgeMS: 1000, MaxLeaseMS: 1000, ClockUncertaintyMS: 1}}
	l, _ := json.Marshal(limits)
	for name, raw := range map[string][]byte{"DISPOSABLE-MNET-SETUP": []byte("mnet-disposable-enrollment-v1\n"), "enrollment-policy-v2.json": p, "workflow-limits.json": l} {
		if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	err := RunDisposableEnrollment(context.Background(), root, func(ev DevelopmentEnrollmentEvent) error {
		calls++
		if ev.Stage != "waiting-for-browser" {
			t.Fatal("unexpected setup stage")
		}
		return errors.New("explicit fixture stops before browser/source request")
	})
	if err == nil || calls != 1 {
		t.Fatal("prepared fixture not created")
	}
	type image struct {
		raw      string
		mode     os.FileMode
		modified time.Time
	}
	snap := func() map[string]image {
		out := map[string]image{}
		if e := filepath.Walk(root, func(path string, i os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			raw := []byte(nil)
			if i.Mode().IsRegular() {
				var err error
				raw, err = os.ReadFile(path)
				if err != nil {
					return err
				}
			}
			out[path] = image{string(raw), i.Mode(), i.ModTime()}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := snap()
	status, err := InspectDisposableEnrollment(root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != "prepared-listener-not-owned" || status.JoiningSaved || status.NetworkVerified || status.Recovery != "unavailable" {
		t.Fatalf("prepared file misrepresented: %+v", status)
	}
	if _, err := RecoverDisposableEnrollment(context.Background(), root); err == nil {
		t.Fatal("prepared listener was restarted or recovered without eligible history")
	}
	after := snap()
	if len(before) != len(after) {
		t.Fatal("inspection changed directory entries")
	}
	for p, b := range before {
		a, ok := after[p]
		if !ok || a != b {
			t.Fatal("inspection mutated path", p)
		}
	}

	// Invalid retained DATA must neither repair files nor start a replacement.
	for _, name := range []string{"state.json", "enrollment-pending/enrollment-pending-v1.json"} {
		path := filepath.Join(root, "native", name)
		original, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		for _, missing := range []bool{false, true} {
			if missing {
				os.Remove(path)
			} else {
				os.WriteFile(path, []byte("{}"), 0600)
			}
			imageBefore := snap()
			if _, e := InspectDisposableEnrollment(root); e == nil {
				t.Fatal("invalid retained image inspected", name, missing)
			}
			if _, e := RecoverDisposableEnrollment(context.Background(), root); e == nil {
				t.Fatal("invalid retained image recovered", name, missing)
			}
			imageAfter := snap()
			if !reflect.DeepEqual(imageBefore, imageAfter) {
				t.Fatal("invalid image was repaired", name, missing)
			}
			os.WriteFile(path, original, 0600)
		}
	}
	// Valid but different native key cannot bind the retained original journal.
	path := filepath.Join(root, "native", "state.json")
	original, _ := os.ReadFile(path)
	other, _, _ := enrollmentContextFixture(t)
	otherRaw, _ := os.ReadFile(other.stateOwner.path)
	other.Close()
	os.WriteFile(path, otherRaw, 0600)
	if _, e := InspectDisposableEnrollment(root); e == nil {
		t.Fatal("changed valid identity admitted")
	}
	if _, e := RecoverDisposableEnrollment(context.Background(), root); e == nil {
		t.Fatal("changed identity recovered")
	}
	os.WriteFile(path, original, 0600)
	lease, _, err := stateowner.Acquire(filepath.Join(root, "native", "daemon"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDisposableEnrollment(root); err == nil {
		t.Fatal("owned State inspection admitted")
	}
	lease.Close()
	if err := os.WriteFile(filepath.Join(root, "native", "enrollment-policy-v2.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDisposableEnrollment(root); err == nil {
		t.Fatal("changed installed policy accepted")
	}
}
