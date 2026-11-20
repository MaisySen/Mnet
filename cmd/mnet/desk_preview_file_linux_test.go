//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDeskPreviewFilesAreRegularBoundedAndDescriptorOwned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.json")
	if err := os.WriteFile(path, []byte(deskFixture), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readDeskLayoutPreviewFile(path)
	if err != nil || snapshot.display().ID != "a" {
		t.Fatal(snapshot, err)
	}
	link := filepath.Join(dir, "explicit-layout-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readDeskLayoutPreviewFile(link); err != nil {
		t.Fatal("explicit regular file link changed", err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if snapshot.display().ID != "a" || len(snapshot.layout.Displays) != 2 {
		t.Fatal("snapshot followed replacement")
	}
	fifo := filepath.Join(dir, "layout.fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{dir, fifo, "/dev/zero", filepath.Join(dir, "missing")} {
		if p, err := readDeskLayoutPreviewFile(bad); err == nil || p != nil {
			t.Fatal("nonregular file accepted", bad)
		}
	}
	for _, bad := range []string{"{}", deskFixture + "{}", strings.Repeat(" ", 65537)} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if p, err := readDeskLayoutPreviewFile(path); err == nil || p != nil {
			t.Fatal("invalid/oversized file accepted")
		}
	}
}
