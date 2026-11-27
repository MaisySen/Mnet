package main

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSavedSessionFrameHasProtectedWindowsDACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.jpg")
	for _, content := range []string{"original", "replacement"} {
		if err := saveSessionFrame(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		flags, _, err := sd.Control()
		if err != nil || flags&windows.SE_DACL_PROTECTED == 0 {
			t.Fatal("snapshot DACL permits inheritance", err)
		}
	}
}
