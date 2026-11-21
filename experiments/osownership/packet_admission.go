//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"net/netip"
	"slices"
)

type packetRefusal string

func (e packetRefusal) Error() string { return string(e) }

const (
	packetFraming  packetRefusal = "packet-framing-or-mtu-unproven"
	packetHeader   packetRefusal = "packet-invalid-ipv4"
	packetEndpoint packetRefusal = "packet-outside-owned-endpoints"
	packetProtocol packetRefusal = "packet-protocol-not-supported"
	packetFragment packetRefusal = "packet-invalid-or-unsupported-fragment"
	packetChecksum packetRefusal = "packet-invalid-checksum"
	packetICMP     packetRefusal = "packet-icmp-not-admitted"
)

type packetDirection uint8

const (
	packetToPeer packetDirection = iota + 1
	packetToKernel
)

// These inputs are correlation/readback claims until bound to original FD by a
// future supplier. No caller can use this policy as OS/readiness evidence.
type packetFramingReadback struct {
	Own, Peer                    netip.Addr
	LocalMTU, PeerMTU            int
	TUN, NoPI, NoVNET, NoOffload bool
}
type endpointPacketPolicy struct {
	own, peer [4]byte
	mtu       int
}

func newEndpointPacketPolicy(r packetFramingReadback) (*endpointPacketPolicy, error) {
	if !r.TUN || !r.NoPI || !r.NoVNET || !r.NoOffload || r.LocalMTU != MTU || r.PeerMTU != MTU || !r.Own.Is4() || !r.Peer.Is4() || r.Own == r.Peer || !netip.MustParsePrefix("100.64.0.0/10").Contains(r.Own) || !netip.MustParsePrefix("100.64.0.0/10").Contains(r.Peer) {
		return nil, packetFraming
	}
	return &endpointPacketPolicy{own: r.Own.As4(), peer: r.Peer.As4(), mtu: MTU}, nil
}

// Opaque immutable copy, no TUN FD/callback/transport execution. Each fragment is
// passed as-is to receiving kernel, never user reassembly or inferred L4 proof.
type admittedEndpointPacket struct {
	bytes    []byte
	fragment bool
}

