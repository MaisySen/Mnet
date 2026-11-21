//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
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

func TestDuplicateDeskStoreEntry(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`{"version":1,"displays":[]}`)
	if err := os.WriteFile(filepath.Join(dir, "arrangement-1.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := statDeskDraft(dir, "arrangement-1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "../copy.json", ".hidden", "arrangement-1.json"} {
		if err := duplicateDeskStoreEntry(dir, "arrangement-1.json", bad, id); err == nil {
			t.Fatalf("duplicate destination %q must fail closed", bad)
		}
	}
	if err := duplicateDeskStoreEntry(dir, "arrangement-1.json", "arrangement-2.json", deskDraftID{}); !errors.Is(err, errDeskDraftChanged) {
		t.Fatalf("stale identity must cancel a duplicate, got %v", err)
	}
	if err := duplicateDeskStoreEntry(dir, "missing.json", "arrangement-2.json", id); err == nil {
		t.Fatal("duplicating a missing draft must fail")
	}
	if err := duplicateDeskStoreEntry(dir, "arrangement-1.json", "arrangement-2.json", id); err != nil {
		t.Fatalf("duplicate failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "arrangement-2.json"))
	if err != nil || string(got) != string(body) {
		t.Fatalf("duplicate content wrong: %v %q", err, got)
	}
	info, err := os.Lstat(filepath.Join(dir, "arrangement-2.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("duplicate not owner-only: %v", err)
	}
	if err := duplicateDeskStoreEntry(dir, "arrangement-1.json", "arrangement-2.json", id); err == nil {
		t.Fatal("an existing destination must never be replaced")
	}
	if _, err := os.Lstat(filepath.Join(dir, "arrangement-1.json")); err != nil {
		t.Fatal("source draft was touched")
	}
}

func TestDuplicateDeskStoreEntrySwappedStore(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "arrangement-1.json"), []byte(`{"displays":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	id, err := statDeskDraft(real, "arrangement-1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := duplicateDeskStoreEntry(link, "arrangement-1.json", "arrangement-2.json", id); err == nil {
		t.Fatal("a symlinked store path must fail closed for duplicate")
	}
	if _, err := os.Lstat(filepath.Join(real, "arrangement-2.json")); !os.IsNotExist(err) {
		t.Fatal("duplicate wrote through a swapped store path")
	}
	if err := deleteDeskStoreEntry(link, "arrangement-1.json", id); err == nil {
		t.Fatal("a symlinked store path must fail closed for delete")
	}
	if _, err := os.Lstat(filepath.Join(real, "arrangement-1.json")); err != nil {
		t.Fatal("delete removed through a swapped store path")
	}
}

func TestDeleteDeskStoreEntryRestoredMtime(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "arrangement-1.json")
	if err := os.WriteFile(target, []byte(`{"displays":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := statDeskDraft(dir, "arrangement-1.json")
	if err != nil {
		t.Fatal(err)
	}
	var st unix.Stat_t
	if err := unix.Lstat(target, &st); err != nil {
		t.Fatal(err)
	}
	// Same-size edit with the mtime forged back: ctime still moves.
	if err := os.WriteFile(target, []byte(`{"displays":[1]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := []unix.Timeval{{Sec: st.Mtim.Sec, Usec: st.Mtim.Nsec / 1000}, {Sec: st.Mtim.Sec, Usec: st.Mtim.Nsec / 1000}}
	if err := unix.Utimes(target, ts); err != nil {
		t.Fatal(err)
	}
	if err := deleteDeskStoreEntry(dir, "arrangement-1.json", id); !errors.Is(err, errDeskDraftChanged) {
		t.Fatalf("edit with restored mtime must cancel, got %v", err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatal("forged-mtime draft was removed")
	}
}

func TestDedupePinnedStoreEntries(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"displays":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("arrangement-1.json")
	write("arrangement-2.json")
	entries, err := listDeskStore(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("listing: %v %d", err, len(entries))
	}
	layout := filepath.Join(dir, "arrangement-1.json")
	kept := dedupePinnedStoreEntries(dir, layout, entries)
	if len(kept) != 1 || kept[0].Name != "arrangement-2.json" {
		t.Fatalf("pinned store file must appear once, kept %+v", kept)
	}
	// A hard-linked alias of the pinned file is deduped by identity too.
	if err := os.Link(layout, filepath.Join(dir, "alias.json")); err != nil {
		t.Fatal(err)
	}
	entries, err = listDeskStore(dir)
	if err != nil || len(entries) != 3 {
		t.Fatalf("listing with alias: %v %d", err, len(entries))
	}
	kept = dedupePinnedStoreEntries(dir, layout, entries)
	if len(kept) != 1 || kept[0].Name != "arrangement-2.json" {
		t.Fatalf("alias of pinned file must be deduped, kept %+v", kept)
	}
	// A pinned file outside the store dedupes nothing.
	outside := filepath.Join(t.TempDir(), "other.json")
	if err := os.WriteFile(outside, []byte(`{"displays":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	kept = dedupePinnedStoreEntries(dir, outside, entries)
	if len(kept) != 3 {
		t.Fatalf("external pinned file must not hide store drafts, kept %+v", kept)
	}
}
