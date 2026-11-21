//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDeskEditedLayoutOutcomes(t *testing.T) {
	data := []byte("{\"version\":1}\n")
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Success: one new owner-only file.
	good := filepath.Join(dir, "new.json")
	if err := writeDeskEditedLayout(good, data); err != nil {
		t.Fatalf("clean save: %v", err)
	}
	info, err := os.Stat(good)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("saved file mode: %v %v", info, err)
	}
	// Existing destination: refused, bytes untouched, classified as exists.
	if err := os.WriteFile(filepath.Join(dir, "kept.json"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeDeskEditedLayout(filepath.Join(dir, "kept.json"), data); !errors.Is(err, errDeskEditSaveExists) {
		t.Fatalf("existing destination: %v", err)
	}
	kept, _ := os.ReadFile(filepath.Join(dir, "kept.json"))
	if string(kept) != "keep" {
		t.Fatal("existing destination was modified")
	}
	// Unwritable private parent: safely absent, nothing created.
	ro := filepath.Join(dir, "readonly")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := writeDeskEditedLayout(filepath.Join(ro, "blocked.json"), data); !errors.Is(err, errDeskEditSaveCreate) {
		t.Fatalf("unwritable parent: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(ro, "blocked.json")); !os.IsNotExist(err) {
		t.Fatalf("blocked destination should be absent: %v", err)
	}
	// Missing or group-writable parents refuse before any create.
	if err := writeDeskEditedLayout(filepath.Join(dir, "missing", "x.json"), data); !errors.Is(err, errDeskEditSaveParent) {
		t.Fatalf("missing parent: %v", err)
	}
	if err := os.Chmod(ro, 0o720); err != nil {
		t.Fatal(err)
	}
	if err := writeDeskEditedLayout(filepath.Join(ro, "x.json"), data); !errors.Is(err, errDeskEditSaveParent) {
		t.Fatalf("group-writable parent: %v", err)
	}
}