func packetSum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b))
		b = b[2:]
	}
	if len(b) > 0 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return ^uint16(sum)
}
func packetL4Valid(ip []byte) bool {
	payload := ip[20:]
	pseudo := make([]byte, 12+len(payload))
	copy(pseudo, ip[12:20])
	pseudo[9] = ip[9]
	binary.BigEndian.PutUint16(pseudo[10:], uint16(len(payload)))
	copy(pseudo[12:], payload)
	return packetSum(pseudo) == 0
}
func (p *endpointPacketPolicy) admit(raw []byte, d packetDirection) (admittedEndpointPacket, error) {
	var zero admittedEndpointPacket
	if p == nil || p.mtu != MTU {
		return zero, packetFraming
	}
	if len(raw) < 20 || len(raw) > p.mtu || raw[0] != 0x45 || int(binary.BigEndian.Uint16(raw[2:])) != len(raw) || raw[8] == 0 {
		return zero, packetHeader
	}
	if packetSum(raw[:20]) != 0 {
		return zero, packetChecksum
	}
	src, dst := p.own, p.peer
	if d == packetToKernel {
		src, dst = p.peer, p.own
	} else if d != packetToPeer {
		return zero, packetEndpoint
	}
	if [4]byte(raw[12:16]) != src || [4]byte(raw[16:20]) != dst {
		return zero, packetEndpoint
	}
	field := binary.BigEndian.Uint16(raw[6:])
	offset := int(field&0x1fff) * 8
	more := field&0x2000 != 0
	fragment := more || offset != 0
	if field&0x8000 != 0 || ((field&0x4000) != 0 && fragment) || len(raw) == 20 || offset+len(raw)-20 > 65535-20 || (more && (len(raw)-20)%8 != 0) {
		return zero, packetFragment
	}
	proto := raw[9]
	if proto != 6 && proto != 17 && proto != 1 {
		return zero, packetProtocol
	}
	if fragment {
		// ICMP control types can't be safely filtered in non-first fragments without
		// reassembly/state. Ordinary TCP/UDP fragments stay supported, kernel owns
		// overlap/timeout/reassembly and final transport checksum validation.
		if proto == 1 {
			return zero, packetFragment
		}
		return admittedEndpointPacket{bytes: slices.Clone(raw), fragment: true}, nil
	}
	payload := raw[20:]
	switch proto {
	case 6:
		if len(payload) < 20 || int(payload[12]>>4)*4 < 20 || int(payload[12]>>4)*4 > len(payload) {
			return zero, packetHeader
		}
		if !packetL4Valid(raw) {
			return zero, packetChecksum
		}
	case 17:
		if len(payload) < 8 || int(binary.BigEndian.Uint16(payload[4:])) != len(payload) {
			return zero, packetHeader
		}
		if binary.BigEndian.Uint16(payload[6:]) != 0 && !packetL4Valid(raw) {
			return zero, packetChecksum
		}
	case 1:
		if e := p.admitICMP(payload, src, dst); e != nil {
			return zero, e
		}
	}
	return admittedEndpointPacket{bytes: slices.Clone(raw)}, nil
}
func (p *endpointPacketPolicy) admitICMP(b []byte, src, dst [4]byte) error {
	if len(b) < 8 {
		return packetICMP
	}
	if packetSum(b) != 0 {
		return packetChecksum
	}
	typ, code := b[0], b[1]
	if typ == 0 || typ == 8 {
		if code != 0 {
			return packetICMP
		}
		return nil
	}
	allowed := false
	switch typ {
	case 3:
		switch code {
		case 0, 1, 2, 3, 4, 9, 10, 13:
			allowed = true
		}
	case 11:
		allowed = code <= 1
	case 12:
		allowed = code <= 2
	}
	if !allowed || len(b) < 8+28 {
		return packetICMP
	}
	q := b[8:]
	if q[0] != 0x45 || packetSum(q[:20]) != 0 || int(binary.BigEndian.Uint16(q[2:])) < 28 || int(binary.BigEndian.Uint16(q[2:])) < len(q) || [4]byte(q[12:16]) != dst || [4]byte(q[16:20]) != src || binary.BigEndian.Uint16(q[6:])&0x9fff != 0 {
		return packetICMP
	}
	// Quote identifies the reverse-direction original endpoint datagram, first
	// fragment only. No recursive ICMP errors or redirect/IGMP protocol quotes.
	if q[9] != 6 && q[9] != 17 && !(q[9] == 1 && (q[20] == 0 || q[20] == 8) && q[21] == 0) {
		return packetICMP
	}
	if typ == 3 && code == 4 {
		mtu := int(binary.BigEndian.Uint16(b[6:8]))
		if b[4] != 0 || b[5] != 0 || (mtu != 0 && (mtu < 68 || mtu > p.mtu)) {
			return packetICMP
		}
	} // zero legacy MTU preserved
	if typ == 12 && int(b[4]) >= len(q) {
		return packetICMP
	}
	return nil
}

// Bounded in-memory drain model, not a reader loop/queue or live execution.
// No payload logs, only finite counters. Caller must service backpressure and
// enforce authority/lifetime separately before every real FD/transport action.
type packetDrainCounts struct {
	Seen, Accepted, Dropped, Bytes uint64
	Refusals                       map[packetRefusal]uint64
}

func (p *endpointPacketPolicy) drainBatch(batch [][]byte, d packetDirection) ([]admittedEndpointPacket, packetDrainCounts, error) {
	c := packetDrainCounts{Refusals: map[packetRefusal]uint64{}}
	if len(batch) > 64 {
		return nil, c, packetFraming
	}
	var out []admittedEndpointPacket
	for _, b := range batch {
		c.Seen++
		c.Bytes += uint64(len(b))
		v, e := p.admit(b, d)
		if e != nil {
			c.Dropped++
			reason, ok := e.(packetRefusal)
			if !ok {
				reason = packetHeader
			}
			c.Refusals[reason]++
			continue
		}
		c.Accepted++
		out = append(out, v)
	}
	return out, c, nil
}
