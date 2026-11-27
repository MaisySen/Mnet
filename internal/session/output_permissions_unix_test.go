//go:build !windows

package session

import (
	"os"
	"testing"
)

func assertPrivateOutput(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("output permissions = %v, want %v", info.Mode().Perm(), mode)
	}
}
