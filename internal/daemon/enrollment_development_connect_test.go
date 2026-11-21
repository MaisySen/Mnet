//go:build mnet_enrollment_development

package daemon

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"fintex.au/mnet/internal/stateowner"
)

func connectionPersistenceFixture(t *testing.T) (*developmentConnectionOwner, func()) {
	t.Helper()
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	l, _, e := stateowner.Acquire(filepath.Join(dir, "daemon"))
	if e != nil {
		t.Fatal(e)
	}
	d := &Daemon{stateOwner: &nativeStateOwner{lease: l}}
	r := developmentConnectionRecord{Version: 1, Kind: developmentConnectionLimitKind, Phase: "ready", State: "fixture", Total: 16, Setup: 2}
	o := &developmentConnectionOwner{d: d, path: filepath.Join(dir, developmentConnectionRecordName), record: r}
	if e = o.publish(true); e != nil {
		t.Fatal(e)
	}
	return o, func() { l.Close() }
}
func TestDevelopmentConnectionDurableReservationFaults(t *testing.T) {
	for _, stage := range []string{"write", "file-sync", "before-publish", "after-publish", "directory-sync"} {
		t.Run(stage, func(t *testing.T) {
			o, close := connectionPersistenceFixture(t)
			defer close()
			before := append([]byte(nil), o.raw...)
			o.fault = func(s string) error {
				if s == stage {
					return errors.New("injected persistence fault")
				}
				return nil
			}
			if o.transition("ready", "connection-reserved") == nil || !o.frozen {
				t.Fatal("fault not frozen")
			}
			if o.check() == nil {
				t.Fatal("uncertain owner usable")
			}
			if o.transition("connection-reserved", "ready") == nil {
				t.Fatal("refund after fault")
			}
			raw, e := os.ReadFile(o.path)
			if e != nil {
				t.Fatal(e)
			}
			if stage == "after-publish" || stage == "directory-sync" {
				if bytes.Equal(raw, before) {
					t.Fatal("postpublication image missing")
				}
			} else {
				if !bytes.Equal(raw, before) {
					t.Fatal("prepublication changed record")
				}
			}
			// No HTTP/device is invoked by a transition failing at any point. The public
			// caller returns its error before constructing a client or calling Device.
		})
	}
}
func TestDevelopmentConnectionNoRefundOrDoubleSpend(t *testing.T) {
	o, close := connectionPersistenceFixture(t)
	defer close()
	if e := o.transition("ready", "connection-reserved"); e != nil {
		t.Fatal(e)
	}
	if e := o.transition("ready", "connection-reserved"); e == nil {
		t.Fatal("double spend")
	}
	// Even with a supplied source phase, only legal transitions may occur.
	if e := o.transition("connection-reserved", "ready"); e == nil {
		t.Fatal("refund allowed")
	}
}
func TestDevelopmentConnectionConcurrentExistingOwner(t *testing.T) {
	o, close := connectionPersistenceFixture(t)
	defer close()
	if second, e := stateowner.AcquireExistingDevelopment(filepath.Join(filepath.Dir(o.path), "daemon")); e == nil {
		second.Close()
		t.Fatal("concurrent owner admitted")
	}
	if e := o.transition("ready", "connection-reserved"); e != nil {
		t.Fatal(e)
	}
}
func TestDevelopmentConnectionKilledAfterReservation(t *testing.T) {
	if dir := os.Getenv("MNET_TEST_CONNECTION_CRASH_DIR"); dir != "" {
		l, e := stateowner.AcquireExistingDevelopment(filepath.Join(dir, "daemon"))
		if e != nil {
			panic(e)
		}
		raw, e := os.ReadFile(filepath.Join(dir, developmentConnectionRecordName))
		if e != nil {
			panic(e)
		}
		o := &developmentConnectionOwner{d: &Daemon{stateOwner: &nativeStateOwner{lease: l}}, path: filepath.Join(dir, developmentConnectionRecordName), raw: raw}
		// Persistence-only child. No HTTP/client/device constructor exists here.
		o.record = developmentConnectionRecord{Version: 1, Kind: developmentConnectionLimitKind, Phase: "ready", State: "fixture", Total: 16, Setup: 2}
		if e = o.transition("ready", "connection-reserved"); e != nil {
			panic(e)
		}
		os.WriteFile(filepath.Join(dir, "reserved"), []byte("durable before any IO"), 0600)
		select {}
	}
	o, close := connectionPersistenceFixture(t)
	dir := filepath.Dir(o.path)
	close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDevelopmentConnectionKilledAfterReservation$")
	cmd.Env = append(os.Environ(), "MNET_TEST_CONNECTION_CRASH_DIR="+dir)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer cmd.Process.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e := os.Stat(filepath.Join(dir, "reserved")); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no child reservation")
		}
		time.Sleep(time.Millisecond * 10)
	}
	if e := cmd.Process.Signal(syscall.SIGKILL); e != nil {
		t.Fatal(e)
	}
	if e := cmd.Wait(); e == nil {
		t.Fatal("child not killed")
	}
	lease, e := stateowner.AcquireExistingDevelopment(filepath.Join(dir, "daemon"))
	if e != nil {
		t.Fatal(e)
	}
	defer lease.Close()
	raw, e := os.ReadFile(o.path)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(raw, []byte(`"phase":"connection-reserved"`)) || !bytes.Contains(raw, []byte(`"local_connection_reserved":14`)) {
		t.Fatal("crash refunded slice")
	}
}

func TestDevelopmentConnectionSetupUncertainIsNeverAutoReady(t *testing.T) {
	for _, stage := range []string{"write", "file-sync", "before-publish", "after-publish", "directory-sync"} {
		t.Run(stage, func(t *testing.T) {
			o, close := connectionPersistenceFixture(t)
			defer close()
			o.record.Phase = "setup-reserved"
			if e := o.publish(false); e != nil {
				t.Fatal(e)
			}
			o.fault = func(s string) error {
				if s == stage {
					return errors.New("injected setup-ready persistence fault")
				}
				return nil
			}
			if e := o.transition("setup-reserved", "ready"); e == nil || !o.frozen {
				t.Fatal("uncertain setup became usable")
			}
			if e := o.transition("ready", "connection-reserved"); e == nil {
				t.Fatal("same uncertain owner connected")
			}
			raw, e := os.ReadFile(o.path)
			if e != nil {
				t.Fatal(e)
			}
			if stage == "write" || stage == "file-sync" || stage == "before-publish" {
				if !bytes.Contains(raw, []byte(`"phase":"setup-reserved"`)) {
					t.Fatal("stranded setup changed")
				}
			}
			// A whole visible ready record after publication is safe on a later fresh
			// owner: setup2 was reserved durably before setup I/O. Never auto-resume it.
		})
	}
}
