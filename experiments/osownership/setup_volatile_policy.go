//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"reflect"
)

// Exact pinned v6.17 volatile-field comparison. Presence/shape/profile remain
// equal. Original bytes are preserved; only comparison copies mask named counter
// data. NO generic ignore-unknown, flags, routing, conf or feature exception.
func comparableSetupRaw(v RawInventory) (RawInventory, error) {
	out := cloneRawExact(v)
	for n := range out.Links {
		attrs := out.Links[n].Raw
		// rtnl_link_stats:24 u32, rtnl_link_stats64:25 u64 in if_link.h.
		for key, size := range map[uint16]int{7: 96, 23: 200} {
			if b, ok := attrs[key]; ok {
				if len(b) != size {
					return RawInventory{}, afShape
				}
				attrs[key] = make([]byte, size)
			}
		}
		af, ok := attrs[26]
		if !ok {
			continue
		}
		families, e := decodeAttributes(af)
		if e != nil {
			return RawInventory{}, e
		}
		body, ok := families[10]
		if !ok {
			continue
		}
		a6, e := decodeAttributes(body)
		if e != nil {
			return RawInventory{}, e
		}
		for key, size := range map[uint16]int{3: 304, 6: 56} {
			if b, ok := a6[key]; ok {
				if len(b) != size || binary.NativeEndian.Uint64(b) != uint64(size/8) {
					return RawInventory{}, afShape
				}
				masked := make([]byte, size)
				copy(masked[:8], b[:8])
				a6[key] = masked
			}
		}
		// CACHEINFO5 tstamp is device metadata update time, not authority freshness.
		// Preserve max_reasm_len/reachable_time/retrans_time exact; only offset4..7
		// may vary. Refuse unknown layout instead of masking whole cache.
		if b, ok := a6[5]; ok {
			if len(b) != 16 {
				return RawInventory{}, afShape
			}
			for i := 4; i < 8; i++ {
				b[i] = 0
			}
		}
		families[10], e = replaceAttrValues(body, a6)
		if e != nil {
			return RawInventory{}, e
		}
		attrs[26], e = replaceAttrValues(af, families)
		if e != nil {
			return RawInventory{}, e
		}
	}
	return out, nil
}
func replaceAttrValues(raw []byte, values map[uint16][]byte) ([]byte, error) {
	out := append([]byte(nil), raw...)
	for pos := 0; pos < len(out); {
		if len(out)-pos < 4 {
			return nil, afShape
		}
		n := int(binary.NativeEndian.Uint16(out[pos:]))
		key := binary.NativeEndian.Uint16(out[pos+2:])
		value, ok := values[key]
		if !ok || n < 4 || n > len(out)-pos || len(value) != n-4 {
			return nil, afShape
		}
		copy(out[pos+4:pos+n], value)
		pos += (n + 3) &^ 3
	}
	return out, nil
}
func exactSetupPostMatches(actual, expected RawInventory) bool {
	a, e := comparableSetupRaw(actual)
	if e != nil {
		return false
	}
	b, e := comparableSetupRaw(expected)
	return e == nil && reflect.DeepEqual(a, b)
}
