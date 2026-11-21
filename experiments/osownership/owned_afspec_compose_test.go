//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"maps"
	"net/netip"
	"reflect"
	"testing"
)

// Drives real ledger transitions with value fake readbacks. Full raw pass is
// built separately through the inert recording reader, no kernel supplier.
func diagnosticComposeRig(t *testing.T, steps int) (*adapterSession, *afReadPass, RawInventory, *fullAFMemorySource) {
	t.Helper()
	_, adapter, k := protocolRig(t)
	s := adapter.session
	raw, sys := recordedRaw(t)
	baseInv, e := s.baseline.Compose(raw, sys, nil, false)
	if e != nil {
		t.Fatal(e)
	}
	binding, _, now := rig()
	binding.Namespace = s.baseline.ns
	plan, e := Prepare(binding, baseInv, now)
	if e != nil {
		t.Fatal(e)
	}
	h, e := k.Create(context.Background(), plan)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.bindCreated(plan, h); e != nil {
		t.Fatal(e)
	}
	l, synthetic := fullAFSynthetic()
	oldName := l.Name
	l.Name = h.CreationLink.Name
	l.MTU = h.CreationLink.MTU
	l.Raw[3] = append([]byte(l.Name), 0)
	l.Raw[4] = u32(l.MTU)
	renamed := maps.Clone(sys)
	for path, value := range synthetic {
		for n := 0; n+len(oldName) <= len(path); n++ {
			if path[n:n+len(oldName)] == oldName {
				path = path[:n] + l.Name + path[n+len(oldName):]
				break
			}
		}
		if _, baseline := renamed[path]; !baseline {
			renamed[path] = value
		}
	}
	for _, baseline := range raw.Links {
		applicable, e := LinkIPv6Applicable(baseline)
		if e != nil {
			t.Fatal(e)
		}
		if applicable {
			renamed["/proc/sys/net/ipv6/conf/"+baseline.Name+"/force_forwarding"] = "0"
		}
	}
	source := &fullAFMemorySource{link: l, extra: raw.Links, sys: renamed}
	reader := &inertAFReader{source: source, localPID: 77}
	pass, e := reader.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.bindCreationApplicability(pass); e != nil {
		t.Fatal(e)
	}
	for n := 1; n < steps; n++ {
		op := plan.Operations[n]
		if e = s.begin(h, op); e != nil {
			t.Fatal(e)
		}
		if e = k.Apply(context.Background(), h, op); e != nil {
			t.Fatal(e)
		}
		if e = s.recordACK(nil); e != nil {
			t.Fatal(e)
		}
		if e = s.observe(k.i); e != nil {
			t.Fatal(e)
		}
		h = *s.canonical
	}
	source.link.Alias = h.Link.Alias
	source.link.MTU = h.Link.MTU
	source.link.Raw[20] = append([]byte(h.Link.Alias), 0)
	source.link.Raw[4] = u32(h.Link.MTU)
	source.link.Flags = 1
	pass, e = reader.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	raw.Links, e = pass.linkSnapshot()
	if e != nil {
		t.Fatal(e)
	}
	if steps >= 4 {
		raw.Addresses = []AddressRecord{{Index: h.Link.Index, Family: 2, Prefix: 32, Flags: 0x80, Local: h.Address.Prefix.Addr(), Address: h.Address.Prefix.Addr(), Raw: map[uint16][]byte{8: u32(0x280)}}}
	}
	if steps >= 6 {
		raw.Routes = []RouteRecord{{Family: 2, DestBits: 32, Table: 254, TableID: 254, Protocol: 242, Scope: 253, Type: 1, Dst: h.Route.Destination.Addr(), OIF: uint32(h.Link.Index), Priority: h.Route.Priority}}
	}
	return s, pass, raw, source
}
func TestCanonicalOwnedDiagnosticComposeNeverReady(t *testing.T) {
	s, p, raw, _ := diagnosticComposeRig(t, 6)
	if _, e := s.baseline.Compose(raw, p.sys, s.canonical, true); e == nil || e.Error() != "unreviewed owned link attr 26" {
		t.Fatal("legacy generic path changed", e)
	}
	before := cloneRawExact(raw)
	d, e := s.composeOwnedTUNDiagnostic(raw, p)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, raw) || d.policy.Unresolved == 0 || len(d.policy.Words) != 93 {
		t.Fatal("policy/bytes lost")
	}
	if _, e = d.activeInventory(); !errors.Is(e, ownedAFPolicyIncomplete) {
		t.Fatal("ignored unresolved policy promoted", e)
	}
	// Even in-package tampering with an unchecked flag cannot produce readiness.
	c, e := s.disabledAFCandidate(p)
	if e != nil {
		t.Fatal(e)
	}
	c.uncheckedDevconf = false
	if _, e = d.activeInventory(); !errors.Is(e, ownedAFPolicyIncomplete) {
		t.Fatal("candidate flag became authority", e)
	}
	originalMTU := p.links[0].MTU
	raw.Links[0].MTU++
	if p.links[0].MTU != originalMTU {
		t.Fatal("pass aliased returned snapshot")
	}
	if _, e = s.composeOwnedTUNDiagnostic(raw, p); !errors.Is(e, afRequest) {
		t.Fatal("caller raw differs from received pass", e)
	}
}
func TestDiagnosticComposeBaselineAndIPv6PhaseRefusals(t *testing.T) {
	for _, phase := range []int{3, 4, 5, 6} {
		for _, kind := range []string{"address", "route"} {
			t.Run(string(rune('0'+phase))+kind, func(t *testing.T) {
				s, p, raw, _ := diagnosticComposeRig(t, phase)
				if kind == "address" {
					raw.Addresses = append(raw.Addresses, AddressRecord{Index: s.canonical.Link.Index, Family: 10, Local: netip.MustParseAddr("fe80::1"), Address: netip.MustParseAddr("fe80::1")})
				} else {
					raw.Routes = append(raw.Routes, RouteRecord{Family: 10, Dst: netip.MustParseAddr("fe80::")})
				}
				if _, e := s.composeOwnedTUNDiagnostic(raw, p); !errors.Is(e, ownedAFIPv6Present) {
					t.Fatal("implicit v6 admitted", phase, kind, e)
				}
			})
		}
	}
	s, p, raw, source := diagnosticComposeRig(t, 6)
	source.extra = cloneRawExact(RawInventory{Links: source.extra}).Links
	source.extra[0].MTU++
	source.extra[0].Raw[4] = u32(source.extra[0].MTU)
	newer, e := p.reader.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	raw.Links, e = newer.linkSnapshot()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.composeOwnedTUNDiagnostic(raw, newer); e == nil {
		t.Fatal("baseline mutation accepted")
	}
}
func TestNamedPolicyDoesNotTreatRetainedWordsAsSafe(t *testing.T) {
	l, _ := fullAFSynthetic()
	mutateFull(&l, func(v4, v6 map[uint16][]byte) {
		binary.NativeEndian.PutUint32(v4[1][7*4:], 123)
		binary.NativeEndian.PutUint32(v6[2][5*4:], 456)
	})
	a, e := assessOwnedTUNPolicy(l)
	if e != nil {
		t.Fatal(e)
	}
	for _, w := range a.Words {
		if (w.Family == 2 && w.Index == 7) || (w.Family == 10 && w.Index == 5) {
			if w.Resolved || w.Name == "" {
				t.Fatal("unchecked word deemed safe", w)
			}
		}
	}
	if a.Unresolved == 0 {
		t.Fatal("retention laundered policy")
	}
}

