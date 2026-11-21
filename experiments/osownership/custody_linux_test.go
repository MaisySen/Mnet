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
func TestNoHandleCreationReadbackFailureClosesOriginal(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	f, e := os.CreateTemp(t.TempDir(), "create")
	if e != nil {
		t.Fatal(e)
	}
	got, e := bindOriginalFD(f, p, func() (Link, error) { return Link{}, ErrConflict })
	if e == nil || got != nil {
		t.Fatal("ambiguous create did not return error")
	}
	if _, e = f.Write([]byte("must be closed")); e == nil {
		t.Fatal("FD leaked on no-handle return")
	}
}
func TestOriginalCreationHasNoAliasUntilExplicitACK(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	f, e := os.CreateTemp(t.TempDir(), "create")
	if e != nil {
		t.Fatal(e)
	}
	h, _ := AttachCreated(p, 7, 1500)
	created, e := bindOriginalFD(f, p, func() (Link, error) { return h.CreationLink, nil })
	if e != nil {
		t.Fatal(e)
	}
	defer created.fd.Close()
	if created.handle.TagConfirmed || created.handle.CreationLink.Alias != "" || created.handle.CreationLink.MTU != 1500 {
		t.Fatal("tagging claimed at create")
	}
}
func TestResidentCommandPlacementSourceNoExecution(t *testing.T) {
	cmd := residentCommand("/fixed/source", 1000, 1000, nil, nil, nil)
	s := cmd.SysProcAttr
	if s.Cloneflags != 0x10000000|0x40000000 || s.GidMappingsEnableSetgroups || s.UidMappings[0].HostID != 1000 || s.GidMappings[0].HostID != 1000 || len(cmd.ExtraFiles) != 2 {
		t.Fatal("resident placement changed")
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "--osownership-resident-no-tun-source" {
		t.Fatal("wrong reexec mode")
	}
	if kernelExecutionReleased {
		t.Fatal("source execution guard changed")
	}
}
