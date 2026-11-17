package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSavedSessionFrameIsPrivateAndReplacesOldFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.jpg")
	for _, data := range [][]byte{[]byte("first-frame"), []byte("second")} {
		if err := saveSessionFrame(path, data); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("frame content: %q %v", got, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("frame not private: %v", info.Mode())
		}
	}
}
func TestSavedSessionFrameDoesNotWriteThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "latest.jpg")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := saveSessionFrame(path, []byte("frame")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "preserve" {
		t.Fatalf("symlink target overwritten: %q %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("temporary file leak: %v %v", entries, err)
	}
}

func TestSavedSessionFrameFailedReplacementCleansTemporary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.jpg")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(path, "preserve")
	if err := os.WriteFile(sentinel, []byte("old-destination"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveSessionFrame(path, []byte("new-frame")); err == nil {
		t.Fatal("replaced nonempty directory")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "old-destination" {
		t.Fatalf("old destination changed: %q %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "latest.jpg" {
		t.Fatalf("failed replacement leaked temporary: %v %v", entries, err)
	}
}
func TestSavedSessionFrameReplacesBroadFileWithPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.jpg")
	if err := os.WriteFile(path, []byte("older"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := saveSessionFrame(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("inherited old permissions: %v", info.Mode())
	}
}
