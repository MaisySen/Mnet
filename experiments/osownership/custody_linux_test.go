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
func TestNoHandleCreationReadbackFailureRetainsOriginal(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	closer := &countOriginalCloser{}
	attempt := &originalAttempt{plan: p, custody: &fileCustody{closer: closer}}
	got, e := bindOriginalCustody(attempt, p, func() (Link, error) { return Link{}, ErrConflict })
	if e == nil || got == nil || got.fd != attempt.custody || attempt.handle != nil || closer.calls != 0 {
		t.Fatal("lost error custody", e)
	}
	if e = attempt.closeOriginal(); e != nil || closer.calls != 1 {
		t.Fatal("close owed", e, closer.calls)
	}
}
func TestActualBindOriginalCustodyMismatchedLinkRetainsOriginal(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	closer := &countOriginalCloser{}
	attempt := &originalAttempt{plan: p, custody: &fileCustody{closer: closer}}
	h, _ := AttachCreated(p, 7, 1500)
	wrong := h.CreationLink
	wrong.Alias = "foreign"
	got, e := bindOriginalCustody(attempt, p, func() (Link, error) { return wrong, nil })
	if e == nil || got == nil || got.fd != attempt.custody || attempt.handle != nil || closer.calls != 0 {
		t.Fatal("mismatched link accepted/lost original", e)
	}
	_ = attempt.closeOriginal()
	if closer.calls != 1 {
		t.Fatal("original not closed")
	}
}
func TestOriginalCreationHasNoAliasUntilExplicitACK(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	closer := &countOriginalCloser{}
	attempt := &originalAttempt{plan: p, custody: &fileCustody{closer: closer}}
	h, _ := AttachCreated(p, 7, 1500)
	created, e := bindOriginalCustody(attempt, p, func() (Link, error) { return h.CreationLink, nil })
	if e != nil {
		t.Fatal(e)
	}
	defer created.fd.Close()
	if created.handle.TagConfirmed || created.handle.CreationLink.Alias != "" || created.handle.CreationLink.MTU != 1500 {
		t.Fatal("tagging claimed at create")
	}
}
func TestResidentCommandPlacementSourceNoExecution(t *testing.T) {
	cmd := residentCommand("/fixed/source", 1000, 1000, nil, Namespace{Device: 7, Inode: 9}, 123, nil)
	s := cmd.SysProcAttr
	if s.Cloneflags != 0x10000000|0x40000000 || s.GidMappingsEnableSetgroups || s.UidMappings[0].HostID != 1000 || s.GidMappings[0].HostID != 1000 || len(cmd.ExtraFiles) != 1 {
		t.Fatal("resident placement changed")
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "--osownership-resident-no-tun-source" {
		t.Fatal("wrong reexec mode")
	}
	if kernelExecutionReleased {
		t.Fatal("source execution guard changed")
	}
}
