//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"slices"
)

// Narrow explicit nested-attribute decoder, separate from generic rtnetlink
// admission. Required flags are part of this schema; unknown types fail closed.
func schemaAttrs(raw []byte, kinds map[uint16]bool) (map[uint16][]byte, error) {
	if len(raw) > 4096 {
		return nil, afShape
	}
	out := map[uint16][]byte{}
	for len(raw) > 0 {
		if len(raw) < 4 {
			return nil, afShape
		}
		n := int(binary.NativeEndian.Uint16(raw))
		typ := binary.NativeEndian.Uint16(raw[2:])
		nested, known := kinds[typ&0x3fff]
		if !known || typ&0x4000 != 0 || (typ&0x8000 != 0) != nested || n < 4 || n > len(raw) {
			return nil, afShape
		}
		key := typ & 0x3fff
		if _, exists := out[key]; exists {
			return nil, afShape
		}
		aligned := (n + 3) &^ 3
		if aligned > len(raw) {
			return nil, afShape
		}
		for _, v := range raw[n:aligned] {
			if v != 0 {
				return nil, afShape
			}
		}
		out[key] = slices.Clone(raw[4:n])
		raw = raw[aligned:]
	}
	return out, nil
}

type tunNestedFraming struct{ typ, pi, vnet, persist, multiqueue byte }

// rtnl_link_fill and rtnl_link_info_fill both use nest_start_noflag.
// DecodeLink checks the outer LINKINFO flags; this checks INFO_DATA flags.
// tun_fill_info, if_link.h: TYPE3/PI4/VNET5/PERSIST6/MULTI_QUEUE7.
// Optional OWNER1/GROUP2 are u32. Queue counts8/9 unsupported for single queue.
func parseTUNNested(link LinkRecord) (tunNestedFraming, error) {
	var zero tunNestedFraming
	info, e := schemaAttrs(link.Raw[18], map[uint16]bool{1: false, 2: false})
	if e != nil || len(info) != 2 {
		return zero, afShape
	}
	kind, e := attrText(info, 1, true)
	if e != nil || kind != "tun" {
		return zero, afShape
	}
	data, e := schemaAttrs(info[2], map[uint16]bool{1: false, 2: false, 3: false, 4: false, 5: false, 6: false, 7: false})
	if e != nil {
		return zero, e
	}
	for _, k := range []uint16{1, 2} {
		if v, ok := data[k]; ok && len(v) != 4 {
			return zero, afShape
		}
	}
	for _, k := range []uint16{3, 4, 5, 6, 7} {
		if len(data[k]) != 1 {
			return zero, afShape
		}
	}
	f := tunNestedFraming{data[3][0], data[4][0], data[5][0], data[6][0], data[7][0]}
	if f != (tunNestedFraming{typ: 1}) {
		return zero, packetFraming
	}
	return f, nil
}

// Compact v6.17 ETHTOOL_MSG_FEATURES_GET reply schema. Family ID is dynamically
// resolved, not a fixed constant. The source profile must be established outside
// this parser; family version1 alone does not identify kernel feature numbering.
type ethtoolFeatureRequest struct {
	family              uint16
	sequence, recipient uint32
	ifindex             uint32
	name                string
	profile             string
}
type ethtoolActiveFeatures struct{ hardware, wanted, active, nochange uint64 }

const pinnedFeaturesV617 = "linux-v6.17-netdev-features64"

