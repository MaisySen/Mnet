//go:build linux && mnet_enrollment_development

package osownership

import (
	"os"
	"testing"
)

func TestActualFileCustodyReceivesOriginalLedgerAndEffectUsesIt(t *testing.T) {
	b, _, m := packetBindingRig(t, false)
	// Zero os.File is an inert nonnil identity placeholder. No method on it is
	// invoked. The effect callback below changes only local fake counters.
	c := &ownedCreation{handle: m.sample.handle, fd: &fileCustody{file: &os.File{}}}
	ns := &namespaceCustody{identity: m.sample.namespace}
	id := heldPacketIdentity{generation: 1, target: m.sample.namespace, device: m.sample.namespace, identity: m.sample.identity, handle: m.sample.handle}
	original, e := retainOriginalPacketResources(c, ns, b.owner, b.session, id)
	if e != nil {
		t.Fatal(e)
	}
	if c.fd.lineage != original.ledger || original.creation != c || original.namespace != ns || original.ledger.snapshot().fdObject == nil {
		t.Fatal("detached token")
	}
	if _, e = retainOriginalPacketResources(c, ns, b.owner, b.session, id); e == nil {
		t.Fatal("rebound original")
	}
	called := false
	if e = c.guardedEffect(c.handle, "config-intent", func() error {
		called = true
		if !original.ledger.snapshot().uncertain {
			t.Fatal("actual attached lineage not invalidated")
		}
		return nil
	}); e != nil || !called {
		t.Fatal(e)
	}
	c.fd.mu.Lock()
	c.fd.closed = true
	c.fd.mu.Unlock()
	if e = c.guardedEffect(c.handle, "config-intent", func() error { t.Fatal("closed effect"); return nil }); e == nil {
		t.Fatal("closed custody admitted")
	}
}

func TestHeldDiagnosticKeepsParsedEffectiveAndFramingData(t *testing.T) {
	b, _, m := packetBindingRig(t, false)
	id := heldPacketIdentity{fdObject: m.sample.original, generation: 1, target: m.sample.namespace, device: m.sample.namespace, identity: m.sample.identity, handle: m.sample.handle}
	l, e := newHeldPacketLedger(id)
	if e != nil {
		t.Fatal(e)
	}
	held, e := heldSourceSnapshot(l, b.owner, b.session)
	if e != nil {
		t.Fatal(e)
	}
	link := m.sample.link
	link.Raw = cloneRawAttrs(link.Raw)
	link.Raw[18] = nestedTunTest()
	sys := SysctlSnapshot{}
	for _, p := range effectiveV4Paths() {
		sys[p] = "0"
	}
	sys["/proc/sys/net/ipv4/conf/all/send_redirects"] = "1"
	raw := RawInventory{Links: []LinkRecord{link}}
	d, e := diagnosticForHeldRecords(raw, sys, held)
	if e != nil || d.held != held || d.framing.typ != 1 || len(d.effective) != 33 || d.effective[5].Value != 1 || !d.lineageUsable {
		t.Fatal("discarded parsed data", e)
	}
	sys["/proc/sys/net/ipv4/conf/all/send_redirects"] = "0"
	if d.effective[5].Value != 1 || d.sys["/proc/sys/net/ipv4/conf/all/send_redirects"] != "1" {
		t.Fatal("mutable operands")
	}
	_ = l.invalidate("close-intent")
	if _, e = diagnosticForHeldRecords(raw, sys, held); e == nil {
		t.Fatal("held generation not bound")
	}
}
