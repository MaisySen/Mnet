package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRefusesLinkedIdentity(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"node_name":"original"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, statePath(dir)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := LoadState(dir); err == nil {
		t.Fatal("loaded linked identity")
	}
	if err := (&State{NodeName: "overwritten"}).Save(dir); err == nil {
		t.Fatal("saved over linked identity")
	}
	got, _ := os.ReadFile(outside)
	if string(got) != `{"node_name":"original"}` {
		t.Fatal("outside identity changed")
	}
}

func TestPrivateStateAtomicRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s := &State{NodeName: "first"}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	s.NodeName = "second"
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil || got.NodeName != "second" {
		t.Fatalf("roundtrip: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files: %v %v", entries, err)
	}
}

func TestStateRejectsBrokenIdentityWithoutReplacement(t *testing.T) {
	valid := &State{}
	if err := valid.EnsureKey(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"", "null", `[]`, `true`, `{"node_id":"orphan"}`, `{"node_token":"orphan"}`, `{"private_key":"broken","public_key":"broken"}`, `{"private_key":"` + valid.PrivateKey + `"}`, `{"public_key":"` + valid.PublicKey + `"}`, `{"private_key":"` + valid.PrivateKey + `","public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`} {
		t.Run(body[:min(20, len(body))], func(t *testing.T) {
			dir := t.TempDir()
			path := statePath(dir)
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if st, err := LoadState(dir); err == nil || st != nil {
				t.Fatal("broken identity accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != body {
				t.Fatal("broken identity replaced")
			}
		})
	}
	// A caller holding malformed in-memory state cannot silently rotate it.
	broken := &State{PublicKey: valid.PublicKey}
	dir := t.TempDir()
	if err := broken.EnsureKey(dir); err == nil {
		t.Fatal("half identity silently regenerated")
	}
	if _, err := os.Stat(statePath(dir)); !os.IsNotExist(err) {
		t.Fatal("failed identity check wrote state")
	}
}

func TestStateGeneratedIdentityReloadAndMetadataCompatibility(t *testing.T) {
	dir := t.TempDir()
	st := &State{NodeName: "fixture"}
	if err := st.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateKey != st.PrivateKey || got.PublicKey != st.PublicKey {
		t.Fatal("identity changed on reload")
	}
	if err := got.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(statePath(dir))
	if err != nil || string(before) != string(after) {
		t.Fatal("existing key regenerated or rewritten")
	}
	metadataDir := t.TempDir()
	if err := os.WriteFile(statePath(metadataDir), []byte(`{"node_name":"legacy"}`), 0600); err != nil {
		t.Fatal(err)
	}
	legacy, err := LoadState(metadataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.EnsureKey(metadataDir); err != nil {
		t.Fatal(err)
	}
	if legacy.NodeName != "legacy" || legacy.PrivateKey == "" || legacy.PublicKey == "" {
		t.Fatal("metadata compatibility failed")
	}
}

func TestIdentityGenerationFailedSaveRollsBackAndRetries(t *testing.T) {
	dir := t.TempDir()
	// A directory at the file destination fails after first-run generation.
	if err := os.Mkdir(statePath(dir), 0700); err != nil {
		t.Fatal(err)
	}
	st := &State{NodeName: "fixture"}
	if err := st.EnsureKey(dir); err == nil {
		t.Fatal("generation save unexpectedly succeeded")
	}
	if st.PrivateKey != "" || st.PublicKey != "" || st.NodeName != "fixture" {
		t.Fatal("failed generation left unpublished identity")
	}
	if err := os.Remove(statePath(dir)); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PrivateKey != st.PrivateKey || loaded.PublicKey != st.PublicKey || loaded.NodeName != "fixture" {
		t.Fatal("retry did not publish its identity")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("generation retry leaked temporary state")
	}
}

func TestRoutingFailedSaveRestoresLiveAndDiskState(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetRouting(false, []string{"10.1.0.0/24"}, ""); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the actual saved file intact while making the next destination fail.
	d.cfg.StateDir = filepath.Join(statePath(dir), "invalid-child")
	if err := d.SetRouting(true, []string{"10.2.0.0/24"}, "exit-candidate"); err == nil {
		t.Fatal("routing save unexpectedly succeeded")
	}
	got := d.State()
	if got.ExitNode || got.UseExitNode != "" || len(got.AdvertiseRoutes) != 1 || got.AdvertiseRoutes[0] != "10.1.0.0/24" {
		t.Fatal("failed routing save changed live settings")
	}
	after, err := os.ReadFile(statePath(dir))
	if err != nil || string(before) != string(after) {
		t.Fatal("failed routing save changed disk state")
	}
	d.cfg.StateDir = dir
	if err := d.SetRouting(false, []string{"10.3.0.0/24"}, ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(dir)
	if err != nil || len(loaded.AdvertiseRoutes) != 1 || loaded.AdvertiseRoutes[0] != "10.3.0.0/24" {
		t.Fatal("routing retry failed", err)
	}
}
