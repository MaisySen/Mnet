//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"strings"
	"testing"
)

func fixtureBaseline(t *testing.T) (*CapturedBaseline, RawInventory, SysctlSnapshot) {
	t.Helper()
	r, s := recordedRaw(t)
	b, e := NewBaselineCandidate(Namespace{Device: 7, Inode: 4026531833}, Namespace{Device: 7, Inode: 4026532293, Isolated: true}, r, s)
	if e != nil {
		t.Fatal(e)
	}
	return b, r, s
}
func TestS4BaselineGuardsIndividuallyDecisive(t *testing.T) {
	for _, name := range []string{"up", "master", "underlay", "crossns", "per-link-forwarding", "ipv6-forwarding", "parent-equality"} {
		_, r, s := fixtureBaseline(t)
		parent := Namespace{Device: 7, Inode: 4026531833}
		ns := Namespace{Device: 7, Inode: 4026532293, Isolated: true}
		switch name {
		case "up":
			r.Links[1].Flags |= 1
		case "master":
			r.Links[1].Raw[10] = u32(1)
		case "underlay":
			r.Links[1].Raw[5] = u32(4)
		case "crossns":
			r.Links[1].Raw[37] = u32(1)
		case "per-link-forwarding":
			s["/proc/sys/net/ipv4/conf/lo/forwarding"] = "1"
		case "ipv6-forwarding":
			s["/proc/sys/net/ipv6/conf/lo/forwarding"] = "1"
		case "parent-equality":
			parent = ns
		}
		if _, e := NewBaselineCandidate(parent, ns, r, s); e == nil {
			t.Fatal("guard not decisive", name)
		}
	}
}
func TestS4BaselineSysctlChangedWhileStillValid(t *testing.T) {
	b, r, s := fixtureBaseline(t)
	s["/proc/sys/net/ipv6/conf/lo/addr_gen_mode"] = "2"
	if _, e := b.Compose(r, s, nil, false); e == nil || !strings.Contains(e.Error(), "baseline sysctl changed") {
		t.Fatal("valid-but-changed baseline sysctl accepted", e)
	}
}
func TestS4IPv6ApplicabilityMatchesRecordedFamilies(t *testing.T) {
	_, r, s := fixtureBaseline(t)
	count := 0
	for _, l := range r.Links {
		yes, e := LinkIPv6Applicable(l)
		if e != nil {
			t.Fatal(e)
		}
		if yes {
			count++
		}
		if e = validateIPv6Directory(l.Name, yes, s); e != nil {
			t.Fatal(e)
		}
		bad := SysctlSnapshot{}
		for k, v := range s {
			bad[k] = v
		}
		if yes {
			bad[ipv6DirPrefix+l.Name] = "ENOENT"
		} else {
			bad[ipv6DirPrefix+l.Name] = "present"
		}
		if validateIPv6Directory(l.Name, yes, bad) == nil {
			t.Fatal("AF_SPEC/directory mismatch accepted")
		}
	}
	if count != 7 {
		t.Fatal("recorded IPv6 applicability changed", count)
	}
}
func TestS4MissingNotDisabledAndAbsentLeavesRefused(t *testing.T) {
	_, r, s := fixtureBaseline(t)
	for _, l := range r.Links {
		yes, _ := LinkIPv6Applicable(l)
		if !yes {
			bad := SysctlSnapshot{}
			for k, v := range s {
				bad[k] = v
			}
			bad["/proc/sys/net/ipv6/conf/"+l.Name+"/disable_ipv6"] = "1"
			if validateIPv6Directory(l.Name, false, bad) == nil {
				t.Fatal("missing was called disabled")
			}
			delete(bad, ipv6DirPrefix+l.Name)
			if validateIPv6Directory(l.Name, false, bad) == nil {
				t.Fatal("missing evidence accepted")
			}
			return
		}
	}
	t.Fatal("no absent fixture")
}
func TestS4TagRequiredBeforeLinkUp(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := AttachCreated(p, 7, 1500)
	if _, e := BuildExactMutation(h, Operation{Kind: "link-up-exact"}); e == nil {
		t.Fatal("untagged link up encoded")
	}
}

