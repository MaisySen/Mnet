//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"testing"
)

// Independent route dump body in fib_dump_info emission order, not EncodeRoute.
func addressLocalRouteEmitter(h *Handle) []byte {
	b := []byte{2, 32, 0, 0, 255, 2, 254, 2, 0, 0, 0, 0}
	for _, a := range [][]byte{attr(15, u32(255)), attr(1, ipBytes(h.Address.Prefix.Addr())), attr(7, ipBytes(h.Address.Prefix.Addr())), attr(4, u32(uint32(h.Link.Index)))} {
		b = append(b, a...)
	}
	return b
}
func addressDownDeltaFixture(t *testing.T) (Handle, RawInventory, SysctlSnapshot, RawInventory) {
	t.Helper()
	b, i, n := rig()
	p, e := Prepare(b, i, n)
	if e != nil {
		t.Fatal(e)
	}
	h, e := Attach(p, 7)
	if e != nil {
		t.Fatal(e)
	}
	af, _ := fullAFSynthetic()
	families, e := decodeAttributes(af.Raw[26])
	if e != nil {
		t.Fatal(e)
	}
	a6, e := decodeAttributes(families[10])
	if e != nil {
		t.Fatal(e)
	}
	binary.NativeEndian.PutUint32(a6[2][26*4:], 1)
	families[10] = encodeIPv6PolicyTestAttributes(a6)
	l := LinkRecord{Index: 7, Name: h.Link.Name, Alias: h.Link.Alias, MTU: h.Link.MTU, Hardware: 65534, Flags: 0x1090, Raw: map[uint16][]byte{26: encodeIPv6PolicyTestAttributes(families)}}
	before := RawInventory{Links: []LinkRecord{l}, Rules: [][]byte{{1, 2, 3}}}
	sys := SysctlSnapshot{"/proc/sys/net/ipv6/conf/" + h.Link.Name + "/disable_ipv6": "1", "/proc/sys/net/ipv4/conf/" + h.Link.Name + "/forwarding": "0"}
	after := cloneRawExact(before)
	a, e := DecodeAddress(addressEmitterFixture(&h, 7, 12))
	if e != nil {
		t.Fatal(e)
	}
	r, e := DecodeRoute(addressLocalRouteEmitter(&h))
	if e != nil {
		t.Fatal(e)
	}
	after.Addresses = []AddressRecord{a}
	after.Routes = []RouteRecord{r}
	return h, before, sys, after
}
func TestAddressDownCombinedEmitterDelta(t *testing.T) {
	h, before, sys, after := addressDownDeltaFixture(t)
	c, e := captureAddressDownDelta(&h, before, sys, after, cloneSysExact(sys))
	if e != nil || c == nil {
		t.Fatal(e)
	}
	if !c.matches(&h, addressEmitterFixture(&h, 7, 12)) {
		t.Fatal("cache evidence lost")
	}
	after.Addresses[0].Raw[6][8] ^= 1
	if !c.matches(&h, addressEmitterFixture(&h, 7, 12)) {
		t.Fatal("output aliases dump")
	}
}
func TestAddressDownCombinedRejectsUnmodeledDelta(t *testing.T) {
	for _, mode := range []string{"missing-address", "missing-route", "extra-route-table", "extra-address", "rule", "ipv4-conf", "sysctl", "up", "disable", "route-linkdown", "route-offload", "route-extra-zero-priority", "route-raw-oif", "route-raw-dst", "address-raw-local", "cache-lifetime"} {
		t.Run(mode, func(t *testing.T) {
			h, before, sys, after := addressDownDeltaFixture(t)
			observed := cloneSysExact(sys)
			switch mode {
			case "missing-address":
				after.Addresses = nil
			case "missing-route":
				after.Routes = nil
			case "extra-route-table":
				r := after.Routes[0]
				r.Table = 254
				r.TableID = 254
				after.Routes = append(after.Routes, r)
			case "extra-address":
				after.Addresses = append(after.Addresses, after.Addresses[0])
			case "rule":
				after.Rules[0][0] ^= 1
			case "ipv4-conf":
				f, _ := decodeAttributes(after.Links[0].Raw[26])
				a, _ := decodeAttributes(f[2])
				a[1][0] ^= 1
				f[2] = encodeIPv6PolicyTestAttributes(a)
				after.Links[0].Raw[26] = encodeIPv6PolicyTestAttributes(f)
			case "sysctl":
				observed["/proc/sys/net/ipv4/conf/"+h.Link.Name+"/forwarding"] = "1"
			case "up":
				after.Links[0].Flags |= 1
			case "disable":
				observed["/proc/sys/net/ipv6/conf/"+h.Link.Name+"/disable_ipv6"] = "0"
			case "route-linkdown":
				after.Routes[0].Flags = 16
			case "route-offload":
				after.Routes[0].Flags = 0x4000
			case "route-extra-zero-priority":
				after.Routes[0].Raw[6] = u32(0)
			case "route-raw-oif":
				after.Routes[0].Raw[4] = u32(99)
			case "route-raw-dst":
				after.Routes[0].Raw[1] = []byte{192, 0, 2, 3}
			case "address-raw-local":
				after.Addresses[0].Raw[2] = []byte{192, 0, 2, 3}
			case "cache-lifetime":
				after.Addresses[0].Raw[6][0] = 0
			}
			if _, e := captureAddressDownDelta(&h, before, sys, after, observed); e == nil {
				t.Fatal("drift admitted")
			}
		})
	}
}
func TestAddressLocalRouteRequiresExactAttrs(t *testing.T) {
	h, _, _, after := addressDownDeltaFixture(t)
	for _, key := range []uint16{1, 4, 7, 15} {
		r := after.Routes[0]
		r.Raw = cloneRawAttrs(r.Raw)
		delete(r.Raw, key)
		if validateAddressLocalRoute(&h, r) == nil {
			t.Fatal("missing accepted", key)
		}
	}
	for _, key := range []uint16{5, 6, 8, 9, 21, 30} {
		r := after.Routes[0]
		r.Raw = cloneRawAttrs(r.Raw)
		r.Raw[key] = u32(0)
		if validateAddressLocalRoute(&h, r) == nil {
			t.Fatal("extra accepted", key)
		}
	}
	for _, suffix := range [][]byte{attr(15, u32(255)), attr(4|0x8000, u32(7))} {
		if _, e := DecodeRoute(append(addressLocalRouteEmitter(&h), suffix...)); e == nil {
			t.Fatal("bad wire accepted")
		}
	}
}
