//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

// Narrow native-endian rtnetlink attribute codec. Unknown attrs are preserved
// for audit but never interpreted as a known attribute. Duplicate singleton
// attrs, nested/net-byteorder flags and nonzero padding fail closed.
func decodeAttributes(b []byte) (map[uint16][]byte, error) {
	return decodeAttributesWithLinkInfo(b, false)
}
func decodeAttributesWithLinkInfo(b []byte, allowLinkInfo bool) (map[uint16][]byte, error) {
	out := map[uint16][]byte{}
	for len(b) > 0 {
		if len(b) < 4 {
			return nil, errors.New("short rtattr")
		}
		n := int(binary.NativeEndian.Uint16(b))
		typ := binary.NativeEndian.Uint16(b[2:])
		if n < 4 || n > len(b) || (typ&0xc000 != 0 && !(allowLinkInfo && ((typ == (62|0x8000) || typ == (65|0x8000)) && n == 4))) {
			return nil, fmt.Errorf("invalid rtattr length/flags type=%d len=%d", typ, n)
		}
		if allowLinkInfo && typ&0x8000 != 0 {
			typ &= 0x3fff
		}
		if _, ok := out[typ]; ok {
			return nil, errors.New("duplicate rtattr")
		}
		out[typ] = append([]byte(nil), b[4:n]...)
		aligned := (n + 3) &^ 3
		if aligned > len(b) {
			return nil, errors.New("rtattr alignment")
		}
		for _, p := range b[n:aligned] {
			if p != 0 {
				return nil, errors.New("rtattr padding")
			}
		}
		b = b[aligned:]
	}
	return out, nil
}
func attr(kind uint16, p []byte) []byte {
	n := 4 + len(p)
	b := make([]byte, (n+3)&^3)
	binary.NativeEndian.PutUint16(b, uint16(n))
	binary.NativeEndian.PutUint16(b[2:], kind)
	copy(b[4:], p)
	return b
}
func u32(v uint32) []byte { b := make([]byte, 4); binary.NativeEndian.PutUint32(b, v); return b }
func attrUint(a map[uint16][]byte, k uint16, required bool) (uint32, error) {
	b, ok := a[k]
	if !ok && !required {
		return 0, nil
	}
	if len(b) != 4 {
		return 0, errors.New("invalid uint attr")
	}
	return binary.NativeEndian.Uint32(b), nil
}
func attrText(a map[uint16][]byte, k uint16, required bool) (string, error) {
	b, ok := a[k]
	if !ok && !required {
		return "", nil
	}
	if !required && len(b) == 1 && b[0] == 0 {
		return "", nil
	}
	if len(b) < 2 || b[len(b)-1] != 0 || bytes.IndexByte(b[:len(b)-1], 0) >= 0 {
		return "", errors.New("invalid text attr")
	}
	return string(b[:len(b)-1]), nil
}
func ipBytes(a netip.Addr) []byte {
	if a.Is4() {
		v := a.As4()
		return v[:]
	}
	v := a.As16()
	return v[:]
}
func addressBytes(family byte, b []byte) (netip.Addr, error) {
	if family == 2 && len(b) == 4 {
		var a [4]byte
		copy(a[:], b)
		return netip.AddrFrom4(a), nil
	}
	if family == 10 && len(b) == 16 {
		var a [16]byte
		copy(a[:], b)
		return netip.AddrFrom16(a), nil
	}
	return netip.Addr{}, errors.New("invalid address family/length")
}
