//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func recordedRaw(t *testing.T) (RawInventory, SysctlSnapshot) {
	t.Helper()
	root := "testdata/read-only-run2"
	var raw RawInventory
	for _, item := range []struct {
		prefix string
		kind   uint16
		n      int
	}{{"link", 18, 3}, {"addr", 22, 1}, {"route", 26, 1}, {"rule", 34, 1}} {
		var ds []Datagram
		var first []byte
		for j := 0; j < item.n; j++ {
			b, e := os.ReadFile(filepath.Join(root, item.prefix+"-"+string(rune('0'+j))+".bin"))
			if e != nil {
				t.Fatal(e)
			}
			if j == 0 {
				first = b
			}
			ds = append(ds, Datagram{Bytes: b})
		}
		seq := binary.NativeEndian.Uint32(first[8:])
		pid := binary.NativeEndian.Uint32(first[12:])
		c := NewInMemoryNetlinkClient(wireFunc(func([]byte) ([]Datagram, error) { return ds, nil }), pid)
		c.sequence = seq - 1
		ms, e := c.requestFixture(context.Background(), item.kind, nil, true)
		if e != nil {
			t.Fatal(item.prefix, e)
		}
		for _, b := range ms {
			switch item.kind {
			case 18:
				r, e := DecodeLink(b)
				if e != nil {
					t.Fatal(e)
				}
				raw.Links = append(raw.Links, r)
			case 22:
				r, e := DecodeAddress(b)
				if e != nil {
					t.Fatal(e)
				}
				raw.Addresses = append(raw.Addresses, r)
			case 26:
				r, e := DecodeRoute(b)
				if e != nil {
					t.Fatal(e)
				}
				raw.Routes = append(raw.Routes, r)
			case 34:
				raw.Rules = append(raw.Rules, b)
			}
		}
	}
	v, e := os.ReadFile(filepath.Join(root, "child-verdict.json"))
	if e != nil {
		t.Fatal(e)
	}
	var obj struct {
		Sysctl SysctlSnapshot `json:"sysctl"`
	}
	if e = json.Unmarshal(v, &obj); e != nil {
		t.Fatal(e)
	}
	return raw, obj.Sysctl
}
func TestRecordedReadOnlyKernelBaselineCandidate(t *testing.T) {
	raw, sys := recordedRaw(t)
	if len(raw.Links) != 10 || len(raw.Addresses) != 0 || len(raw.Routes) != 0 || len(raw.Rules) != 7 {
		t.Fatal("recorded inventory counts differ")
	}
	ns := Namespace{Device: 7, Inode: 4026532293, Isolated: true}
	b, e := NewBaselineCandidate(ns, raw, sys)
	if e == nil || !strings.Contains(e.Error(), "missing/invalid IPv6 readback") {
		t.Fatal("incomplete real sysctl capture not blocked", e)
	}
	_ = b
	// Synthetic completion below tests candidate mechanics, NOT live applicability.
	sys = syntheticCompleteSysctl(raw, sys)
	b, e = NewBaselineCandidate(ns, raw, sys)
	if e != nil {
		t.Fatal(e)
	}
	i, e := b.Compose(raw, sys, nil, false)
	if e != nil || len(i.Links) != 10 {
		t.Fatal(e)
	}
	binding, _, now := rig()
	binding.Namespace = ns
	if _, e = Prepare(binding, i, now); e != nil {
		t.Fatal("immutable baseline candidate refused", e)
	}
	raw.Links[0].Raw[62] = []byte{1}
	if _, e = b.Compose(raw, sys, nil, false); e == nil {
		t.Fatal("unknown baseline attribute mutation tolerated")
	}
}
func TestCompleteDefaultRulesSingleFieldMutations(t *testing.T) {
	original, _ := recordedRaw(t)
	for _, field := range []string{"dst", "src", "tos", "res1", "res2", "action", "flags", "table", "priority", "protocol", "suppress", "new-selector", "family", "duplicate", "missing"} {
		t.Run(field, func(t *testing.T) {
			b := mustJSON(original)
			var raw RawInventory
			_ = json.Unmarshal(b, &raw)
			r := raw.Rules[0]
			switch field {
			case "dst":
				r[1] = 1
			case "src":
				r[2] = 1
			case "tos":
				r[3] = 1
			case "res1":
				r[5] = 1
			case "res2":
				r[6] = 1
			case "action":
				r[7] = 2
			case "flags":
				r[8] = 1
			case "table":
				r[4] = 254
			case "family":
				r[0] = 128
			case "new-selector":
				raw.Rules[0] = append(r, attr(10, u32(1))...)
			case "priority":
				raw.Rules[0] = append(r, attr(6, u32(9))...)
			case "protocol":
				r[32] = 3
			case "suppress":
				r[24] = 0
			case "duplicate":
				raw.Rules[1] = r
			case "missing":
				raw.Rules = raw.Rules[1:]
			}
			if e := ValidateDefaultRules(raw.Rules); e == nil {
				t.Fatal("changed default rule accepted", field)
			}
		})
	}
}
func TestBaselineForwardingUpAndUnknownStateRefused(t *testing.T) {
	for _, mode := range []string{"up", "address", "route", "forwarding", "missing-sysctl", "master", "crossns"} {
		raw, sys := recordedRaw(t)
		ns := Namespace{Device: 7, Inode: 4026532293, Isolated: true}
		switch mode {
		case "up":
			raw.Links[1].Flags |= 1
		case "address":
			raw.Addresses = []AddressRecord{{}}
		case "route":
			raw.Routes = []RouteRecord{{}}
		case "forwarding":
			sys["/proc/sys/net/ipv4/ip_forward"] = "1"
		case "missing-sysctl":
			delete(sys, "/proc/sys/net/ipv6/conf/lo/disable_ipv6")
		case "master":
			raw.Links[1].Raw[10] = u32(1)
		case "crossns":
			raw.Links[1].Raw[37] = u32(1)
		}
		if _, e := NewBaselineCandidate(ns, raw, sys); e == nil {
			t.Fatal("unsafe baseline accepted", mode)
		}
	}
}
func TestNestedEmpty62And65OnlyNoBroadTolerance(t *testing.T) {
	for _, typ := range []uint16{62, 65} {
		if _, e := decodeAttributesWithLinkInfo(attr(typ|0x8000, nil), true); e != nil {
			t.Fatal(e)
		}
		if _, e := decodeAttributesWithLinkInfo(attr(typ|0x8000, []byte{1}), true); e == nil {
			t.Fatal("unreviewed nested content accepted")
		}
	}
	if _, e := decodeAttributesWithLinkInfo(attr(66|0x8000, nil), true); e == nil {
		t.Fatal("unreviewed nested type accepted")
	}
}
func TestBaselineDeepCopyIncludesRawAttrsAndSysctl(t *testing.T) {
	r, s := recordedRaw(t)
	ns := Namespace{Device: 7, Inode: 4026532293, Isolated: true}
	s = syntheticCompleteSysctl(r, s)
	b, e := NewBaselineCandidate(ns, r, s)
	if e != nil {
		t.Fatal(e)
	}
	saved := append([]byte(nil), b.raw.Links[0].Raw[3]...)
	r.Links[0].Raw[3][0] = 'X'
	s["/proc/sys/net/ipv4/ip_forward"] = "1"
	if !reflect.DeepEqual(b.raw.Links[0].Raw[3], saved) || b.sys["/proc/sys/net/ipv4/ip_forward"] != "0" {
		t.Fatal("baseline aliases input")
	}
}

