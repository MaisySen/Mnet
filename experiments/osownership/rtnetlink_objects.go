//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
)

// Decoded records preserve attributes not yet modeled. A future live inventory
// must refuse unknown route effects rather than drop them as harmless.
type LinkRecord struct {
	Index       int
	Hardware    uint16
	Flags       uint32
	Name, Alias string
	MTU         uint32
	Raw         map[uint16][]byte
}
type AddressRecord struct {
	Index                        int
	Family, Prefix, Flags, Scope byte
	Local, Address               netip.Addr
	Raw                          map[uint16][]byte
}
type RouteRecord struct {
	Family, DestBits, SourceBits, TOS, Table, Protocol, Scope, Type byte
	Flags                                                           uint32
	Dst, Source, Gateway, PreferredSource                           netip.Addr
	OIF, IIF, Priority, TableID                                     uint32
	Raw                                                             map[uint16][]byte
}

func DecodeLink(b []byte) (LinkRecord, error) {
	var r LinkRecord
	if len(b) < 16 {
		return r, errors.New("short ifinfomsg")
	}
	r.Hardware = binary.NativeEndian.Uint16(b[2:])
	r.Index = int(int32(binary.NativeEndian.Uint32(b[4:])))
	r.Flags = binary.NativeEndian.Uint32(b[8:])
	if r.Index <= 0 {
		return r, errors.New("invalid ifindex")
	}
	a, e := decodeAttributesWithLinkInfo(b[16:], true)
	if e != nil {
		return r, e
	}
	r.Raw = a
	if r.Name, e = attrText(a, 3, true); e != nil {
		return r, e
	}
	if r.Alias, e = attrText(a, 20, false); e != nil {
		return r, e
	}
	r.MTU, e = attrUint(a, 4, false)
	return r, e
}
func DecodeAddress(b []byte) (AddressRecord, error) {
	var r AddressRecord
	if len(b) < 8 {
		return r, errors.New("short ifaddrmsg")
	}
	r.Family = b[0]
	r.Prefix = b[1]
	r.Flags = b[2]
	r.Scope = b[3]
	r.Index = int(binary.NativeEndian.Uint32(b[4:]))
	if r.Index <= 0 || r.Family != 2 && r.Family != 10 || r.Family == 2 && r.Prefix > 32 || r.Family == 10 && r.Prefix > 128 {
		return r, errors.New("invalid address header")
	}
	a, e := decodeAttributes(b[8:])
	if e != nil {
		return r, e
	}
	r.Raw = a
	raw, ok := a[1]
	if !ok {
		return r, errors.New("address missing")
	}
	if r.Address, e = addressBytes(r.Family, raw); e != nil {
		return r, e
	}
	r.Local = r.Address
	if raw, ok = a[2]; ok {
		r.Local, e = addressBytes(r.Family, raw)
	}
	return r, e
}
func DecodeRoute(b []byte) (RouteRecord, error) {
	var r RouteRecord
	if len(b) < 12 {
		return r, errors.New("short rtmsg")
	}
	r.Family = b[0]
	r.DestBits = b[1]
	r.SourceBits = b[2]
	r.TOS = b[3]
	r.Table = b[4]
	r.Protocol = b[5]
	r.Scope = b[6]
	r.Type = b[7]
	r.Flags = binary.NativeEndian.Uint32(b[8:])
	if r.Family != 2 && r.Family != 10 || r.Family == 2 && (r.DestBits > 32 || r.SourceBits > 32) || r.Family == 10 && (r.DestBits > 128 || r.SourceBits > 128) {
		return r, errors.New("invalid route header")
	}
	a, e := decodeAttributes(b[12:])
	if e != nil {
		return r, e
	}
	r.Raw = a
	r.TableID = uint32(r.Table)
	for k, dst := range map[uint16]*netip.Addr{1: &r.Dst, 2: &r.Source, 5: &r.Gateway, 7: &r.PreferredSource} {
		if v, ok := a[k]; ok {
			*dst, e = addressBytes(r.Family, v)
			if e != nil {
				return r, e
			}
		}
	}
	for k, dst := range map[uint16]*uint32{3: &r.IIF, 4: &r.OIF, 6: &r.Priority, 15: &r.TableID} {
		if v, ok := a[k]; ok {
			if len(v) != 4 {
				return r, errors.New("invalid route scalar")
			}
			*dst = binary.NativeEndian.Uint32(v)
		}
	}
	if !r.Dst.IsValid() {
		if r.DestBits != 0 {
			return r, errors.New("nondefault route missing dst")
		}
		if r.Family == 2 {
			r.Dst = netip.IPv4Unspecified()
		} else {
			r.Dst = netip.IPv6Unspecified()
		}
	}
	return r, nil
}

