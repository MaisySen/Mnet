package session

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func assertPrivateOutput(t *testing.T, path string, _ os.FileMode) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	flags, _, err := sd.Control()
	if err != nil || flags&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("output DACL permits inheritance", err)
	}
}
