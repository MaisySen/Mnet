package daemon

import (
	"testing"
)

// Actual native owner contracts, promoted from the initially failing opt-in
// checkpoint. These exercise existing APIs, not an alternate fixture journal.
func TestNativeStateContractDaemonOwnsEvenAbsentFile(t *testing.T) {
	dir := t.TempDir()
	first, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := New(Config{StateDir: dir}); err == nil {
		second.Close()
		t.Fatal("second native State owner admitted before first write")
	}
	first.Close()
	reopened, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("Close did not release native State", err)
	}
	reopened.Close()
}
func TestNativeStateContractDirectWriterCannotBypassDaemon(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	raw := State{NodeName: "bypass"}
	if err := raw.Save(dir); err == nil {
		t.Fatal("direct State writer bypassed live daemon owner")
	}
}
func TestNativeStateContractDetachedSnapshotRejectsStaleSave(t *testing.T) {
	dir := t.TempDir()
	initial := State{NodeName: "first"}
	if err := initial.Save(dir); err != nil {
		t.Fatal(err)
	}
	a, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	a.NodeName = "newer"
	if err := a.Save(dir); err != nil {
		t.Fatal(err)
	}
	b.NodeName = "stale"
	if err := b.Save(dir); err == nil {
		t.Fatal("detached stale snapshot overwrote a newer native State")
	}
}
