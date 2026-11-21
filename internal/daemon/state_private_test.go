package daemon

import (
	"encoding/json"
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
	if err != nil || len(entries) != 2 {
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
	if err != nil || len(entries) != 2 {
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
	d.Close()
	loaded, err := LoadState(dir)
	if err != nil || len(loaded.AdvertiseRoutes) != 1 || loaded.AdvertiseRoutes[0] != "10.3.0.0/24" {
		t.Fatal("routing retry failed", err)
	}
}

func TestStartupConfigFailedSaveDoesNotLeakIntoRetry(t *testing.T) {
	for _, mode := range []string{"up", "prepare"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			initial := &State{Server: "http://127.0.0.1:1", Network: "original", NodeName: "original-device"}
			if err := initial.EnsureKey(dir); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			d, err := New(Config{StateDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			d.cfg.StateDir = filepath.Join(statePath(dir), "invalid-child")
			if mode == "up" {
				err = d.Up("http://127.0.0.1:2", "wrong-network", "wrong-device", "")
			} else {
				_, err = d.Prepare("http://127.0.0.1:2", "wrong-network", "wrong-device", "")
			}
			if err == nil {
				t.Fatal("startup configuration save unexpectedly succeeded")
			}
			got := d.State()
			if got.Server != initial.Server || got.Network != initial.Network || got.NodeName != initial.NodeName || got.PrivateKey != initial.PrivateKey || got.PublicKey != initial.PublicKey {
				t.Fatal("failed startup changed live persisted configuration or identity")
			}
			if d.Status().Status != "down" {
				t.Fatal("failed startup did not return down")
			}
			after, err := os.ReadFile(statePath(dir))
			if err != nil || string(before) != string(after) {
				t.Fatal("failed startup changed saved bytes")
			}
		})
	}
}

func TestStateRejectsIncompleteEnrollmentWithoutRepair(t *testing.T) {
	valid := &State{Server: "https://control.example", Network: "home", NodeID: "fixture-id", NodeToken: "fixture-token"}
	// Generate first, then add enrollment, just as the ordinary workflow does.
	valid.NodeID, valid.NodeToken = "", ""
	if err := valid.EnsureKey(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	valid.NodeID, valid.NodeToken = "fixture-id", "fixture-token"
	for _, field := range []string{"node-id", "node-token", "blank-id", "blank-token", "server", "network"} {
		t.Run(field, func(t *testing.T) {
			broken := *valid
			switch field {
			case "node-id":
				broken.NodeID = ""
			case "node-token":
				broken.NodeToken = ""
			case "blank-id":
				broken.NodeID = " \t"
			case "blank-token":
				broken.NodeToken = " \t"
			case "server":
				broken.Server = ""
			case "network":
				broken.Network = ""
			}
			dir := t.TempDir()
			raw, err := json.Marshal(broken)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(statePath(dir), raw, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			if got, err := LoadState(dir); err == nil || got != nil {
				t.Fatal("incomplete saved enrollment accepted")
			}
			if err := broken.EnsureKey(dir); err == nil {
				t.Fatal("incomplete in-memory enrollment accepted")
			}
			after, err := os.ReadFile(statePath(dir))
			if err != nil || string(after) != string(before) {
				t.Fatal("incomplete enrollment repaired or overwritten")
			}
			if broken.PrivateKey != valid.PrivateKey || broken.PublicKey != valid.PublicKey {
				t.Fatal("existing identity replaced")
			}
		})
	}
	dir := t.TempDir()
	valid.revisionPath = ""
	valid.revisionExists = false
	valid.revision = [32]byte{}
	if err := valid.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil || got.NodeID != valid.NodeID || got.NodeToken != valid.NodeToken {
		t.Fatal("complete saved enrollment failed reload", err)
	}
}
