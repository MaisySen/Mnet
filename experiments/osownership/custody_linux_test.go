//go:build linux && mnet_enrollment_development

package osownership

import (
	"os"
	"testing"
)

func TestFileCustodyDoesNotCloseReusedDescriptor(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "original")
	if e != nil {
		t.Fatal(e)
	}
	original := f.Fd()
	c := &fileCustody{file: f}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	replacement, e := os.CreateTemp(t.TempDir(), "replacement")
	if e != nil {
		t.Fatal(e)
	}
	defer replacement.Close()
	if replacement.Fd() != original {
		t.Skip("descriptor reuse unavailable; transaction count test still decisive")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = replacement.Write([]byte("alive")); e != nil {
		t.Fatal("reused FD closed", e)
	}
}
