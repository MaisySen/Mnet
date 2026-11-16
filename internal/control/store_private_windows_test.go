//go:build windows

package control

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func assertControlPrivateDACL(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		t.Fatalf("state descriptor: %v", err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("state DACL inherits parent grants")
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		t.Fatal("state owner missing")
	}
	allowed := map[string]bool{owner.String(): true, "S-1-5-18": true, "S-1-5-32-544": true}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		t.Fatal("state DACL absent/empty")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatal("unexpected state ACE type")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !allowed[sid.String()] {
			t.Fatalf("unexpected state access grant: %s", sid.String())
		}
	}
}

func TestControlStateWindowsDACLAfterLoadAndReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Broad existing state simulates an inherited file that needs protection.
	broad, err := windows.SecurityDescriptorFromString("D:(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := broad.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	parentBefore, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	assertControlPrivateDACL(t, path)
	for _, name := range []string{"first", "second"} {
		if _, err := st.CreateNetwork(name); err != nil {
			t.Fatal(err)
		}
		assertControlPrivateDACL(t, path)
	}
	parentAfter, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || parentBefore.String() != parentAfter.String() {
		t.Fatal("shared parent security descriptor changed")
	}
	reopened, err := Open(path)
	if err != nil || len(reopened.Networks()) != 2 {
		t.Fatal("protected replacement not readable", err)
	}
	assertControlPrivateDACL(t, path)
	matches, err := filepath.Glob(filepath.Join(root, ".mnet-state-*"))
	if err != nil || len(matches) != 0 {
		t.Fatal("state temporary files leaked")
	}
}