func TestPassLinkSnapshotDeepIsolation(t *testing.T) {
	_, p, _, _ := diagnosticComposeRig(t, 6)
	first, e := p.linkSnapshot()
	if e != nil {
		t.Fatal(e)
	}
	second, e := p.linkSnapshot()
	if e != nil {
		t.Fatal(e)
	}
	before := cloneRawExact(RawInventory{Links: p.links})
	first[0].MTU++
	first[0].Raw[4][0] ^= 0xff
	if !reflect.DeepEqual(before.Links, p.links) || !reflect.DeepEqual(before.Links, second) {
		t.Fatal("returned copy aliases immutable pass")
	}
}
func TestNamedWordDecisionCoverageAndConservativeChoices(t *testing.T) {
	for index := range ownedTUNIPv4Words {
		d := ownedV4Decision(index)
		if d.Decision == "unresolved" || d.Decision == "" || d.Source == "" || d.Reason == "" {
			t.Fatal("missing grounded IPv4 decision", index, d)
		}
	}
	for index := range ownedTUNIPv6Words {
		d := ownedV6Decision(index)
		if d.Decision == "" || d.Source == "" || d.Reason == "" {
			t.Fatal("missing grounded IPv6 decision", index, d)
		}
	}
	for index := range ownedTUNIPv4Words {
		if ownedV4Decision(index).Decision == "assert" {
			t.Fatal("obsolete blanket hardening", index)
		}
	}
	l, _ := fullAFSynthetic()
	a, e := assessOwnedTUNPolicy(l)
	if e != nil {
		t.Fatal(e)
	}
	if a.Unresolved != 93 {
		t.Fatal("claims promoted without original NOARP/effective-all/membership evidence", a.Unresolved)
	}
}
