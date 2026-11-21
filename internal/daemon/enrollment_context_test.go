package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func enrollmentContextFixture(t *testing.T) (*Daemon, string, []byte) {
	t.Helper()
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	d.mu.Lock()
	err = d.ensureStateKeyLocked()
	d.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	p := config.LocalConfigV2{Version: 2, Revision: "r1", SourceOrigin: "https://source.test", PrincipalAuthority: "https://principal.test", PrincipalNamespace: contract.PrincipalNamespace, IssuerLabel: "https://issuer.test/path", RegistrarOrigin: "https://registrar.test", Audience: "fixture", ServiceID: "fixture-service", Bindings: []config.LocalBindingV2{{WorkspaceID: "workspace", NetworkID: "network", PrincipalIDs: []string{"local-user"}}}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d.stateOwner.lease.Parent(), "enrollment-policy-v2.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return d, path, raw
}
func captureEnrollmentContext(t *testing.T, d *Daemon) *nativeEnrollmentContext {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	c, err := d.enrollmentContextLocked()
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func checkEnrollmentContext(d *Daemon, c *nativeEnrollmentContext) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.checkEnrollmentContextLocked(c)
}
func TestOwnedEnrollmentContextNoStartupTriggerMissingOffAndNoKeyGeneration(t *testing.T) {
	dir := t.TempDir()
	policy := filepath.Join(dir, "enrollment-policy-v2.json")
	if err := os.WriteFile(policy, []byte(`invalid`), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("New read inactive policy", err)
	}
	defer d.Close()
	d.mu.Lock()
	_, err = d.enrollmentContextLocked()
	d.mu.Unlock()
	if err == nil {
		t.Fatal("missing key allowed context")
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !os.IsNotExist(err) {
		t.Fatal("context created identity")
	}
	d.mu.Lock()
	err = d.ensureStateKeyLocked()
	d.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(policy)
	d.mu.Lock()
	_, err = d.enrollmentContextLocked()
	d.mu.Unlock()
	if !errors.Is(err, errEnrollmentPolicyOff) {
		t.Fatal("missing policy not off", err)
	}
	if _, err := os.Stat(policy); !os.IsNotExist(err) {
		t.Fatal("policy default created")
	}
}
func TestOwnedEnrollmentContextCurrentKeyAndPolicyBoundToSameRetainedOwner(t *testing.T) {
	d, _, _ := enrollmentContextFixture(t)
	c := captureEnrollmentContext(t, d)
	if err := checkEnrollmentContext(d, c); err != nil {
		t.Fatal(err)
	}
	other, _, _ := enrollmentContextFixture(t)
	if err := checkEnrollmentContext(other, c); err == nil {
		t.Fatal("cross-owner snapshot accepted")
	}
	forged := *c
	forged.publicKey = "other"
	if err := checkEnrollmentContext(d, &forged); err == nil {
		t.Fatal("forged key accepted")
	}
	forged = *c
	forged.stateRevision[0] ^= 1
	if err := checkEnrollmentContext(d, &forged); err == nil {
		t.Fatal("forged revision accepted")
	}
	forged = *c
	snapshot, _ := c.policy.Snapshot()
	snapshot.ServiceID = "changed"
	p, err := config.ValidateLocalV2(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	forged.policy = p
	if err := checkEnrollmentContext(d, &forged); err == nil {
		t.Fatal("substituted policy accepted")
	}
	rendered := fmt.Sprintf("%v %#v", c, *c)
	if strings.Contains(rendered, c.publicKey) || strings.Contains(rendered, "local-user") {
		t.Fatal("context formatter leaked private fields")
	}
	d.Close()
	if err := checkEnrollmentContext(d, c); err == nil {
		t.Fatal("closed context usable")
	}
}
func TestOwnedEnrollmentContextRejectsPolicyRemovalMalformedAndEveryByteChange(t *testing.T) {
	for _, kind := range []string{"missing", "invalid", "duplicate", "legacy", "changed", "whitespace", "symlink", "hardlink", "directory", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			d, path, raw := enrollmentContextFixture(t)
			c := captureEnrollmentContext(t, d)
			switch kind {
			case "missing":
				os.Remove(path)
			case "invalid":
				os.WriteFile(path, []byte(`{}`), 0600)
			case "duplicate":
				os.WriteFile(path, append([]byte(`{"version":2,`), raw[1:]...), 0600)
			case "legacy":
				os.WriteFile(path, bytes.Replace(raw, []byte(`"version":2`), []byte(`"version":1`), 1), 0600)
			case "changed":
				os.WriteFile(path, bytes.Replace(raw, []byte(`local-user`), []byte(`other-user`), 1), 0600)
			case "whitespace":
				os.WriteFile(path, append(raw, ' '), 0600)
			case "symlink":
				os.Remove(path)
				target := filepath.Join(t.TempDir(), "policy")
				os.WriteFile(target, raw, 0600)
				if err := os.Symlink(target, path); err != nil {
					t.Skip(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".linked"); err != nil {
					t.Skip(err)
				}
			case "directory":
				os.Remove(path)
				os.Mkdir(path, 0700)
			case "oversize":
				os.WriteFile(path, bytes.Repeat([]byte(" "), config.MaxBytes+1), 0600)
			}
			if err := checkEnrollmentContext(d, c); err == nil {
				t.Fatal("changed policy accepted")
			}
		})
	}
}
func TestOwnedEnrollmentContextRefusesUnsavedOrExternallyChangedState(t *testing.T) {
	for _, kind := range []string{"working-key", "working-config", "disk", "missing", "state-symlink", "fault", "owner-released"} {
		t.Run(kind, func(t *testing.T) {
			d, _, _ := enrollmentContextFixture(t)
			c := captureEnrollmentContext(t, d)
			switch kind {
			case "working-key":
				d.state.PublicKey = "changed"
			case "working-config":
				d.state.NodeName = "unsaved"
			case "disk":
				raw, _ := os.ReadFile(d.stateOwner.path)
				os.WriteFile(d.stateOwner.path, append(raw, ' '), 0600)
			case "missing":
				os.Remove(d.stateOwner.path)
			case "state-symlink":
				raw, _ := os.ReadFile(d.stateOwner.path)
				target := filepath.Join(t.TempDir(), "state")
				os.WriteFile(target, raw, 0600)
				os.Remove(d.stateOwner.path)
				if err := os.Symlink(target, d.stateOwner.path); err != nil {
					t.Skip(err)
				}
			case "fault":
				d.stateOwner.fault = true
			case "owner-released":
				d.stateOwner.lease.Close()
			}
			if err := checkEnrollmentContext(d, c); err == nil {
				t.Fatal("unowned or changed key state accepted")
			}
		})
	}
}
func TestOwnedEnrollmentContextNativeStateWritesInvalidatePriorCapture(t *testing.T) {
	d, _, _ := enrollmentContextFixture(t)
	c := captureEnrollmentContext(t, d)
	d.mu.Lock()
	d.state.NodeName = "saved-next"
	err := d.saveStateLocked()
	d.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkEnrollmentContext(d, c); err == nil {
		t.Fatal("old native state revision accepted")
	}
	next := captureEnrollmentContext(t, d)
	if next.publicKey != c.publicKey || next.stateRevision == c.stateRevision {
		t.Fatal("state transition rotated key or lost revision")
	}
}