// Deliberately synthetic applicability. Never saved over captured JSON.
func syntheticCompleteSysctl(raw RawInventory, s SysctlSnapshot) SysctlSnapshot {
	out := SysctlSnapshot{}
	for k, v := range s {
		out[k] = v
	}
	for _, l := range raw.Links {
		for _, leaf := range []string{"disable_ipv6", "addr_gen_mode", "forwarding"} {
			k := "/proc/sys/net/ipv6/conf/" + l.Name + "/" + leaf
			if _, ok := out[k]; !ok {
				out[k] = "0"
			}
		}
	}
	return out
}
func TestOriginRequiresExactLocalShapeAndAddressWitness(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	r := RouteRecord{Family: 2, DestBits: 32, Table: 255, TableID: 255, Protocol: 2, Scope: 254, Type: 2, Dst: h.Address.Prefix.Addr(), PreferredSource: h.Address.Prefix.Addr(), OIF: 7}
	if _, e := composeRoute(h, r, false); e == nil {
		t.Fatal("protocol alone created origin")
	}
	if _, e := composeRoute(h, r, true); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"table", "scope", "proto", "metric", "source", "gateway", "type", "index", "unknown"} {
		bad := r
		bad.Raw = map[uint16][]byte{}
		switch field {
		case "table":
			bad.TableID = 254
		case "scope":
			bad.Scope = 253
		case "proto":
			bad.Protocol = 242
		case "metric":
			bad.Priority = 1
		case "source":
			bad.Source = h.Address.Prefix.Addr()
		case "gateway":
			bad.Gateway = h.Address.Prefix.Addr()
		case "type":
			bad.Type = 1
		case "index":
			bad.OIF = 8
		case "unknown":
			bad.Raw[99] = []byte{1}
		}
		if _, e := composeRoute(h, bad, true); e == nil {
			t.Fatal("foreign route assigned origin", field)
		}
	}
}
