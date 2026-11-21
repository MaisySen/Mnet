//go:build mnet_enrollment_development && (linux || darwin || freebsd || openbsd || netbsd || dragonfly)

package stateowner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingDevelopmentLeaseNeverCreatesOrRepairs(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "owned")
	lock := path + ".lock"
	if _, err := AcquireExistingDevelopment(path); err == nil {
		t.Fatal("missing lock accepted")
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("missing lock was created")
	}
	if err := os.WriteFile(lock, []byte("unchanged-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(lock)
	raw, _ := os.ReadFile(lock)
	lease, err := AcquireExistingDevelopment(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := AcquireExistingDevelopment(path); err == nil {
		other.Close()
		t.Fatal("concurrent lease accepted")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(lock)
	now, _ := os.ReadFile(lock)
	if string(raw) != string(now) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(before, after) {
		t.Fatal("read lease mutated existing lock")
	}
	if err := os.Chmod(lock, 0644); err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireExistingDevelopment(path); err == nil {
		lease.Close()
		t.Fatal("insecure lock accepted")
	}
	after, _ = os.Stat(lock)
	if after.Mode().Perm() != 0644 {
		t.Fatal("insecure lock silently repaired")
	}
}
func TestExistingDevelopmentLeaseRefusesAliasesAndReplacement(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "owned")
	if err := os.WriteFile(path+".lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	if l, e := AcquireExistingDevelopment(filepath.Join(alias, "owned")); e == nil {
		l.Close()
		t.Fatal("parent alias accepted")
	}
	l, e := AcquireExistingDevelopment(path)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if err := os.Rename(path+".lock", path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if l.Check() == nil {
		t.Fatal("replaced lock resurrected ownership")
	}
}