func compactFeatureBitmap(raw []byte, masked bool) (uint64, error) {
	fields, e := schemaAttrs(raw, map[uint16]bool{1: false, 2: false, 4: false, 5: false})
	if e != nil {
		return 0, e
	}
	size, e := attrUint(fields, 2, true)
	if e != nil || size != 64 || len(fields[4]) != 8 {
		return 0, afShape
	}
	if masked {
		if len(fields) != 3 || len(fields[5]) != 8 || binary.NativeEndian.Uint64(fields[5]) != ^uint64(0) {
			return 0, afShape
		}
	} else {
		flag, ok := fields[1]
		if len(fields) != 3 || !ok || len(flag) != 0 {
			return 0, afShape
		}
	}
	// Wire is array of native u32, NOT native u64 on big endian machines.
	value := uint64(binary.NativeEndian.Uint32(fields[4])) | uint64(binary.NativeEndian.Uint32(fields[4][4:]))<<32
	return value, nil
}
func parseEthtoolActive(d Datagram, r ethtoolFeatureRequest) (ethtoolActiveFeatures, error) {
	var out ethtoolActiveFeatures
	raw := d.Bytes
	if d.SenderPID != 0 || d.Truncated || r.profile != pinnedFeaturesV617 || r.family < 16 || r.sequence == 0 || r.recipient == 0 || r.ifindex == 0 || !safeInterfaceName(r.name) || len(raw) < 20 || len(raw) > 4096 {
		return out, afRequest
	}
	if int(binary.NativeEndian.Uint32(raw)) != len(raw) || binary.NativeEndian.Uint16(raw[4:]) != r.family || binary.NativeEndian.Uint16(raw[6:]) != 0 || binary.NativeEndian.Uint32(raw[8:]) != r.sequence || binary.NativeEndian.Uint32(raw[12:]) != r.recipient || raw[16] != 11 || raw[17] != 1 || raw[18] != 0 || raw[19] != 0 {
		return out, afRequest
	}
	attrs, e := schemaAttrs(raw[20:], map[uint16]bool{1: true, 2: true, 3: true, 4: true, 5: true})
	if e != nil || len(attrs) != 5 {
		return out, afShape
	}
	header, e := schemaAttrs(attrs[1], map[uint16]bool{1: false, 2: false})
	if e != nil || len(header) != 2 {
		return out, afShape
	}
	index, e := attrUint(header, 1, true)
	if e != nil || index != r.ifindex {
		return out, afRequest
	}
	name, e := attrText(header, 2, true)
	if e != nil || name != r.name {
		return out, afRequest
	}
	values := [4]uint64{}
	for i := range values {
		values[i], e = compactFeatureBitmap(attrs[uint16(i+2)], i == 0)
		if e != nil {
			return out, e
		}
	}
	out = ethtoolActiveFeatures{values[0], values[1], values[2], values[3]}
	// v6.17 reserved feature positions2,12,13 have no names/meaning.
	const reserved = uint64(1<<2 | 1<<12 | 1<<13)
	if (out.active|out.hardware|out.wanted|out.nochange)&reserved != 0 {
		return ethtoolActiveFeatures{}, afShape
	}
	return out, nil
}

// Source-pinned classifier, NOT proof this source profile runs on a host.
// checksum:IP1/HW3/IPv6 4; GSO protocol feature positions16..35 (incl SCTP,
// encapsulation and newer ACCECN). Generic GSO11/GRO14/SG0 need not be zero.
func (f ethtoolActiveFeatures) rawOutputCompatible() bool {
	const checksum = uint64(1<<1 | 1<<3 | 1<<4)
	const protocolGSO = (uint64(1) << 36) - (uint64(1) << 16)
	return f.active&(checksum|protocolGSO) == 0
}

// Read-only compact FEATURES_GET command11/version1, indexed device request.
// Family must come from a separately reviewed dynamic resolver, not this helper.
func buildEthtoolFeatureRequest(r ethtoolFeatureRequest) ([]byte, error) {
	if r.family < 16 || r.sequence == 0 || r.recipient == 0 || r.ifindex == 0 || !safeInterfaceName(r.name) || r.profile != pinnedFeaturesV617 {
		return nil, afRequest
	}
	header := append(attr(1, u32(r.ifindex)), attr(3, u32(1))...)
	attrs := attr(0x8001, header)
	out := make([]byte, 20+len(attrs))
	binary.NativeEndian.PutUint32(out, uint32(len(out)))
	binary.NativeEndian.PutUint16(out[4:], r.family)
	binary.NativeEndian.PutUint16(out[6:], 1)
	binary.NativeEndian.PutUint32(out[8:], r.sequence)
	binary.NativeEndian.PutUint32(out[12:], r.recipient)
	out[16] = 11
	out[17] = 1
	copy(out[20:], attrs)
	return out, nil
}
