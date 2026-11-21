package daemon

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestDetachedExistingKeyCheckCannotBypassLiveOrStaleOwner(t *testing.T) {
	dir := t.TempDir()
	s := &State{}
	if err := s.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	stale := *s
	current, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureKey(dir); err == nil {
		t.Fatal("public no-op EnsureKey bypassed live library owner")
	}
	current.Close()
	fresh, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	fresh.NodeName = "newer"
	if err := fresh.Save(dir); err != nil {
		t.Fatal(err)
	}
	if err := stale.EnsureKey(dir); !errors.Is(err, ErrStateStale) {
		t.Fatal("public no-op EnsureKey accepted stale revision", err)
	}
}
func TestNativeRetainedOwnerDoesNotContinueAfterExternalStateMutation(t *testing.T) {
	d, dir := nativeOwned(t)
	raw, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(raw, []byte(`"node_name": ""`), []byte(`"node_name": "changed-outside-owner"`), 1)
	if bytes.Equal(changed, raw) {
		t.Fatal("fixture mutation not applied")
	}
	if err := os.WriteFile(statePath(dir), changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.SetRouting(false, nil, ""); !errors.Is(err, ErrStateUncertain) {
		t.Fatal("retained stale owner continued serving prior image", err)
	}
	if _, err := d.PrepareIdentity(); err == nil {
		t.Fatal("external mutation failure was not permanent")
	}
}
func TestNativeGeneratedKeyUncertainPublishNeverRegeneratesInPlace(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	original := d.stateOwner.fs
	d.stateOwner.fs.rename = func(a, b string) error {
		if err := original.rename(a, b); err != nil {
			return err
		}
		return errNativeInjected
	}
	if key, err := d.PrepareIdentity(); err == nil || key != "" {
		t.Fatal("uncertain generation reported success")
	}
	raw, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if key, err := d.PrepareIdentity(); err == nil || key != "" {
		t.Fatal("uncertain generation created another key")
	}
	after, err := os.ReadFile(statePath(dir))
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("uncertain generated identity was rotated", err)
	}
	d.Close()
	fresh, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if key, err := fresh.PrepareIdentity(); err != nil || key == "" {
		t.Fatal("whole generated identity did not reopen", err)
	}
	after, err = os.ReadFile(statePath(dir))
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("reopen rewrote existing generated key", err)
	}
}

func TestCurrentNativeSnapshotCannotReplaceExistingKeyPair(t *testing.T) {
	dir := t.TempDir()
	original := &State{}
	if err := original.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	other := &State{}
	if err := other.EnsureKey(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.PrivateKey = other.PrivateKey
	snapshot.PublicKey = other.PublicKey
	if err := snapshot.Save(dir); err == nil {
		t.Fatal("current snapshot silently replaced existing identity")
	}
	reloaded, err := LoadState(dir)
	if err != nil || reloaded.PrivateKey != original.PrivateKey || reloaded.PublicKey != original.PublicKey {
		t.Fatal("refused replacement changed original identity", err)
	}
}
