//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"reflect"
	"testing"
)

func featureNamesReplyTest(r ethtoolFeatureRequest, names [64]string) Datagram {
	header := append(attr(1, u32(r.ifindex)), attr(2, append([]byte(r.name), 0))...)
	var entries []byte
	for n, name := range names {
		entries = append(entries, attr(0x8001, append(attr(1, u32(uint32(n))), attr(2, append([]byte(name), 0))...))...)
	}
	set := append(append(attr(1, u32(4)), attr(2, u32(64))...), attr(0x8003, entries)...)
	top := append(attr(0x8001, header), attr(0x8002, attr(0x8001, set))...)
	b := make([]byte, 20+len(top))
	binary.NativeEndian.PutUint32(b, uint32(len(b)))
	binary.NativeEndian.PutUint16(b[4:], r.family)
	binary.NativeEndian.PutUint32(b[8:], r.sequence)
	binary.NativeEndian.PutUint32(b[12:], r.recipient)
	b[16] = 1
	b[17] = 1
	copy(b[20:], top)
	return Datagram{Bytes: b}
}
func TestFeatureSTRSETVerifiesObservedNamesAndIndexes(t *testing.T) {
	req := testFeatureReq()
	names, e := parseFeatureNames(featureNamesReplyTest(req, pinnedFeatureNamesV617), req)
	if e != nil {
		t.Fatal(e)
	}
	// Actual fresh source mask: SG/FRAGLIST/VLAN ctag+stag and software GSO/GRO.
	f := ethtoolActiveFeatures{active: 1<<0 | 1<<6 | 1<<7 | 1<<11 | 1<<14 | 1<<45}
	if !f.observedNamesCompatible(names) {
		t.Fatal("ordinary fresh TUN rejected")
	}
	for _, n := range []uint{1, 3, 4, 16, 18, 20, 26, 27, 32, 33, 35, 51} {
		f.active = 1 << n
		if f.observedNamesCompatible(names) {
			t.Fatal("unsupported feature", n)
		}
	}
	bad := pinnedFeatureNamesV617
	bad[3], bad[11] = bad[11], bad[3]
	if _, e = parseFeatureNames(featureNamesReplyTest(req, bad), req); e == nil {
		t.Fatal("same-size swapped meaning")
	}
	bad = pinnedFeatureNamesV617
	bad[2] = "new-checksum-offload"
	if _, e = parseFeatureNames(featureNamesReplyTest(req, bad), req); e == nil {
		t.Fatal("reserved now named")
	}
	if f.observedNamesCompatible(nil) {
		t.Fatal("unverified names")
	}
	request, e := buildFeatureNamesRequest(req)
	if e != nil || request[16] != 1 || binary.NativeEndian.Uint16(request[6:]) != 1 {
		t.Fatal("STRSET read-only request", e)
	}
}
func TestFeatureSTRSETBoundedSchemaRefusals(t *testing.T) {
	for _, mode := range []string{"sender", "truncated", "seq", "family", "device", "version", "command", "oversize", "count", "missing", "duplicate-index", "unterminated", "unknown-name"} {
		t.Run(mode, func(t *testing.T) {
			req := testFeatureReq()
			d := featureNamesReplyTest(req, pinnedFeatureNamesV617)
			switch mode {
			case "sender":
				d.SenderPID = 1
			case "truncated":
				d.Truncated = true
			case "seq":
				req.sequence++
			case "family":
				req.family++
			case "device":
				req.ifindex++
			case "version":
				d.Bytes[17]++
			case "command":
				d.Bytes[16] = 11
			case "oversize":
				d.Bytes = make([]byte, 4097)
			default:
				top, _ := schemaAttrs(d.Bytes[20:], map[uint16]bool{1: true, 2: true})
				sets, _ := repeatedSchema(top[2], 0x8001)
				set, _ := schemaAttrs(sets[0], map[uint16]bool{1: false, 2: false, 3: true})
				entries, _ := repeatedSchema(set[3], 0x8001)
				switch mode {
				case "count":
					set[2] = u32(63)
				case "missing":
					entries = entries[:63]
				case "duplicate-index":
					copy(entries[1][4:8], entries[0][4:8])
				case "unterminated":
					entry, _ := schemaAttrs(entries[0], map[uint16]bool{1: false, 2: false})
					entry[2][len(entry[2])-1] = 'x'
					entries[0] = append(attr(1, entry[1]), attr(2, entry[2])...)
				case "unknown-name":
					entry, _ := schemaAttrs(entries[0], map[uint16]bool{1: false, 2: false})
					entries[0] = append(attr(1, entry[1]), attr(2, []byte("unknown\x00"))...)
				}
				var joined []byte
				for _, entry := range entries {
					joined = append(joined, attr(0x8001, entry)...)
				}
				setBody := append(append(attr(1, set[1]), attr(2, set[2])...), attr(0x8003, joined)...)
				topBody := append(attr(0x8001, top[1]), attr(0x8002, attr(0x8001, setBody))...)
				d.Bytes = append(d.Bytes[:20], topBody...)
				binary.NativeEndian.PutUint32(d.Bytes, uint32(len(d.Bytes)))
			}
			if _, e := parseFeatureNames(d, req); e == nil {
				t.Fatal(mode)
			}
		})
	}
}
func TestSourceReaderDiagnosticsUseSameFramingAndEffectiveOperands(t *testing.T) {
	s, ar, source := effectiveRig(t)
	source.link.Hardware = 65534
	source.link.Flags = 0x1091
	source.link.Raw[18] = nestedTunTest()
	source.sys["/proc/sys/net/ipv4/conf/all/send_redirects"] = "1"
	p, e := ar.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	raw := RawInventory{Links: p.links}
	before := cloneRawExact(raw)
	d, e := sourceDiagnosticFromRecords(raw, p.sys, *s.canonical, false)
	if e != nil || d.lineageUsable || len(d.effective) != 33 || d.effective[5].Value != 1 {
		t.Fatal(d, e)
	}
	if !reflect.DeepEqual(before, raw) {
		t.Fatal("mutated raw input")
	}
	source.link.Raw[18] = nil
	if !reflect.DeepEqual(before, d.raw) {
		t.Fatal("aliased diagnostic")
	}
	bad := *s.canonical
	bad.Link.Index++
	if _, e = sourceDiagnosticFromRecords(raw, p.sys, bad, true); e == nil {
		t.Fatal("wrong ownership index")
	}
}
