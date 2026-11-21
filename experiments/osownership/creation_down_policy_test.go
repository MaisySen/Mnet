//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"errors"
	"testing"
)

func creationDownFixture(t *testing.T) ownedSourceDiagnostic {
	t.Helper()
	b, _, m := packetBindingRig(t, false)
	id := heldPacketIdentity{fdObject: m.sample.original, generation: 1, target: m.sample.namespace, device: m.sample.namespace, identity: m.sample.identity, handle: m.sample.handle}
	ledger, e := newHeldPacketLedger(id)
	if e != nil {
		t.Fatal(e)
	}
	held, e := heldSourceSnapshot(ledger, b.owner, b.session)
	if e != nil {
		t.Fatal(e)
	}
	link := m.sample.link
	link.Raw = cloneRawAttrs(link.Raw)
	link.Raw[18] = nestedTunTest()
	af, _ := fullAFSynthetic()
	link.Raw[26] = af.Raw[26]
	sys := ipv6PolicyOperandFixture()
	for _, p := range effectiveV4Paths() {
		sys[p] = "0"
	}
	sys["/proc/sys/net/ipv6/conf/all/accept_source_route"] = "-1"
	sys["/proc/sys/net/ipv4/conf/"+link.Name+"/forwarding"] = "0"
	p := "/proc/sys/net/ipv6/conf/" + link.Name + "/"
	sys[p+"forwarding"] = "0"
	sys[p+"force_forwarding"] = "0"
	sys[p+"disable_ipv6"] = "1"
	sys[p+"addr_gen_mode"] = "1"
	d, e := diagnosticForHeldRecords(RawInventory{Links: []LinkRecord{link}}, sys, held)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func phaseWord(t *testing.T, p ownedTUNPolicyAssessment, f, i int) ownedTUNWordPolicy {
	t.Helper()
	for _, w := range p.Words {
		if w.Family == f && w.Index == i {
			return w
		}
	}
	t.Fatal("missing word", f, i)
	return ownedTUNWordPolicy{}
}
func setCreationWord(t *testing.T, d *ownedSourceDiagnostic, f, i int, n uint32) {
	t.Helper()
	a, e := decodeAttributes(d.raw.Links[0].Raw[26])
	if e != nil {
		t.Fatal(e)
	}
	b, e := decodeAttributes(a[uint16(f)])
	if e != nil {
		t.Fatal(e)
	}
	key := uint16(1)
	if f == 10 {
		key = 2
	}
	binary.NativeEndian.PutUint32(b[key][4*i:], n)
	a[uint16(f)] = encodeIPv6PolicyTestAttributes(b)
	d.raw.Links[0].Raw[26] = encodeIPv6PolicyTestAttributes(a)
}
func TestCreationDownPredicatesResolveOnlyExcludedEffects(t *testing.T) {
	d := creationDownFixture(t)
	p, e := assessCreationDownPolicy(d)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Words) != 93 || p.Unresolved != 75 {
		t.Fatalf("resolved count %d/%d", p.Unresolved, len(p.Words))
	}
	for _, x := range []struct{ f, i int }{{2, 0}, {2, 1}, {2, 3}, {2, 5}, {2, 8}, {2, 14}, {2, 15}, {2, 25}, {2, 31}, {10, 0}, {10, 3}, {10, 4}, {10, 5}, {10, 10}, {10, 24}, {10, 26}, {10, 48}, {10, 59}} {
		w := phaseWord(t, p, x.f, x.i)
		if !w.Resolved || !w.Satisfied || w.Decision.Decision != "creation-down-excluded-effect" {
			t.Fatal("not resolved", w)
		}
	}
	for _, x := range []struct{ f, i int }{{2, 7}, {10, 36}, {10, 47}, {10, 44}, {10, 51}} {
		if phaseWord(t, p, x.f, x.i).Resolved {
			t.Fatal("unproved resolution", x)
		}
	}
}
func TestCreationDownPredicatesRejectTransitionStateAndLineage(t *testing.T) {
	for _, mode := range []string{"up", "address", "route", "nhid", "multipath", "pending", "generation", "lost-lineage", "wrong-owner", "proc-local-mismatch", "missing-all"} {
		t.Run(mode, func(t *testing.T) {
			d := creationDownFixture(t)
			switch mode {
			case "up":
				d.raw.Links[0].Flags |= 1
			case "address":
				d.raw.Addresses = []AddressRecord{{Index: d.raw.Links[0].Index}}
			case "route":
				d.raw.Routes = []RouteRecord{{OIF: uint32(d.raw.Links[0].Index)}}
			case "nhid":
				d.raw.Routes = []RouteRecord{{Raw: map[uint16][]byte{30: u32(1)}}}
			case "multipath":
				d.raw.Routes = []RouteRecord{{Raw: map[uint16][]byte{9: {0}}}}
			case "pending":
				d.held.ledger.setup = &packetSetupTransition{}
			case "generation":
				d.held.identity.generation = 2
			case "lost-lineage":
				_ = d.held.ledger.invalidate("export-intent")
			case "wrong-owner":
				d.held.owner = &Transaction{}
			case "proc-local-mismatch":
				d.sys["/proc/sys/net/ipv6/conf/"+d.raw.Links[0].Name+"/disable_ipv6"] = "0"
			case "missing-all":
				delete(d.sys, "/proc/sys/net/ipv6/conf/all/forwarding")
			}
			if _, e := assessCreationDownPolicy(d); e == nil {
				t.Fatal("accepted", mode)
			}
		})
	}
}
func TestCreationDownEnabledGatesStayUnresolved(t *testing.T) {
	for _, x := range []struct {
		f, i int
		all  string
	}{{2, 3, "accept_redirects"}, {2, 8, "accept_source_route"}, {2, 14, "disable_xfrm"}, {2, 15, "disable_policy"}, {2, 25, "route_localnet"}, {10, 3, ""}, {10, 5, ""}, {10, 48, "disable_policy"}} {
		d := creationDownFixture(t)
		setCreationWord(t, &d, x.f, x.i, 1)
		if x.all != "" {
			family := "ipv4"
			if x.f == 10 {
				family = "ipv6"
			}
			d.sys["/proc/sys/net/"+family+"/conf/all/"+x.all] = "1"
		}
		p, e := assessCreationDownPolicy(d)
		if e != nil {
			t.Fatal(e)
		}
		if phaseWord(t, p, x.f, x.i).Resolved {
			t.Fatal("enabled effect resolved", x)
		}
	}
}
func TestIPv6HandlerExactUnsignedBounds(t *testing.T) {
	for _, x := range []struct{ leaf, value string }{{"force_forwarding", "-1"}, {"force_forwarding", "2"}, {"addr_gen_mode", "4"}, {"ioam6_enabled", "2"}, {"ioam6_id", "65536"}, {"ra_defrtr_metric", "0"}, {"accept_untracked_na", "3"}, {"ndisc_tclass", "256"}} {
		s := ipv6PolicyOperandFixture()
		s["/proc/sys/net/ipv6/conf/all/"+x.leaf] = x.value
		if _, e := parseIPv6PolicyOperands(s); e == nil {
			t.Fatal("handler range ignored", x)
		}
	}
}
func TestIPv6GapRemainsUnsupportedNotZeroOrConfigProof(t *testing.T) {
	cause := errors.New("inert read error")
	for _, leafMissing := range []bool{false, true} {
		g := &ipv6OperandReadGap{Path: "ipv6/conf/all/seg6_require_hmac", Config: ipv6OperandConfig(45), ParentOpened: true, FinalLeafMissing: leafMissing, Cause: cause}
		if !errors.Is(g, cause) {
			t.Fatal("lost cause")
		}
		s := ipv6PolicyOperandFixture()
		delete(s, "/proc/sys/net/ipv6/conf/all/seg6_require_hmac")
		if _, e := parseIPv6PolicyOperands(s); e == nil {
			t.Fatal("conditional absence became zero")
		}
	}
	if len(ipv6CompiledOutSlots("CONFIG_IPV6_ROUTER_PREF")) != 4 || len(ipv6CompiledOutSlots("CONFIG_IPV6_ROUTE_INFO")) != 2 || ipv6CompiledOutSlots("unknown") != nil {
		t.Fatal("wrong emitter group")
	}
}
