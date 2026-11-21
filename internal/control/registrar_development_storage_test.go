//go:build mnet_enrollment_development || multies_native_fixture

package control

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentStoreCodecLegacyBytesAndReverseRefusal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateNetwork("legacy"); e != nil {
		t.Fatal(e)
	}
	expected, _ := json.MarshalIndent(&s.data, "", "  ")
	raw, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(raw, expected) {
		t.Fatal("nil codec changed legacy bytes")
	}
	s.Close()
	if other, e := openStore(path, developmentRegistrarCodec{}); e == nil {
		other.Close()
		t.Fatal("new loader adopted legacy")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("legacy adoption refusal mutated bytes")
	}
	prefixed, e := (developmentRegistrarCodec{}).encode(raw)
	if e != nil {
		t.Fatal(e)
	}
	dedicated := filepath.Join(dir, "registrar-only.store")
	os.WriteFile(dedicated, prefixed, 0600)
	if old, e := Open(dedicated); e == nil {
		old.Close()
		t.Fatal("legacy adopted dedicated actual file")
	}
	after, _ = os.ReadFile(dedicated)
	if !bytes.Equal(prefixed, after) {
		t.Fatal("old refusal rewrote image")
	}
	d, e := openStore(dedicated, developmentRegistrarCodec{})
	if e != nil {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(dedicated)
	if !bytes.Equal(prefixed, after) {
		t.Fatal("loadsync rewrote image")
	}
	if _, e = d.CreateNetwork("second"); e != nil {
		t.Fatal(e)
	}
	d.Close()
	newest, _ := os.ReadFile(dedicated)
	if !bytes.HasPrefix(newest, []byte(developmentStoreHeader)) {
		t.Fatal("publish stripped prefix")
	}
	if old, e := Open(dedicated); e == nil {
		old.Close()
		t.Fatal("legacy adopted later image")
	}
}
func TestDevelopmentStoreCodecRejectsMissingWrongHeaderAndBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent")
	if s, e := openStore(path, developmentRegistrarCodec{}); e == nil {
		s.Close()
		t.Fatal("dedicated mode created absent image")
	}
	for _, raw := range [][]byte{nil, []byte("{}"), []byte("MNET-DISPOSABLE-REGISTRAR-ONLY-STORE-V2\n{}"), []byte(developmentStoreHeader), append([]byte(developmentStoreHeader), bytes.Repeat([]byte{'x'}, developmentStoreMaxBytes)...)} {
		if _, e := (developmentRegistrarCodec{}).decode(raw); e == nil {
			t.Fatal("bad envelope accepted")
		}
	}
}

func TestDevelopmentStoreCodecPublicationFailures(t *testing.T) {
	for _, stage := range []string{"create-temp", "write", "short-write", "sync-file", "close-file", "rename", "rename-after-publish", "sync-directory"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "store")
			s, e := Open(path)
			if e != nil {
				t.Fatal(e)
			}
			s.storageCodec = developmentRegistrarCodec{}
			if _, e = s.CreateNetwork("before"); e != nil {
				t.Fatal(e)
			}
			before, _ := os.ReadFile(path)
			real := s.fs
			s.fs.createTemp = func(dir string) (storeTemp, error) {
				if stage == "create-temp" {
					return nil, errInjectedStoreIO
				}
				tmp, e := real.createTemp(dir)
				return faultStoreTemp{tmp, stage}, e
			}
			if stage == "rename" {
				s.fs.rename = func(string, string) error { return errInjectedStoreIO }
			}
			if stage == "rename-after-publish" {
				s.fs.rename = func(a, b string) error {
					if e := real.rename(a, b); e != nil {
						return e
					}
					return errInjectedStoreIO
				}
			}
			if stage == "sync-directory" {
				s.fs.syncDir = func(p string) error {
					if e := real.syncDir(p); e != nil {
						return e
					}
					return errInjectedStoreIO
				}
			}
			if _, e = s.CreateNetwork("after"); e == nil {
				t.Fatal("injected error hidden")
			}
			s.Close()
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			payload, e := (developmentRegistrarCodec{}).decode(raw)
			if e != nil {
				t.Fatal(e)
			}
			var data storeData
			if json.Unmarshal(payload, &data) != nil {
				t.Fatal("partial envelope")
			}
			published := stage == "rename-after-publish" || stage == "sync-directory"
			if published && len(data.Networks) != 2 {
				t.Fatal("published candidate missing")
			}
			if !published && (!bytes.Equal(before, raw) || len(data.Networks) != 1) {
				t.Fatal("prepublication failure changed old image")
			}
			reopened, e := openStore(path, developmentRegistrarCodec{})
			if e != nil {
				t.Fatal(e)
			}
			reopened.Close()
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatal("load rewrote")
			}
		})
	}
}

func TestDevelopmentRootFreshMarkedOnlyAndLegacyNoAdoption(t *testing.T) {
	d, e := NewDisposableRegistrar(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := d.Root()
	network := d.NetworkID()
	d.Close()
	image := filepath.Join(root, developmentStoreName)
	before, _ := os.ReadFile(image)
	if s, e := Open(image); e == nil {
		s.Close()
		t.Fatal("legacy loader adopted actual prefixed image")
	}
	d, e = OpenDisposableRegistrar(root)
	if e != nil {
		t.Fatal(e)
	}
	if d.NetworkID() != network {
		t.Fatal("network changed")
	}
	d.Close()
	after, _ := os.ReadFile(image)
	if !bytes.Equal(before, after) {
		t.Fatal("reopen rewrote")
	}
	// Actual locks are FILE-path scoped, not root/directory scoped. Demonstrate
	// the documented limit: legacy can create a different absent filename here.
	other, e := Open(filepath.Join(root, "other-legacy.json"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = other.CreateNetwork("not-adopted"); e != nil {
		t.Fatal(e)
	}
	other.Close()
	d, e = OpenDisposableRegistrar(root)
	if e != nil {
		t.Fatal(e)
	}
	d.Close()
	os.WriteFile(image, []byte(`{"networks":{},"nodes":{},"auth_keys":{},"node_tokens":{},"next_ip":{}}`), 0600)
	invalid, _ := os.ReadFile(image)
	if d, e = OpenDisposableRegistrar(root); e == nil {
		d.Close()
		t.Fatal("tagged root converted legacy JSON")
	}
	after, _ = os.ReadFile(image)
	if !bytes.Equal(invalid, after) {
		t.Fatal("legacy refusal changed bytes")
	}
	os.Remove(image)
	if d, e = OpenDisposableRegistrar(root); e == nil {
		d.Close()
		t.Fatal("missing payload adopted")
	}
	os.WriteFile(image, before, 0600)
	os.WriteFile(filepath.Join(root, "root-kind"), []byte("wrong-root-kind"), 0600)
	if d, e = OpenDisposableRegistrar(root); e == nil {
		d.Close()
		t.Fatal("wrong marker adopted")
	}
}
