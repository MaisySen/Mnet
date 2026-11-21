//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
	"reflect"
)

// Verified runtime stringset, not inferred from version1 or size64. Still only
// a parsed fake-wire response until real family/socket provenance is supplied.
type verifiedFeatureNames struct{ values [64]string }

func buildFeatureNamesRequest(r ethtoolFeatureRequest) ([]byte, error) {
	out, e := buildEthtoolFeatureRequest(r)
	if e != nil {
		return nil, e
	}
	header := attr(0x8001, attr(1, u32(r.ifindex)))
	sets := attr(0x8002, attr(0x8001, attr(1, u32(4)))) // ETH_SS_FEATURES=4
	out = out[:20]
	out[16] = 1
	out = append(append(out, header...), sets...)
	binary.NativeEndian.PutUint32(out, uint32(len(out)))
	return out, nil
}
func repeatedSchema(raw []byte, typ uint16) ([][]byte, error) {
	var out [][]byte
	if len(raw) > 4096 {
		return nil, afShape
	}
	for len(raw) > 0 {
		if len(raw) < 4 || len(out) >= 64 {
			return nil, afShape
		}
		n := int(binary.NativeEndian.Uint16(raw))
		kind := binary.NativeEndian.Uint16(raw[2:])
		if kind != typ || n < 4 || n > len(raw) {
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
		out = append(out, append([]byte(nil), raw[4:n]...))
		raw = raw[aligned:]
	}
	return out, nil
}
func parseFeatureNames(d Datagram, r ethtoolFeatureRequest) (*verifiedFeatureNames, error) {
	raw := d.Bytes
	if d.SenderPID != 0 || d.Truncated || r.family < 16 || r.sequence == 0 || r.recipient == 0 || r.profile != pinnedFeaturesV617 || len(raw) < 20 || len(raw) > 4096 || int(binary.NativeEndian.Uint32(raw)) != len(raw) || binary.NativeEndian.Uint16(raw[4:]) != r.family || binary.NativeEndian.Uint16(raw[6:]) != 0 || binary.NativeEndian.Uint32(raw[8:]) != r.sequence || binary.NativeEndian.Uint32(raw[12:]) != r.recipient || raw[16] != 1 || raw[17] != 1 || raw[18] != 0 || raw[19] != 0 {
		return nil, afRequest
	}
	top, e := schemaAttrs(raw[20:], map[uint16]bool{1: true, 2: true})
	if e != nil || len(top) != 2 {
		return nil, afShape
	}
	h, e := schemaAttrs(top[1], map[uint16]bool{1: false, 2: false})
	if e != nil || len(h) != 2 {
		return nil, afShape
	}
	index, e := attrUint(h, 1, true)
	if e != nil || index != r.ifindex {
		return nil, afRequest
	}
	name, e := attrText(h, 2, true)
	if e != nil || name != r.name {
		return nil, afRequest
	}
	sets, e := repeatedSchema(top[2], 0x8001)
	if e != nil || len(sets) != 1 {
		return nil, afShape
	}
	set, e := schemaAttrs(sets[0], map[uint16]bool{1: false, 2: false, 3: true})
	if e != nil || len(set) != 3 {
		return nil, afShape
	}
	id, e := attrUint(set, 1, true)
	if e != nil || id != 4 {
		return nil, afShape
	}
	count, e := attrUint(set, 2, true)
	if e != nil || count != 64 {
		return nil, afShape
	}
	strings, e := repeatedSchema(set[3], 0x8001)
	if e != nil || len(strings) != 64 {
		return nil, afShape
	}
	names := &verifiedFeatureNames{}
	seen := [64]bool{}
	for _, entry := range strings {
		attrs, e := schemaAttrs(entry, map[uint16]bool{1: false, 2: false})
		if e != nil || len(attrs) != 2 {
			return nil, afShape
		}
		n, e := attrUint(attrs, 1, true)
		if e != nil || n >= 64 || seen[n] {
			return nil, afShape
		}
		value := attrs[2]
		if len(value) < 1 || len(value) > 33 || value[len(value)-1] != 0 || bytes.IndexByte(value[:len(value)-1], 0) >= 0 {
			return nil, afShape
		}
		names.values[n] = string(value[:len(value)-1])
		seen[n] = true
	}
	if !reflect.DeepEqual(names.values, pinnedFeatureNamesV617) {
		return nil, afShape
	}
	return names, nil
}
func (f ethtoolActiveFeatures) observedNamesCompatible(names *verifiedFeatureNames) bool {
	if names == nil || !reflect.DeepEqual(names.values, pinnedFeatureNamesV617) {
		return false
	}
	// Precisely fresh TUN's allowed non-user-offload active feature names. This is
	// not blanket bit-zero and doesn't mistake generic software GSO/GRO for TSO.
	allowed := map[string]bool{"tx-scatter-gather": true, "highdma": true, "tx-scatter-gather-fraglist": true, "tx-vlan-hw-insert": true, "tx-vlan-stag-hw-insert": true, "tx-generic-segmentation": true, "rx-gro": true}
	for n, name := range names.values {
		if f.active&(uint64(1)<<n) != 0 && !allowed[name] {
			return false
		}
	}
	return true
}
