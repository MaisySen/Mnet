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