// Linux RTA_PRIORITY is both iproute2 metric and route priority, not two
// independent attributes. Require the model aliases agree before serializing.
func EncodeRoute(r Route) ([]byte, error) {
	if !r.Destination.IsValid() || !r.Destination.Addr().Is4() || r.Destination.Bits() != 32 || r.LinkIndex <= 0 || r.Protocol != 242 || r.Table != 254 || r.Scope != "link" || r.Type != "unicast" || r.Metric != r.Priority {
		return nil, errors.New("unsafe exact route")
	}
	b := make([]byte, 12)
	b[0] = 2
	b[1] = 32
	b[4] = 254
	b[5] = 242
	b[6] = 253
	b[7] = 1
	b = append(b, attr(1, ipBytes(r.Destination.Addr()))...)
	b = append(b, attr(4, u32(uint32(r.LinkIndex)))...)
	b = append(b, attr(6, u32(r.Priority))...)
	return b, nil
}
func EncodeAddress(index int, ip netip.Addr) ([]byte, error) {
	if index <= 0 || !ip.Is4() || !netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
		return nil, errors.New("unsafe exact address")
	}
	b := make([]byte, 8)
	b[0] = 2
	b[1] = 32
	binary.NativeEndian.PutUint32(b[4:], uint32(index))
	b = append(b, attr(1, ipBytes(ip))...)
	b = append(b, attr(2, ipBytes(ip))...)
	b = append(b, attr(8, u32(0x200))...)
	return b, nil
} // IFA_F_NOPREFIXROUTE
func EncodeLink(index int, name, alias string, mtu uint32, up bool) ([]byte, error) {
	if index <= 0 || len(name) < 7 || len(name) > 15 || len(alias) != 36 || mtu != 1280 {
		return nil, errors.New("unsafe link update")
	}
	b := make([]byte, 16)
	binary.NativeEndian.PutUint32(b[4:], uint32(index))
	if up {
		binary.NativeEndian.PutUint32(b[8:], 1)
	}
	binary.NativeEndian.PutUint32(b[12:], 1)
	b = append(b, attr(3, append([]byte(name), 0))...)
	b = append(b, attr(20, append([]byte(alias), 0))...)
	b = append(b, attr(4, u32(mtu))...)
	return b, nil
}

// Dummy creation exists only for proposed no-TUN conformance. It carries an
// explicit kind, never a generic arbitrary link executor.
func EncodeDummy(name, alias string) ([]byte, error) {
	if len(name) < 7 || len(name) > 15 || len(alias) != 36 {
		return nil, errors.New("invalid dedicated dummy identity")
	}
	b := make([]byte, 16)
	b = append(b, attr(3, append([]byte(name), 0))...)
	b = append(b, attr(20, append([]byte(alias), 0))...)
	nested := attr(1, []byte{'d', 'u', 'm', 'm', 'y', 0})
	return append(b, attr(18|0x8000, nested)...), nil
}
func (c *NetlinkClient) createDummy(ctx context.Context, payload []byte) error {
	if len(payload) < 16 {
		return errors.New("short dummy creation")
	}
	a, e := decodeAttributesWithLinkInfo(payload[16:], true)
	if e != nil {
		return e
	}
	nested, e := decodeAttributes(a[18])
	if e != nil {
		return e
	}
	kind, e := attrText(nested, 1, true)
	if e != nil || kind != "dummy" {
		return errors.New("only explicit dummy kind permitted")
	}
	_, e = c.request(ctx, 16, payload, false, 1|4|0x200|0x400, true)
	return e
}
