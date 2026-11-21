//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"net/netip"
	"reflect"
	"testing"
)

func packetRig(t *testing.T) *endpointPacketPolicy {
	t.Helper()
	p, e := newEndpointPacketPolicy(packetFramingReadback{Own: netip.MustParseAddr("100.64.0.1"), Peer: netip.MustParseAddr("100.64.0.2"), LocalMTU: 1280, PeerMTU: 1280, TUN: true, NoPI: true, NoVNET: true, NoOffload: true})
	if e != nil {
		t.Fatal(e)
	}
	return p
}

// Independent straightforward one's-complement builder for test vectors.
func testChecksum(b []byte) uint16 {
	sum := 0
	for i := 0; i < len(b); i += 2 {
		sum += int(b[i]) << 8
		if i+1 < len(b) {
			sum += int(b[i+1])
		}
	}
	for sum > 65535 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return uint16(65535 - sum)
}
func testIP(proto byte, payload []byte, reverse bool, frag uint16) []byte {
	b := make([]byte, 20+len(payload))
	b[0] = 0x45
	b[8] = 64
	b[9] = proto
	binary.BigEndian.PutUint16(b[2:], uint16(len(b)))
	binary.BigEndian.PutUint16(b[6:], frag)
	copy(b[12:16], []byte{100, 64, 0, 1})
	copy(b[16:20], []byte{100, 64, 0, 2})
	if reverse {
		copy(b[12:16], []byte{100, 64, 0, 2})
		copy(b[16:20], []byte{100, 64, 0, 1})
	}
	copy(b[20:], payload)
	binary.BigEndian.PutUint16(b[10:], testChecksum(b[:20]))
	return b
}
func testUDP(n int) []byte {
	b := make([]byte, n)
	binary.BigEndian.PutUint16(b, 1234)
	binary.BigEndian.PutUint16(b[2:], 4321)
	binary.BigEndian.PutUint16(b[4:], uint16(n))
	return b
}
func TestEndpointAdmissionUDPFragmentsPreserved(t *testing.T) {
	p := packetRig(t)
	udp := testUDP(4008)
	// Receiving kernel, not this parser, owns reassembly. Out of order is allowed.
	offsets := []int{2512, 0, 1256, 3768}
	sizes := []int{1256, 1256, 1256, 240}
	for n, start := range offsets {
		more := start+sizes[n] < len(udp)
		field := uint16(start / 8)
		if more {
			field |= 0x2000
		}
		raw := testIP(17, udp[start:start+sizes[n]], true, field)
		admitted, e := p.admit(raw, packetToKernel)
		if e != nil || !admitted.fragment || !reflect.DeepEqual(admitted.bytes, raw) {
			t.Fatal("fragment changed/refused", start, e)
		}
		raw[20] ^= 1
		if reflect.DeepEqual(admitted.bytes, raw) {
			t.Fatal("admitted packet alias")
		}
	}
}
func TestEndpointAdmissionHeaderAndProtocolRefusals(t *testing.T) {
	p := packetRig(t)
	for _, mode := range []string{"options", "checksum", "length", "wrong-peer", "ipv6", "igmp", "redirect", "bad-fragment-alignment", "df-fragment", "reserved", "fragment-overflow", "ttl", "udp-length"} {
		t.Run(mode, func(t *testing.T) {
			raw := testIP(17, testUDP(16), true, 0)
			switch mode {
			case "options":
				raw[0] = 0x46
			case "checksum":
				raw[10] ^= 1
			case "length":
				raw[3]--
			case "wrong-peer":
				raw[12] = 127
			case "ipv6":
				raw[0] = 0x65
			case "igmp":
				raw[9] = 2
			case "redirect":
				raw = testIP(1, []byte{5, 0, 0, 0, 0, 0, 0, 0}, true, 0)
			case "bad-fragment-alignment":
				raw = testIP(17, make([]byte, 9), true, 0x2000)
			case "df-fragment":
				binary.BigEndian.PutUint16(raw[6:], 0x6000)
			case "reserved":
				binary.BigEndian.PutUint16(raw[6:], 0x8000)
			case "fragment-overflow":
				raw = testIP(17, make([]byte, 16), true, 0x1fff)
			case "ttl":
				raw[8] = 0
			case "udp-length":
				raw[25]++
			}
			if mode != "checksum" {
				raw[10] = 0
				raw[11] = 0
				binary.BigEndian.PutUint16(raw[10:], testChecksum(raw[:20]))
			}
			if _, e := p.admit(raw, packetToKernel); e == nil {
				t.Fatal("invalid admitted", mode)
			}
		})
	}
}
func testICMPError(typ, code byte, quote []byte, mtu uint16) []byte {
	b := make([]byte, 8+len(quote))
	b[0] = typ
	b[1] = code
	binary.BigEndian.PutUint16(b[6:], mtu)
	copy(b[8:], quote)
	binary.BigEndian.PutUint16(b[2:], testChecksum(b))
	return b
}
func TestEndpointAdmissionPMTUAndBoundedErrors(t *testing.T) {
	p := packetRig(t)
	quote := testIP(17, testUDP(128), false, 0x4000)[:28]
	for _, tc := range []struct {
		typ, code byte
		mtu       uint16
	}{{3, 0, 0}, {3, 1, 0}, {3, 2, 0}, {3, 3, 0}, {3, 4, 1200}, {3, 4, 0}, {3, 13, 0}, {11, 0, 0}, {11, 1, 0}, {12, 0, 0}} {
		raw := testIP(1, testICMPError(tc.typ, tc.code, quote, tc.mtu), true, 0)
		if _, e := p.admit(raw, packetToKernel); e != nil {
			t.Fatal("needed ICMP refused", tc, e)
		}
	}
	for _, mode := range []string{"redirect", "router", "quench", "quote-peer", "quote-short", "quote-options", "mtu-small", "mtu-large", "recursive"} {
		t.Run(mode, func(t *testing.T) {
			q := append([]byte(nil), quote...)
			typ, code := byte(3), byte(4)
			mtu := uint16(1200)
			switch mode {
			case "redirect":
				typ = 5
			case "router":
				typ = 9
			case "quench":
				typ = 4
			case "quote-peer":
				q[16]++
			case "quote-short":
				q = q[:27]
			case "quote-options":
				q[0] = 0x46
			case "mtu-small":
				mtu = 67
			case "mtu-large":
				mtu = 1281
			case "recursive":
				q[9] = 1
				q[20] = 3
				q[10] = 0
				q[11] = 0
				binary.BigEndian.PutUint16(q[10:], testChecksum(q[:20]))
			}
			raw := testIP(1, testICMPError(typ, code, q, mtu), true, 0)
			if _, e := p.admit(raw, packetToKernel); e == nil {
				t.Fatal("bad control admitted", mode)
			}
		})
	}
}
func TestEndpointAdmissionBoundedDrainAndFraming(t *testing.T) {
	p := packetRig(t)
	valid := testIP(17, testUDP(8), true, 0)
	bad := testIP(2, make([]byte, 8), true, 0)
	out, c, e := p.drainBatch([][]byte{valid, bad}, packetToKernel)
	if e != nil || len(out) != 1 || c.Seen != 2 || c.Accepted != 1 || c.Dropped != 1 || c.Refusals[packetProtocol] != 1 {
		t.Fatal(c, e)
	}
	if _, _, e = p.drainBatch(make([][]byte, 65), packetToKernel); e == nil {
		t.Fatal("unbounded drain")
	}
	if _, e = newEndpointPacketPolicy(packetFramingReadback{}); e == nil {
		t.Fatal("unproven framing")
	}
}