type changedFinalStages struct{ fakeStages }

func (f *changedFinalStages) CleanupAndVerify(context.Context) (string, string, error) {
	return strings.Repeat("a", 64), strings.Repeat("b", 64), nil
}
func TestS4FinalHashMismatchFailsEvenWithoutCleanupError(t *testing.T) {
	f := &changedFinalStages{}
	v, e := RunNoTunDriver(context.Background(), context.Background(), validVerdict().Namespace, f)
	if e == nil || v.Result == "pass" || v.CleanupConfirmed {
		t.Fatal("mismatched baseline passed")
	}
}
func TestS4RuleCountDirectly(t *testing.T) {
	r, _ := recordedRaw(t)
	if e := ValidateDefaultRules(r.Rules[:6]); e == nil || e.Error() != "nondefault rule count" {
		t.Fatal("count not independently decisive", e)
	}
}
func TestS4OwnedAddressHeaderAndAttrFlagsIndependently(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	a := AddressRecord{Index: 7, Family: 2, Prefix: 32, Flags: 0x80, Local: h.Address.Prefix.Addr(), Address: h.Address.Prefix.Addr(), Raw: map[uint16][]byte{8: u32(0x280)}}
	if e := validateOwnedAddress(&h, a); e != nil {
		t.Fatal(e)
	}
	bad := a
	bad.Flags = 0
	if validateOwnedAddress(&h, bad) == nil {
		t.Fatal("header flags ignored")
	}
	for _, v := range []uint32{0, 0x80, 0x200, 0x281} {
		bad = a
		bad.Raw = map[uint16][]byte{8: u32(v)}
		if validateOwnedAddress(&h, bad) == nil {
			t.Fatal("nonexact extended flags ignored", v)
		}
	}
}
func TestS4CrossNamespaceKindsRefusedEvenUnlinked(t *testing.T) {
	for _, kind := range []string{"veth", "vxlan", "ipvlan", "macvlan", "macvtap", "geneve"} {
		_, r, s := fixtureBaseline(t)
		r.Links[1].Raw[18] = attr(1, append([]byte(kind), 0))
		r.Links[1].Raw[5] = u32(0)
		if _, e := NewBaselineCandidate(Namespace{Device: 7, Inode: 4026531833}, Namespace{Device: 7, Inode: 4026532293, Isolated: true}, r, s); e == nil || !strings.Contains(e.Error(), "cross-namespace-capable") {
			t.Fatal("cross-namespace kind permitted", kind, e)
		}
	}
}
func TestS5ApplicabilityGuardsDirect(t *testing.T) {
	for _, name := range []string{"no-afspec", "unknown-family", "no-ipv4"} {
		l := LinkRecord{Raw: map[uint16][]byte{26: append(attr(2, nil), attr(10, nil)...)}}
		switch name {
		case "no-afspec":
			delete(l.Raw, 26)
		case "unknown-family":
			l.Raw[26] = append(l.Raw[26], attr(77, nil)...)
		case "no-ipv4":
			l.Raw[26] = attr(10, nil)
		}
		if _, e := LinkIPv6Applicable(l); e == nil {
			t.Fatal("applicability guard not decisive", name)
		}
	}
}
func TestS5ZeroParentBothComponentsRefused(t *testing.T) {
	_, r, s := fixtureBaseline(t)
	ns := Namespace{Device: 7, Inode: 9, Isolated: true}
	for _, parent := range []Namespace{{}, {Device: 7}, {Inode: 8}} {
		if _, e := NewBaselineCandidate(parent, ns, r, s); e == nil {
			t.Fatal("zero parent accepted", parent)
		}
	}
}
