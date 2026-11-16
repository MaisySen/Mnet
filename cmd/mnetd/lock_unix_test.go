//go:build linux || darwin

package main

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixDaemonStateOwnership(t *testing.T) {
	dir := t.TempDir()
	release, err := lockDaemonState(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if closeAgain, err := lockDaemonState(dir); err == nil {
		closeAgain()
		t.Fatal("duplicate state owner allowed")
	}
	other, err := lockDaemonState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other()
	path := filepath.Join(dir, "daemon.lock")
	before, err := os.Stat(path)
	if err != nil || before.Mode().Perm() != 0600 {
		t.Fatal("lock is not private", err)
	}
	release()
	again, err := lockDaemonState(dir)
	if err != nil {
		t.Fatal("closed owner was not released", err)
	}
	again()
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("stable lock inode replaced", err)
	}
}

func TestUnixDaemonStateLockRejectsAliases(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "daemon.lock")
			target := filepath.Join(t.TempDir(), "preserve")
			if err := os.WriteFile(target, []byte("preserve"), 0644); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(target, path)
			case "hardlink":
				err = os.Link(target, path)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if release, err := lockDaemonState(dir); err == nil {
				release()
				t.Fatal("unsafe lock target accepted")
			}
			b, err := os.ReadFile(target)
			if err != nil || string(b) != "preserve" {
				t.Fatal("alias target changed")
			}
			st, err := os.Stat(target)
			if err != nil || st.Mode().Perm() != 0644 {
				t.Fatal("alias target permissions changed")
			}
		})
	}
}
