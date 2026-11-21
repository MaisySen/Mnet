//go:build mnet_enrollment_development

package osownership

import "encoding/binary"

func buildEthtoolFamilyRequest(sequence, recipient uint32) ([]byte, error) {
	if sequence == 0 || recipient == 0 {
		return nil, afRequest
	}
	attrs := attr(2, []byte("ethtool\x00"))
	b := make([]byte, 20+len(attrs))
	binary.NativeEndian.PutUint32(b, uint32(len(b)))
	binary.NativeEndian.PutUint16(b[4:], 16)
	binary.NativeEndian.PutUint16(b[6:], 1)
	binary.NativeEndian.PutUint32(b[8:], sequence)
	binary.NativeEndian.PutUint32(b[12:], recipient)
	b[16] = 3
	b[17] = 2
	copy(b[20:], attrs)
	return b, nil
}

// ctrl_fill_info emits version2 NEWFAMILY, unflagged attrs/nests. Family name,
// id/version/header size are validated, operations/mcast lists bounded/framed
// but not used as identity. No fixed ethtool family number or fallback assumed.
func parseEthtoolFamilyReply(d Datagram, sequence, recipient uint32) (uint16, error) {
	b := d.Bytes
	if d.SenderPID != 0 || d.Truncated || len(b) < 20 || len(b) > 4096 || binary.NativeEndian.Uint32(b) != uint32(len(b)) || binary.NativeEndian.Uint16(b[4:]) != 16 || binary.NativeEndian.Uint16(b[6:]) != 0 || binary.NativeEndian.Uint32(b[8:]) != sequence || binary.NativeEndian.Uint32(b[12:]) != recipient || b[16] != 1 || b[17] != 2 || b[18] != 0 || b[19] != 0 {
		return 0, afRequest
	}
	a, e := schemaAttrs(b[20:], map[uint16]bool{1: false, 2: false, 3: false, 4: false, 5: false, 6: false, 7: false})
	if e != nil {
		return 0, e
	}
	if len(a[1]) != 2 {
		return 0, afShape
	}
	id := binary.NativeEndian.Uint16(a[1])
	name, e := attrText(a, 2, true)
	if e != nil || name != "ethtool" || id < 16 {
		return 0, afRequest
	}
	version, e := attrUint(a, 3, true)
	if e != nil || version != 1 {
		return 0, afShape
	}
	header, e := attrUint(a, 4, true)
	if e != nil || header != 0 {
		return 0, afShape
	}
	if _, e = attrUint(a, 5, true); e != nil {
		return 0, e
	}
	for _, key := range []uint16{6, 7} {
		if raw, ok := a[key]; ok {
			nests, e := decodeAttributes(raw)
			if e != nil || len(nests) > 256 {
				return 0, afShape
			}
			for _, nest := range nests {
				fields, e := decodeAttributes(nest)
				if e != nil || len(fields) != 2 {
					return 0, afShape
				}
				if key == 6 {
					if len(fields[1]) != 4 || len(fields[2]) != 4 {
						return 0, afShape
					}
				} else {
					if _, e := attrText(fields, 1, true); e != nil || len(fields[2]) != 4 {
						return 0, afShape
					}
				}
			}
		}
	}
	return id, nil
}
