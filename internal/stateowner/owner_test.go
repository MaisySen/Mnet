package stateowner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAcquireStableExclusiveAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "journal.json")
	owner, canonical, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if canonical != path || owner.Parent() != filepath.Dir(path) || owner.LockPath() != path+".lock" {
		t.Fatal("canonical paths differ")
	}
	before, err := os.Stat(owner.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if second, _, err := Acquire(path); err == nil {
		second.Close()
		t.Fatal("second writer admitted")
	}
	// A separate basename can be owned independently in the same parent.
	other, _, err := Acquire(filepath.Join(owner.Parent(), "other.json"))
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal("Close not idempotent", err)
	}
	if err := owner.Check(); err == nil {
		t.Fatal("closed lease healthy")
	}
	again, _, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	after, err := os.Stat(again.LockPath())
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("stable inode changed", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("ownership created state")
	}
}
func TestParentAliasesAndPermissions(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "shared")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := Acquire(filepath.Join(parent, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	after, err := os.Stat(parent)
	if err != nil || before.Mode() != after.Mode() {
		t.Fatal("shared parent permissions changed", err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Skip(err)
	}
	if second, _, err := Acquire(filepath.Join(alias, "state")); err == nil {
		second.Close()
		t.Fatal("alias second owner")
	}
}
func TestReplacedOwnerOrParentIsUnhealthy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows denies ownership file rename while held; exercised by native control contract")
	}
	for _, kind := range []string{"lock", "parent"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "owned")
			owner, _, err := Acquire(filepath.Join(parent, "journal"))
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			if kind == "lock" {
				if err := os.Rename(owner.LockPath(), owner.LockPath()+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(owner.LockPath(), nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(parent, parent+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := owner.Check(); err == nil {
				t.Fatal("replacement not detected")
			}
		})
	}
}
func TestLockLinksAndNonregularRefusedWithoutChmodTarget(t *testing.T) {
	for _, kind := range []string{"hardlink", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "keep")
			if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "journal")
			lock := path + ".lock"
			var err error
			switch kind {
			case "hardlink":
				err = os.Link(target, lock)
			case "symlink":
				err = os.Symlink(target, lock)
			case "directory":
				err = os.Mkdir(lock, 0700)
			}
			if err != nil {
				t.Skip(err)
			}
			if owner, _, err := Acquire(path); err == nil {
				owner.Close()
				t.Fatal("unsafe lock accepted")
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != "keep" {
				t.Fatal("alias target changed", err)
			}
			info, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0644 {
				t.Fatal("alias target permissions changed")
			}
		})
	}
}