func TestEndpointAdmissionTransportChecksumResponsibility(t *testing.T) {
	p := packetRig(t)
	tcp := make([]byte, 20)
	tcp[12] = 0x50
	tcp[13] = 2
	raw := testIP(6, tcp, true, 0)
	pseudo := make([]byte, 12+len(tcp))
	copy(pseudo, raw[12:20])
	pseudo[9] = 6
	binary.BigEndian.PutUint16(pseudo[10:], uint16(len(tcp)))
	copy(pseudo[12:], tcp)
	binary.BigEndian.PutUint16(raw[36:], testChecksum(pseudo))
	if _, e := p.admit(raw, packetToKernel); e != nil {
		t.Fatal("valid TCP checksum", e)
	}
	raw[37] ^= 1
	if _, e := p.admit(raw, packetToKernel); e != packetChecksum {
		t.Fatal("bad TCP checksum", e)
	}
	udp := testIP(17, testUDP(16), true, 0)
	if _, e := p.admit(udp, packetToKernel); e != nil {
		t.Fatal("IPv4 zero UDP checksum legal", e)
	}
	binary.BigEndian.PutUint16(udp[26:], 1)
	if _, e := p.admit(udp, packetToKernel); e != packetChecksum {
		t.Fatal("nonzero UDP checksum must validate", e)
	}
}
