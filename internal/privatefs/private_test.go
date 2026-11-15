package privatefs

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicPrivateStateReplacement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	target := filepath.Join(dir, "identity.json")
	for _, value := range []string{"first identity", "updated identity"} {
		if err := WriteFile(target, []byte(value)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(got, []byte(value)) {
			t.Fatalf("readback: %v", err)
		}
		if runtime.GOOS != "windows" {
			info, _ := os.Stat(target)
			if info.Mode().Perm() != 0600 {
				t.Fatal("not private")
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}

func TestRejectSymlinkStateDestination(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "state")
	if err := EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "identity")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation not available: %v", err)
	}
	if err := WriteFile(link, []byte("overwrite")); err == nil {
		t.Fatal("followed state symlink")
	}
	if err := ProtectFile(link); err == nil {
		t.Fatal("protected symlink target")
	}
	got, _ := os.ReadFile(outside)
	if string(got) != "keep" {
		t.Fatal("outside file modified")
	}
}
