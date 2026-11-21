package control

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestControlStateRejectsLinkedDestination(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "outside.json")
	if err := os.WriteFile(target, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	if st, err := openTestStore(t, link); err == nil || st != nil {
		t.Fatal("opened symlink state")
	}
	st, err := openTestStore(t, filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateNetwork("fixture"); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(st.path); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, st.path); err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateNetwork("rejected"); err == nil {
		t.Fatal("replaced symlink destination")
	}
	if len(st.Networks()) != 1 {
		t.Fatal("failed linked write changed state")
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != `{}` {
		t.Fatal("outside target changed")
	}
	info, err := os.Lstat(st.path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
}

func TestControlStateProtectionDoesNotChangeSharedParent(t *testing.T) {
	root := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(root, 0755); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "state.json")
	if err = os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	st, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateNetwork("fixture"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatal("shared parent permissions changed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("state file not private")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := openTestStore(t, path)
	if err != nil || len(loaded.Networks()) != 1 {
		t.Fatal("private state reload failed", err)
	}
}
