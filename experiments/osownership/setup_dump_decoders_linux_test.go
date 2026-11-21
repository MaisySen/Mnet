//go:build linux && mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// Actual production decoder methods, no Request/socket/namespace execution.
func TestActualSetupDumpDecoderSelectionOwnedForeignAndNames(t *testing.T) {
	_, f := fullSetupRunRig(t, "0")
	target := fullSetupBodyTest(*f.owned(), "")
	creation := creationSetupDump{name: f.h.Link.Name}
	owned := ownedSetupDump{handle: f.h}
	for _, decoder := range []setupLinkDumpDecoder{creation, owned} {
		r, e := decoder.decodeOwnedSetupLink(target)
		if e != nil || r.setupOuter == nil {
			t.Fatal("owned full body", e)
		}
		if e = validateSetupOuter(r); e != nil {
			t.Fatal(e)
		}
		for _, hardware := range []uint16{65534, 772} {
			body := make([]byte, 16)
			binary.NativeEndian.PutUint16(body[2:], hardware)
			binary.NativeEndian.PutUint32(body[4:], 999)
			body = append(body, attr(3, []byte("foreign0\x00"))...)
			body = append(body, attr(4, u32(1500))...)
			want, e := DecodeLink(body)
			if e != nil {
				t.Fatal(e)
			}
			got, e := decoder.decodeOwnedSetupLink(body)
			if e != nil || !reflect.DeepEqual(got, want) || got.setupOuter != nil {
				t.Fatal("foreign decoder widened or owned-selected", hardware, e)
			}
			// Foreign malformed/duplicate singleton and nested flag remain strict.
			for _, bad := range [][]byte{append(append([]byte(nil), body...), attr(4, u32(1500))...), append(append([]byte(nil), body...), attr(18|0x8000, []byte{})...)} {
				if _, e := decoder.decodeOwnedSetupLink(bad); e == nil {
					t.Fatal("foreign strictness lost")
				}
			}
		}
		for _, mode := range []string{"duplicate-name", "flagged-name", "bad-name", "missing-name", "short"} {
			body := append([]byte(nil), target...)
			switch mode {
			case "duplicate-name":
				body = append(body, attr(3, []byte(f.h.Link.Name+"\x00"))...)
			case "flagged-name", "bad-name", "missing-name":
				for pos := 16; pos < len(body); {
					n := int(binary.NativeEndian.Uint16(body[pos:]))
					typ := binary.NativeEndian.Uint16(body[pos+2:])
					if typ == 3 {
						switch mode {
						case "flagged-name":
							binary.NativeEndian.PutUint16(body[pos+2:], 3|0x8000)
						case "bad-name":
							body[pos+n-1] = 1
						case "missing-name":
							body = append(body[:pos], body[pos+((n+3)&^3):]...)
						}
						break
					}
					pos += (n + 3) &^ 3
				}
			case "short":
				body = body[:17]
			}
			if _, e := decoder.decodeOwnedSetupLink(body); e == nil {
				t.Fatal("invalid name selected", mode)
			}
		}
	}
	// Name, not NONE hardware, chooses the owned creation profile. A claimed
	// target with wrong hardware must not slip down the generic baseline path.
	bad := append([]byte(nil), target...)
	binary.NativeEndian.PutUint16(bad[2:], 772)
	if _, e := creation.decodeOwnedSetupLink(bad); e == nil {
		t.Fatal("target wrong hardware bypass")
	}
	// An owned ifindex with a foreign name is never reclassified as foreign.
	body := make([]byte, 16)
	binary.NativeEndian.PutUint32(body[4:], uint32(f.h.Link.Index))
	body = append(body, attr(3, []byte("foreign0\x00"))...)
	if _, e := owned.decodeOwnedSetupLink(body); e == nil {
		t.Fatal("owned index foreign name")
	}
}

func TestActualSetupDecodersDumpPADForeignFlagsAndOwnedTwoSites(t *testing.T) {
	_, f := fullSetupRunRig(t, "0")
	creation := creationSetupDump{name: f.h.Link.Name}
	owned := ownedSetupDump{handle: f.h}
	for _, hardware := range []uint16{65534, 772} {
		body := make([]byte, 16)
		binary.NativeEndian.PutUint16(body[2:], hardware)
		binary.NativeEndian.PutUint32(body[4:], 999)
		binary.NativeEndian.PutUint32(body[8:], 0xabcdef01)
		body = append(body, attr(3, []byte("foreign0\x00"))...)
		body = append(body, attr(4, u32(1500))...)
		body = append(body, attr(42, nil)...)
		body = append(body, attr(23, make([]byte, 200))...)
		body = append(body, attr(42, nil)...)
		body = append(body, attr(14, make([]byte, 32))...)
		want, e := DecodeLink(body)
		if e != nil || want.setupOuter == nil {
			t.Fatal("foreign generic PAD", e)
		}
		for _, decoder := range []setupLinkDumpDecoder{creation, owned} {
			got, e := decoder.decodeOwnedSetupLink(body)
			if e != nil || !reflect.DeepEqual(got, want) || got.Flags != 0xabcdef01 {
				t.Fatal("foreign got owned flags/profile", hardware, e)
			}
			if !reflect.DeepEqual(got.setupOuter.body, body) {
				t.Fatal("foreign wire evidence lost")
			}
			for _, bad := range [][]byte{append(append([]byte(nil), body...), attr(3, []byte("foreign0\x00"))...), append(append([]byte(nil), body...), attr(3|0x8000, []byte("foreign0\x00"))...), append(append([]byte(nil), body...), attr(42, nil)...), append(append([]byte(nil), body...), attr(42|0x8000, nil)...)} {
				if _, e := decoder.decodeOwnedSetupLink(bad); e == nil {
					t.Fatal("foreign name/PAD strictness lost")
				}
			}
		}
	}
	// Test the two actual production decoder types with two supported PAD sites
	// on the owned body. These positions are policy, not a kernel alignment oracle.
	body := fullSetupBodyTest(*f.owned(), "")
	named, _, e := dumpAttrsWithoutPAD(body[16:])
	if e != nil {
		t.Fatal(e)
	}
	two := append([]byte(nil), body[:16]...)
	for raw := named; len(raw) > 0; {
		n := int(binary.NativeEndian.Uint16(raw))
		aligned := (n + 3) &^ 3
		typ := binary.NativeEndian.Uint16(raw[2:])
		if typ == 14 || typ == 23 {
			two = append(two, attr(42, nil)...)
		}
		two = append(two, raw[:aligned]...)
		raw = raw[aligned:]
	}
	for _, decoder := range []setupLinkDumpDecoder{creation, owned} {
		r, e := decoder.decodeOwnedSetupLink(two)
		if e != nil || r.setupOuter == nil {
			t.Fatal("owned twoPAD", e)
		}
		if e = validateSetupOuter(r); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := decodeAttributes(two[16:]); e == nil {
		t.Fatal("nested singleton decoder widened")
	}
	// Name routing cannot authorize ownership: a different-index same-name is
	// parsed only as a candidate. Original custody/index verification is later.
	collision := append([]byte(nil), two...)
	binary.NativeEndian.PutUint32(collision[4:], 999)
	got, e := creation.decodeOwnedSetupLink(collision)
	if e != nil || got.Name != f.h.Link.Name {
		t.Fatal("name routing unexpected", e)
	}
	// Creation decoder deliberately parses a candidate only. The later exclusive
	// create handle/index/device verifier grants custody, not this name scan.
	if got.Index == f.h.Link.Index {
		t.Fatal("parser adopted original index")
	}
}
