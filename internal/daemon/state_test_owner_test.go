package daemon

import "testing"

// Old device/routing tests construct synthetic network data, but even those
// objects now acquire a real temporary file owner. Production has no nil-owner
// health bypass or test trust constructor.
func attachActualTestOwner(t *testing.T, d *Daemon) {
	t.Helper()
	owner, err := openStateOwner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d.stateOwner = owner
	d.cfg.StateDir = owner.lease.Parent()
	if d.state == nil {
		s := cloneState(owner.saved)
		d.state = &s
	}
	t.Cleanup(d.Close)
}
