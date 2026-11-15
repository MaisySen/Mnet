//go:build windows

package privatefs

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

func assertPrivateDACL(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		t.Fatalf("descriptor: %v", err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("DACL still inherits parent permissions")
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		t.Fatal("missing owner")
	}
	allowed := map[string]bool{owner.String(): true, "S-1-5-18": true, "S-1-5-32-544": true}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		t.Fatal("empty/null DACL")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatal("unexpected ACE type")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !allowed[sid.String()] {
			t.Fatalf("unexpected granted principal %s", sid.String())
		}
	}
}

func TestWindowsDACLRestrictsIdentityDirectoryAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(dir, 0777); err != nil {
		t.Fatal(err)
	}
	// Simulate an existing inherited/broad directory before migration.
	broad, err := windows.SecurityDescriptorFromString("D:(A;OICI;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := broad.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if err = EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	assertPrivateDACL(t, dir)
	path := filepath.Join(dir, "identity.json")
	if err = WriteFile(path, []byte("test identity fixture")); err != nil {
		t.Fatal(err)
	}
	assertPrivateDACL(t, path)
	// Owner may replace and read; no Users/Everyone/authenticated-users grants.
	if err = WriteFile(path, []byte("updated fixture")); err != nil {
		t.Fatal(err)
	}
	assertPrivateDACL(t, path)
}
