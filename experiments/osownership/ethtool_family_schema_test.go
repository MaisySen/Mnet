//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"testing"
)

func familyReplyTest(seq, pid uint32) Datagram {
	id := make([]byte, 2)
	binary.NativeEndian.PutUint16(id, 37)
	attrs := append(attr(1, id), attr(2, []byte("ethtool\x00"))...)
	attrs = append(attrs, attr(3, u32(1))...)
	attrs = append(attrs, attr(4, u32(0))...)
	attrs = append(attrs, attr(5, u32(0))...)
	attrs = append(attrs, attr(6, attr(1, append(attr(1, u32(11)), attr(2, u32(2))...)))...)
	attrs = append(attrs, attr(7, attr(1, append(attr(1, []byte("monitor\x00")), attr(2, u32(9))...)))...)
	b := make([]byte, 20+len(attrs))
	binary.NativeEndian.PutUint32(b, uint32(len(b)))
	binary.NativeEndian.PutUint16(b[4:], 16)
	binary.NativeEndian.PutUint32(b[8:], seq)
	binary.NativeEndian.PutUint32(b[12:], pid)
	b[16] = 1
	b[17] = 2
	copy(b[20:], attrs)
	return Datagram{Bytes: b}
}
func TestEthtoolFamilyResolutionExactObservedNameAndPort(t *testing.T) {
	request, e := buildEthtoolFamilyRequest(1, 77)
	if e != nil || request[16] != 3 || request[17] != 2 || binary.NativeEndian.Uint16(request[4:]) != 16 {
		t.Fatal(e)
	}
	id, e := parseEthtoolFamilyReply(familyReplyTest(1, 77), 1, 77)
	if e != nil || id != 37 {
		t.Fatal("not dynamic", id, e)
	}
	for _, mode := range []string{"sender", "truncated", "sequence", "recipient", "family", "version", "command", "name", "id", "header", "ethtool-version", "duplicate", "flags", "ops-shape"} {
		d := familyReplyTest(1, 77)
		switch mode {
		case "sender":
			d.SenderPID = 1
		case "truncated":
			d.Truncated = true
		case "sequence":
			d.Bytes[8]++
		case "recipient":
			d.Bytes[12]++
		case "family":
			d.Bytes[4]++
		case "version":
			d.Bytes[17] = 1
		case "command":
			d.Bytes[16] = 3
		case "flags":
			d.Bytes[6] = 2
		default:
			attrs, _ := schemaAttrs(d.Bytes[20:], map[uint16]bool{1: false, 2: false, 3: false, 4: false, 5: false, 6: false, 7: false})
			switch mode {
			case "name":
				attrs[2] = []byte("not-ethtool\x00")
			case "id":
				attrs[1] = []byte{1, 0}
			case "header":
				attrs[4] = u32(4)
			case "ethtool-version":
				attrs[3] = u32(2)
			case "ops-shape":
				attrs[6] = attr(1, attr(1, u32(11)))
			}
			var raw []byte
			for _, key := range []uint16{1, 2, 3, 4, 5, 6, 7} {
				raw = append(raw, attr(key, attrs[key])...)
			}
			if mode == "duplicate" {
				raw = append(raw, attr(1, attrs[1])...)
			}
			d.Bytes = append(d.Bytes[:20], raw...)
			binary.NativeEndian.PutUint32(d.Bytes, uint32(len(d.Bytes)))
		}
		if _, e := parseEthtoolFamilyReply(d, 1, 77); e == nil {
			t.Fatal("accepted", mode)
		}
	}
}
