//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
)

// Alternate adapter identity exercises the common contract, not a concrete fake
// assertion. Delegated effects are still the same Go-memory-only Kernel.
type alternateLedgerAdapter struct {
	Kernel
	ledger *adapterSession
}

func (a *alternateLedgerAdapter) ownershipSession() *adapterSession { return a.ledger }
func TestLedgerInterfaceAcceptsExactSessionNotConcreteInertType(t *testing.T) {
	b, r, m := packetBindingRig(t, false)
	s := b.session
	s.packetCreation = nil
	s.owner = nil
	tx := m.sample.owner
	tx.k = &alternateLedgerAdapter{Kernel: tx.k, ledger: s}
	next, e := bindPacketCreation(nextPacketPass(t, r))
	if e != nil || next.owner != tx || s.owner != tx {
		t.Fatal("interface creation", e)
	}
	// Original owner is fixed at creation, not stolen by first ready observation.
	wrong := *tx
	if s.owner == &wrong {
		t.Fatal("identity")
	}
	m.sample.owner = &wrong
	s.packetCreation = nil
	if _, e = bindPacketCreation(nextPacketPass(t, r)); e == nil {
		t.Fatal("owner replace")
	}
}
func TestLedgerInterfaceActiveBindingAndWrongSession(t *testing.T) {
	b, r, _ := packetBindingRig(t, true)
	wrapped := &alternateLedgerAdapter{Kernel: b.owner.k, ledger: b.session}
	b.owner.k = wrapped
	if _, e := b.policy(nextPacketPass(t, r)); e != nil {
		t.Fatal("concrete dependency", e)
	}
	wrapped.ledger = &adapterSession{}
	if _, e := b.policy(nextPacketPass(t, r)); e == nil {
		t.Fatal("foreign ledger")
	}
}
func effectiveRig(t *testing.T) (*adapterSession, *inertAFReader, *fullAFMemorySource) {
	t.Helper()
	s, p, _, source := diagnosticComposeRig(t, 6)
	for _, path := range effectiveV4Paths() {
		source.sys[path] = "0"
	}
	return s, p.reader, source
}
func setEffectiveLocal(t *testing.T, source *fullAFMemorySource, index int, n int32) {
	t.Helper()
	families, e := decodeAttributes(source.link.Raw[26])
	if e != nil {
		t.Fatal(e)
	}
	attrs, e := decodeAttributes(families[2])
	if e != nil {
		t.Fatal(e)
	}
	binary.NativeEndian.PutUint32(attrs[1][index*4:], uint32(n))
	families[2] = attr(1, attrs[1])
	source.link.Raw[26] = append(attr(2, families[2]), attr(10, families[10])...)
}
func TestEffectiveALLFormulasAndExactTablePaths(t *testing.T) {
	s, r, source := effectiveRig(t)
	source.sys["/proc/sys/net/ipv4/conf/all/send_redirects"] = "1"
	source.sys["/proc/sys/net/ipv4/conf/all/accept_source_route"] = "1"
	source.sys["/proc/sys/net/ipv4/conf/all/rp_filter"] = "2"
	source.sys["/proc/sys/net/ipv4/conf/all/accept_redirects"] = "1"
	source.sys["/proc/sys/net/ipv4/conf/default/send_redirects"] = "99" // recorded, NEVER used as ALL
	setEffectiveLocal(t, source, 7, 1)
	setEffectiveLocal(t, source, 13, -1)
	p, e := r.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	words, e := p.effectiveV4(s.canonical.Link.Index)
	if e != nil {
		t.Fatal(e)
	}
	if len(words) != 33 || !words[5].Computed || words[5].Value != 1 || words[5].Default != 99 || words[8].Value != 0 || words[7].Value != 2 || words[3].Value != 1 || words[13].Value != -1 {
		t.Fatal("OR/AND/MAX/redirect/signed/default", words)
	}
	for _, index := range []int{9, 16, 26, 27} {
		if words[index].Computed {
			t.Fatal("invented effective proof", index)
		}
	}
	if words[9].AllObserved {
		t.Fatal("invented bootp read")
	}
	for _, path := range source.paths {
		if strings.Contains(path, "bootp_relay") || strings.Contains(path, "src_vmark") || strings.Contains(path, "/noxfrm") {
			t.Fatal("invented sysctl path", path)
		}
	}
	// Conditional ACCEPT_REDIRECTS uses LOCAL forwarding, not all.forwarding.
	setEffectiveLocal(t, source, 0, 1)
	p, e = r.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	words, e = p.effectiveV4(s.canonical.Link.Index)
	if e != nil || words[3].Value != 0 {
		t.Fatal("forwarding conditional", e)
	}
}
func TestEffectiveALLSnapshotFreshnessProfileAndFailure(t *testing.T) {
	s, r, source := effectiveRig(t)
	legacy, e := r.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = legacy.effectiveV4(s.canonical.Link.Index); e == nil {
		t.Fatal("legacy has effective profile")
	}
	p, e := r.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	p.sys["/proc/sys/net/ipv4/conf/all/send_redirects"] = "1"
	if _, e = p.effectiveV4(s.canonical.Link.Index); e == nil {
		t.Fatal("post-pass substitution")
	}
	p, e = r.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	delete(source.sys, "/proc/sys/net/ipv4/conf/all/arp_filter")
	if _, e = r.readEffective(context.Background()); e == nil {
		t.Fatal("missing leaf")
	}
	if _, e = p.effectiveV4(s.canonical.Link.Index); e == nil {
		t.Fatal("stale after failed collection")
	}
}
func TestEffectiveALLCanonicalSignedValuesAndNoMutation(t *testing.T) {
	for _, bad := range []string{"01", "+1", "2147483648", "-2147483649", "", "false"} {
		_, r, source := effectiveRig(t)
		source.sys["/proc/sys/net/ipv4/conf/all/medium_id"] = bad
		if _, e := r.readEffective(context.Background()); e == nil {
			t.Fatal("invalid canonical32", bad)
		}
	}
	s, r, source := effectiveRig(t)
	source.sys["/proc/sys/net/ipv4/conf/all/medium_id"] = "-1"
	before := cloneRawAttrs(source.link.Raw)
	p, e := r.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	words, e := p.effectiveV4(s.canonical.Link.Index)
	if e != nil || words[13].All != -1 || !reflect.DeepEqual(before, source.link.Raw) {
		t.Fatal("signed/raw preservation", e)
	}
}
