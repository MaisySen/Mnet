package daemon

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Test-only FUTURE marker spellings, not a selected storage schema or a writer.
// Current binaries must not silently ignore/strip unknown credential state.
func TestNativeFutureCredentialMarkersRefuseActualOwnerWithoutRewrite(t *testing.T) {
	for _, marker := range []string{"credential_kind", "registrar_credential", "finalization_identity", "credential_version"} {
		t.Run(marker, func(t *testing.T) {
			d, dir := nativeOwned(t)
			detached := d.State()
			d.Close()
			raw, err := os.ReadFile(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatal(err)
			}
			obj[marker] = json.RawMessage(`{"test_only":"unknown future credential DATA"}`)
			modified, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(statePath(dir), modified, 0600); err != nil {
				t.Fatal(err)
			}
			for _, operation := range []string{"load", "new", "stale-save", "fresh-save"} {
				switch operation {
				case "load":
					if _, err := LoadState(dir); err == nil {
						t.Fatal("unknown credential marker loaded")
					}
				case "new":
					fresh, err := New(Config{StateDir: dir})
					if err == nil {
						fresh.Close()
						t.Fatal("unknown credential marker created daemon")
					}
				case "stale-save":
					if err := detached.Save(dir); err == nil {
						t.Fatal("detached save stripped unknown credential marker")
					}
				case "fresh-save":
					fresh := State{}
					if err := fresh.EnsureKey(dir); err == nil {
						t.Fatal("fresh identity replaced future credential state")
					}
				}
				after, err := os.ReadFile(statePath(dir))
				if err != nil || !bytes.Equal(after, modified) {
					t.Fatal("refusal rewrote actual identity", operation, err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 2 {
					t.Fatal("refusal created alternate State/pending storage", operation, err)
				}
			}
			// Failed opens release actual stable owner. Removing only test's added field
			// restores the original identity, never generating replacement key material.
			if err := os.WriteFile(statePath(dir), raw, 0600); err != nil {
				t.Fatal(err)
			}
			restored, err := LoadState(dir)
			if err != nil || restored.PublicKey != detached.PublicKey || restored.PrivateKey != detached.PrivateKey {
				t.Fatal("failed owner leaked lock or changed key", err)
			}
		})
	}
}
func TestNativeIncompleteRegistrarLikeLegacyIdentityRefusesActualOwner(t *testing.T) {
	for _, variant := range []string{"node-without-token", "token-without-node", "node-without-target", "no-key-credential"} {
		t.Run(variant, func(t *testing.T) {
			d, dir := nativeOwned(t)
			state := d.State()
			d.Close()
			state.Server = "https://registrar.invalid"
			state.Network = "fixture-network"
			state.NodeID = "fixture-node"
			state.NodeToken = ""
			switch variant {
			case "token-without-node":
				state.NodeID = ""
				state.NodeToken = "fixture-only-secret"
			case "node-without-target":
				state.NodeToken = "fixture-only-secret"
				state.Server = ""
			case "no-key-credential":
				state.NodeToken = "fixture-only-secret"
				state.PrivateKey = ""
				state.PublicKey = ""
			}
			raw, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(statePath(dir), raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadState(dir); err == nil {
				t.Fatal("incomplete familiar fields became valid enrollment")
			}
			fresh, err := New(Config{StateDir: dir})
			if err == nil {
				fresh.Close()
				t.Fatal("incomplete enrollment started daemon")
			}
			after, err := os.ReadFile(filepath.Join(dir, "state.json"))
			if err != nil || !bytes.Equal(raw, after) {
				t.Fatal("incomplete enrollment repaired or rekeyed", err)
			}
		})
	}
}
