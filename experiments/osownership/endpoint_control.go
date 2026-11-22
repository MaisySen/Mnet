//go:build mnet_enrollment_development

package osownership

import (
	"encoding/base64"
	"encoding/binary"
	"net/netip"
)

// Fixed-layout versioned DATA only. Process identity is compared against the
// actual retained child by the parent, never trusted because it decoded.
const endpointPreparedBytes = 144
const endpointAdmissionBytes = 224

type endpointPreparedBody struct {
	PID                      uint32
	StartTicks               uint64
	Namespace                Namespace
	Generation               uint64
	MTU                      uint32
	Own, Peer                [4]byte
	OwnKey, PeerKey          [32]byte
	SelfUntilMS, PeerUntilMS int64
}

func (b endpointPreparedBody) valid() bool {
	own, peer := netip.AddrFrom4(b.Own), netip.AddrFrom4(b.Peer)
	prefix := netip.MustParsePrefix("100.64.0.0/10")
	return b.PID > 1 && b.StartTicks > 0 && b.Namespace.Device > 0 && b.Namespace.Inode > 0 && b.Generation > 0 && b.MTU == MTU && own != peer && prefix.Contains(own) && prefix.Contains(peer) && b.OwnKey != ([32]byte{}) && b.PeerKey != ([32]byte{}) && b.OwnKey != b.PeerKey && b.SelfUntilMS > 0 && b.PeerUntilMS > 0
}
func (b endpointPreparedBody) encode() ([]byte, error) {
	if !b.valid() {
		return nil, errEndpointFrame
	}
	p := make([]byte, endpointPreparedBytes)
	binary.BigEndian.PutUint16(p, 1)
	binary.BigEndian.PutUint32(p[4:], b.PID)
	binary.BigEndian.PutUint64(p[8:], b.StartTicks)
	binary.BigEndian.PutUint64(p[16:], b.Namespace.Device)
	binary.BigEndian.PutUint64(p[24:], b.Namespace.Inode)
	binary.BigEndian.PutUint64(p[32:], b.Generation)
	binary.BigEndian.PutUint32(p[40:], b.MTU)
	copy(p[44:], b.Own[:])
	copy(p[48:], b.Peer[:])
	copy(p[52:], b.OwnKey[:])
	copy(p[84:], b.PeerKey[:])
	binary.BigEndian.PutUint64(p[116:], uint64(b.SelfUntilMS))
	binary.BigEndian.PutUint64(p[124:], uint64(b.PeerUntilMS))
	return p, nil
}
func decodeEndpointPrepared(p []byte) (b endpointPreparedBody, err error) {
	if len(p) != endpointPreparedBytes || binary.BigEndian.Uint16(p) != 1 || binary.BigEndian.Uint16(p[2:]) != 0 {
		return b, errEndpointFrame
	}
	for _, v := range p[132:] {
		if v != 0 {
			return b, errEndpointFrame
		}
	}
	b.PID = binary.BigEndian.Uint32(p[4:])
	b.StartTicks = binary.BigEndian.Uint64(p[8:])
	b.Namespace = Namespace{Device: binary.BigEndian.Uint64(p[16:]), Inode: binary.BigEndian.Uint64(p[24:]), Isolated: true}
	b.Generation = binary.BigEndian.Uint64(p[32:])
	b.MTU = binary.BigEndian.Uint32(p[40:])
	copy(b.Own[:], p[44:48])
	copy(b.Peer[:], p[48:52])
	copy(b.OwnKey[:], p[52:84])
	copy(b.PeerKey[:], p[84:116])
	b.SelfUntilMS = int64(binary.BigEndian.Uint64(p[116:]))
	b.PeerUntilMS = int64(binary.BigEndian.Uint64(p[124:]))
	if !b.valid() {
		return endpointPreparedBody{}, errEndpointFrame
	}
	return b, nil
}

type endpointAdmissionBody struct {
	Prepared                         endpointPreparedBody
	SelfPublication, PeerPublication [32]byte
	SelfRevision, PeerRevision       uint64
}

func (b endpointAdmissionBody) encode() ([]byte, error) {
	p, e := b.Prepared.encode()
	if e != nil {
		return nil, e
	}
	if b.SelfPublication == ([32]byte{}) || b.PeerPublication == ([32]byte{}) || b.SelfRevision == 0 || b.PeerRevision == 0 || b.SelfRevision > 1<<63-1 || b.PeerRevision > 1<<63-1 {
		return nil, errEndpointFrame
	}
	p = append(p, make([]byte, endpointAdmissionBytes-endpointPreparedBytes)...)
	copy(p[144:], b.SelfPublication[:])
	copy(p[176:], b.PeerPublication[:])
	binary.BigEndian.PutUint64(p[208:], b.SelfRevision)
	binary.BigEndian.PutUint64(p[216:], b.PeerRevision)
	return p, nil
}
func decodeEndpointAdmission(p []byte) (b endpointAdmissionBody, e error) {
	if len(p) != endpointAdmissionBytes {
		return b, errEndpointFrame
	}
	b.Prepared, e = decodeEndpointPrepared(p[:144])
	if e != nil {
		return b, e
	}
	copy(b.SelfPublication[:], p[144:176])
	copy(b.PeerPublication[:], p[176:208])
	b.SelfRevision = binary.BigEndian.Uint64(p[208:])
	b.PeerRevision = binary.BigEndian.Uint64(p[216:])
	_, e = b.encode()
	return b, e
}
func validateEndpointControl(f endpointFrame) error {
	switch f.kind {
	case endpointPrepared:
		b, e := decodeEndpointPrepared(f.payload)
		if e != nil {
			return e
		}
		if b.Generation != f.generation {
			return errEndpointFrame
		}
	case endpointAdmission:
		b, e := decodeEndpointAdmission(f.payload)
		if e != nil {
			return e
		}
		if b.Prepared.Generation != f.generation {
			return errEndpointFrame
		}
	}
	return nil
}
func endpointKey(s string) (k [32]byte, e error) {
	p, e := base64.StdEncoding.Strict().DecodeString(s)
	if e != nil || len(p) != 32 || base64.StdEncoding.EncodeToString(p) != s {
		return k, errEndpointFrame
	}
	copy(k[:], p)
	return k, nil
}
