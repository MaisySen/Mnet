//go:build linux

package main

import (
	"errors"
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

func TestDeleteDeskStoreEntry(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("arrangement-1.json", `{"displays":[]}`)
	if _, err := statDeskDraft(dir, "missing.json"); err == nil {
		t.Fatal("stat of a missing draft must fail")
	}
	for _, bad := range []string{"", "../outside.json", "sub/arrangement.json", ".hidden"} {
		if _, err := statDeskDraft(dir, bad); err == nil {
			t.Fatalf("stat of %q must fail closed", bad)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := statDeskDraft(dir, "adir"); err == nil {
		t.Fatal("a directory is not a draft")
	}
	target := filepath.Join(dir, "arrangement-1.json")
	if err := os.Symlink(target, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := statDeskDraft(dir, "link.json"); err == nil {
		t.Fatal("a symlink is not a draft")
	}
	if err := os.Link(target, filepath.Join(dir, "alias.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := statDeskDraft(dir, "alias.json"); err == nil {
		t.Fatal("a hard-linked alias must be refused")
	}
	if _, err := statDeskDraft(dir, "arrangement-1.json"); err == nil {
		t.Fatal("a draft with a second hard link must be refused")
	}
	if err := os.Remove(filepath.Join(dir, "alias.json")); err != nil {
		t.Fatal(err)
	}
	id, err := statDeskDraft(dir, "arrangement-1.json")
	if err != nil {
		t.Fatal(err)
	}
	// Editing the draft in place changes identity and cancels the delete.
	write("arrangement-1.json", `{"displays":[],"edited":true}`)
	if err := deleteDeskStoreEntry(dir, "arrangement-1.json", id); !errors.Is(err, errDeskDraftChanged) {
		t.Fatalf("edited draft must cancel with errDeskDraftChanged, got %v", err)
	}
	// A same-name replacement (new inode) also cancels.
	id2, err := statDeskDraft(dir, "arrangement-1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	write("arrangement-1.json", `{"displays":[],"edited":true}`)
	if err := deleteDeskStoreEntry(dir, "arrangement-1.json", id2); !errors.Is(err, errDeskDraftChanged) {
		t.Fatalf("replaced draft must cancel with errDeskDraftChanged, got %v", err)
	}
	// A swapped store directory fails closed.
	id3, err := statDeskDraft(dir, "arrangement-1.json")
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(filepath.Dir(dir), "real-store")
	if err := os.Rename(dir, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, dir); err != nil {
		t.Fatal(err)
	}
	if err := deleteDeskStoreEntry(dir, "arrangement-1.json", id3); err == nil || errors.Is(err, errDeskDraftChanged) {
		t.Fatalf("symlinked store must fail closed before identity checks, got %v", err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(real, dir); err != nil {
		t.Fatal(err)
	}
	// Unchanged identity deletes exactly the pinned file.
	id4, err := statDeskDraft(dir, "arrangement-1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteDeskStoreEntry(dir, "arrangement-1.json", id4); err != nil {
		t.Fatalf("deleting an unchanged draft failed: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("draft still present after delete")
	}
	if _, err := os.Lstat(filepath.Join(dir, "link.json")); err != nil {
		t.Fatal("unrelated symlink was touched")
	}
}
