//go:build linux && mnet_enrollment_development

package osownership

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestOwnedEffectInvalidatesBeforeFakeEffectAndStaysUncertain(t *testing.T) {
	for _, fail := range []bool{false, true} {
		r, _, _, _ := heldSupplierRig(t)
		c := &ownedCreation{handle: r.ledger.current.handle, fd: &fileCustody{file: &os.File{}, lineage: r.ledger}}
		initial := r.ledger.current
		called := false
		e := c.guardedEffect(c.handle, "config-intent", func() error {
			called = true
			if !r.ledger.current.uncertain || r.ledger.current.generation != initial.generation+1 || r.ledger.matches(initial) {
				t.Fatal("effect before invalidation")
			}
			if fail {
				return errors.New("fake ambiguous effect")
			}
			return nil
		})
		if !called || (e != nil) != fail || !r.ledger.current.uncertain || r.ledger.matches(initial) {
			t.Fatal("effect result restored evidence", e)
		}
	}
}
func TestOwnedEffectWrongHandleMissingLineageAndEscapeRefuse(t *testing.T) {
	r, _, _, _ := heldSupplierRig(t)
	c := &ownedCreation{handle: r.ledger.current.handle, fd: &fileCustody{file: &os.File{}, lineage: r.ledger}}
	wrong := c.handle
	wrong.Link.Index++
	called := false
	if e := c.guardedEffect(wrong, "config-intent", func() error { called = true; return nil }); e == nil || called {
		t.Fatal("wrong handle effect")
	}
	for _, kind := range []string{"ioctl-intent", "duplicate-intent", "export-intent"} {
		if e := c.refuseControlEscape(c.handle, kind); e == nil || !r.ledger.current.uncertain {
			t.Fatal("escape", kind)
		}
	}
	c.fd.lineage = nil
	if e := c.guardedEffect(c.handle, "config-intent", func() error { called = true; return nil }); e == nil || called {
		t.Fatal("unbound effect")
	}
}
func TestRealSourceLineageHooksPrecedeEffectsNoExecution(t *testing.T) {
	for _, v := range []struct{ file, fn, guard, effect string }{
		{"rtnetlink_linux.go", "applyExact", "guardedEffect", "openRouteSocket"},
		{"custody_effects_live_linux.go", "disableOwnedIPv6", "guardedEffect", "unix.Open("},
		{"file_custody.go", "Close", "lineage.invalidate", "closer.Close"},
	} {
		src := sourceFunction(t, v.file, v.fn)
		a, b := strings.Index(src, v.guard), strings.Index(src, v.effect)
		if a < 0 || b < 0 || a > b {
			t.Fatal("missing/reordered hook", v.file)
		}
	}
	src := sourceFunction(t, "sysctl_read_linux.go", "readPolicySysctlsSource")
	for _, needed := range []string{"effectiveV4Leaves", "parseEffectiveV4", "force_forwarding"} {
		if !strings.Contains(src, needed) {
			t.Fatal("reader disconnected", needed)
		}
	}
}
