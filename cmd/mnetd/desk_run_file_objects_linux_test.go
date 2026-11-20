//go:build linux

package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// All filesystem objects are disposable. No display, certificate parsing,
// listener for a Desk run or review/native input owner is created here.
func TestDeskLocalFilesRejectSpecialObjectsAndExactBounds(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	data := bytes.Repeat([]byte{'x'}, 64)
	if err := os.WriteFile(regular, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, private := range []bool{false, true} {
		got, err := readDeskLocalFile(regular, private, 64)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatal("exact byte bound refused", private, err)
		}
		if got, err = readDeskLocalFile(regular, private, 63); err == nil || got != nil {
			t.Fatal("oversize returned data", private, err)
		}
	}
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "socket")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	link := filepath.Join(dir, "link")
	if err = os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(dir, "dangling")
	if err = os.Symlink(filepath.Join(dir, "absent"), dangling); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, fifo, socket, link, dangling, filepath.Join(dir, "absent")} {
		for _, private := range []bool{false, true} {
			if got, err := readDeskLocalFile(path, private, 64); err == nil || got != nil {
				t.Fatal("special object returned data", filepath.Base(path), private, err)
			}
		}
	}
	// Group/other access on a private key refuses independently of its bytes.
	for _, mode := range []os.FileMode{0640, 0604, 0601, 0620, 0707} {
		if err = os.Chmod(regular, mode); err != nil {
			t.Fatal(err)
		}
		if got, err := readDeskLocalFile(regular, true, 64); err == nil || got != nil {
			t.Fatal("nonprivate key readable", mode, err)
		}
	}
	if err = os.Chmod(regular, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(regular, nil, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readDeskLocalFile(regular, true, 0)
	if err != nil || len(got) != 0 {
		t.Fatal("zero-size file at zero bound", err)
	}
}

func TestDeskRunExistingReplayObjectsNeverOverwritten(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			token := strings.Repeat("b", 64)
			path := filepath.Join(dir, token)
			sentinel := filepath.Join(t.TempDir(), "sentinel")
			content := []byte("unchanged fixture bytes")
			if err := os.WriteFile(sentinel, content, 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "file":
				err = os.WriteFile(path, content, 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "symlink":
				err = os.Symlink(sentinel, path)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if err = reserveDeskRun(dir, token); err == nil {
					t.Fatal("existing replay object reused", kind)
				}
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("replay object replaced", kind, err)
			}
			got, err := os.ReadFile(sentinel)
			if err != nil || !bytes.Equal(got, content) {
				t.Fatal("replay changed symlink target", err)
			}
			if kind == "file" {
				got, err = os.ReadFile(path)
				if err != nil || !bytes.Equal(got, content) {
					t.Fatal("existing record overwritten", err)
				}
			}
		})
	}
}
