//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDeskStoreCreatesPrivateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "desk-layouts")
	if err := ensureDeskStore(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("store = %v %v, want private 0700 directory", info, err)
	}
	if err := ensureDeskStore(dir); err != nil {
		t.Fatal("ensuring an existing private store must succeed")
	}
}

func TestEnsureDeskStoreRefusesSharedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "desk-layouts")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0770); err != nil {
		t.Fatal(err)
	}
	if err := ensureDeskStore(dir); err == nil {
		t.Fatal("a group/other-writable store must be refused")
	}
}

func TestDeskStoreOutPathStaysInsideStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "desk-layouts")
	out, err := deskStoreOutPath(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(out) != dir || filepath.Base(out) != "arrangement-1.json" {
		t.Fatalf("out = %q, want arrangement-1.json inside the store", out)
	}
}
