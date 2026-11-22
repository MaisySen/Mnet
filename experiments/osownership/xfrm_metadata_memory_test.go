//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"testing"
)

// DATA only. No original evidence, namespace, socket, context, timer, syscall,
// source collector or effectful callback is constructed or invoked here.
func TestXfrmMetadataRequestsMemory(t *testing.T) {
	for _, kind := range []uint16{0x23, 0x25, 0x28} {
		b, e := buildXfrmMetadataRequest(kind, 7, 99)
		if e != nil || len(b) != 20 || binary.NativeEndian.Uint32(b) != 20 || binary.NativeEndian.Uint16(b[4:]) != kind || binary.NativeEndian.Uint16(b[6:]) != 1 || binary.NativeEndian.Uint32(b[8:]) != 7 || binary.NativeEndian.Uint32(b[12:]) != 99 || binary.NativeEndian.Uint32(b[16:]) != 0 {
			t.Fatal("GET framing", kind)
		}
	}
	// Exhaustively reject every other 16-bit message kind, including key-bearing
	// GETSA, GETPOLICY, SETDEFAULT and their creation/update/delete counterparts.
	for kind := 0; kind < 65536; kind++ {
		_, e := buildXfrmMetadataRequest(uint16(kind), 7, 99)
		if (kind == 0x23 || kind == 0x25 || kind == 0x28) != (e == nil) {
			t.Fatal("message allowlist", kind)
		}
	}
	for _, ids := range [][2]uint32{{0, 99}, {7, 0}, {0, 0}} {
		if _, e := buildXfrmMetadataRequest(0x23, ids[0], ids[1]); e == nil {
			t.Fatal("missing correlation")
		}
	}
}
func xfrmFrameMemory(kind uint16, body []byte) Datagram {
	b := make([]byte, (16+len(body)+3)&^3)
	binary.NativeEndian.PutUint32(b, uint32(16+len(body)))
	binary.NativeEndian.PutUint16(b[4:], kind)
	binary.NativeEndian.PutUint32(b[8:], 7)
	binary.NativeEndian.PutUint32(b[12:], 99)
	copy(b[16:], body)
	return Datagram{Bytes: b}
}
func TestXfrmMetadataDecodersMemory(t *testing.T) {
	sadBody := append(u32(0), attr(1, u32(0))...)
	sadBody = append(sadBody, attr(2, append(u32(8), u32(1024)...))...)
	spdBody := append(u32(0), attr(1, make([]byte, 24))...)
	spdBody = append(spdBody, attr(2, append(u32(7), u32(1024)...))...)
	spdBody = append(spdBody, attr(3, []byte{32, 32})...)
	spdBody = append(spdBody, attr(4, []byte{128, 128})...)
	sad := xfrmFrameMemory(0x22, sadBody)
	spd := xfrmFrameMemory(0x24, spdBody)
	defaults := xfrmFrameMemory(0x28, []byte{2, 2, 2, 0})
	a, e := parseXfrmSADMetadata(sad, 7, 99)
	if e != nil || a.Hash != [2]uint32{8, 1024} {
		t.Fatal("SAD", e)
	}
	b, e := parseXfrmSPDMetadata(spd, 7, 99)
	if e != nil || b.Threshold6 != [2]byte{128, 128} {
		t.Fatal("SPD", e)
	}
	c, e := parseXfrmDefaults(defaults, 7, 99)
	if e != nil {
		t.Fatal("defaults", e)
	}
	data := xfrmMetadataData{SAD: a, SPD: b, Defaults: c}
	if !data.emptyAcceptDefaults() || (xfrmMetadataData{}).emptyAcceptDefaults() {
		t.Fatal("empty/unknown predicate")
	}
	data.SAD.Count = 1
	if data.emptyAcceptDefaults() {
		t.Fatal("nonzero SA")
	}
	data.SAD.Count = 0
	for i := range data.SPD.Counts {
		data.SPD.Counts[i] = 1
		if data.emptyAcceptDefaults() {
			t.Fatal("nonzero SPD", i)
		}
		data.SPD.Counts[i] = 0
	}
	for i := range data.Defaults {
		data.Defaults[i] = 1
		if data.emptyAcceptDefaults() {
			t.Fatal("BLOCK default", i)
		}
		data.Defaults[i] = 2
	}
	for _, tc := range []struct {
		name     string
		which    int
		position int
		value    byte
	}{
		{"SAD-kind", 0, 4, 0x12}, {"SPD-kind", 1, 4, 0x15}, {"default-kind", 2, 4, 0x27},
		{"SAD-seq", 0, 8, 8}, {"SPD-port", 1, 12, 98}, {"multi", 0, 6, 2}, {"ACK", 1, 6, 4},
		{"SAD-flags-echo", 0, 16, 1}, {"SAD-unknown-attr", 0, 22, 9}, {"SAD-missing-count", 0, 20, 7},
		{"SPD-unknown-attr", 1, 22, 9}, {"SPD-threshold-v4", 1, 64, 33}, {"SPD-threshold-v6", 1, 72, 129},
		{"default-unknown", 2, 16, 0}, {"default-unknown2", 2, 18, 3}, {"default-padding", 2, 19, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := []Datagram{sad, spd, defaults}[tc.which]
			d.Bytes = append([]byte(nil), d.Bytes...)
			d.Bytes[tc.position] = tc.value
			var e error
			switch tc.which {
			case 0:
				_, e = parseXfrmSADMetadata(d, 7, 99)
			case 1:
				_, e = parseXfrmSPDMetadata(d, 7, 99)
			case 2:
				_, e = parseXfrmDefaults(d, 7, 99)
			}
			if e == nil {
				t.Fatal("malformed accepted")
			}
		})
	}
	for _, tc := range []struct {
		name string
		d    Datagram
	}{
		{"short", Datagram{Bytes: []byte{0}}}, {"sender", Datagram{Bytes: defaults.Bytes, SenderPID: 4}}, {"truncated", Datagram{Bytes: defaults.Bytes, Truncated: true}},
		{"trailer", Datagram{Bytes: append(append([]byte(nil), defaults.Bytes...), 0, 0, 0, 0)}},
		{"default-unfinalized", xfrmFrameMemory(0x28, []byte{2, 2, 2})},
		{"SAD-as-default", sad}, {"SPD-as-default", spd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := parseXfrmDefaults(tc.d, 7, 99); e == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
	duplicate := append(append([]byte(nil), sadBody...), attr(1, u32(0))...)
	if _, e := parseXfrmSADMetadata(xfrmFrameMemory(0x22, duplicate), 7, 99); e == nil {
		t.Fatal("duplicate attribute")
	}
	// A successful DATA predicate above cannot mint the private original witness.
}
