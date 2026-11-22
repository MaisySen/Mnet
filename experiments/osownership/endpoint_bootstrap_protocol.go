//go:build mnet_enrollment_development

package osownership

import (
	"encoding/base64"
	"encoding/binary"
	"net/netip"
	"strings"
	"time"
)

func encodeEndpointKey(k [32]byte) string { return base64.StdEncoding.EncodeToString(k[:]) }
func endpointStopPayload(r endpointStopReason) []byte {
	p := make([]byte, 4)
	binary.BigEndian.PutUint16(p, 1)
	binary.BigEndian.PutUint16(p[2:], uint16(r))
	return p
}
func validateEndpointStop(p []byte) error {
	if len(p) != 4 || binary.BigEndian.Uint16(p) != 1 {
		return errEndpointFrame
	}
	r := endpointStopReason(binary.BigEndian.Uint16(p[2:]))
	if r < endpointReasonRequested || r > endpointReasonTimeout {
		return errEndpointFrame
	}
	return nil
}
func endpointIPv4(s string) ([4]byte, error) {
	a, e := netip.ParseAddr(s)
	if e != nil || !a.Is4() {
		return [4]byte{}, errEndpointFrame
	}
	return a.As4(), nil
}

const endpointResidentMode = "--osownership-resident-endpoint-v1-source"
const endpointStart byte = 6
const endpointStats byte = 7
const endpointStartBytes = 196
const endpointStatsBytes = 80

type endpointStartBody struct {
	Prepared    endpointPreparedBody
	Name, Alias string
}

func (b endpointStartBody) encode() ([]byte, error) {
	p, e := b.Prepared.encode()
	if e != nil {
		return nil, e
	}
	if len(b.Name) < 7 || len(b.Name) > 15 || !strings.HasPrefix(b.Name, "mno-") || !safeInterfaceName(b.Name) || len(b.Alias) != 36 || !strings.HasPrefix(b.Alias, "mno-") {
		return nil, errEndpointFrame
	}
	for _, c := range b.Alias[4:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return nil, errEndpointFrame
		}
	}
	p = append(p, make([]byte, endpointStartBytes-endpointPreparedBytes)...)
	copy(p[144:160], b.Name)
	copy(p[160:196], b.Alias)
	return p, nil
}
func decodeEndpointStart(p []byte) (b endpointStartBody, e error) {
	if len(p) != endpointStartBytes {
		return b, errEndpointFrame
	}
	b.Prepared, e = decodeEndpointPrepared(p[:144])
	if e != nil {
		return b, e
	}
	n := 0
	for n < 16 && p[144+n] != 0 {
		n++
	}
	for _, v := range p[144+n : 160] {
		if v != 0 {
			return b, errEndpointFrame
		}
	}
	b.Name = string(p[144 : 144+n])
	b.Alias = string(p[160:196])
	_, e = b.encode()
	return b, e
}
func (b endpointStartBody) binding() Binding {
	return Binding{Namespace: b.Prepared.Namespace, OwnIP: netip.AddrFrom4(b.Prepared.Own).String(), PeerIP: netip.AddrFrom4(b.Prepared.Peer).String(), OwnKey: encodeEndpointKey(b.Prepared.OwnKey), PeerKey: encodeEndpointKey(b.Prepared.PeerKey), SelfExpiry: time.UnixMilli(b.Prepared.SelfUntilMS), PeerExpiry: time.UnixMilli(b.Prepared.PeerUntilMS), Name: b.Name, Alias: b.Alias}
}

type endpointStopReason uint16

const (
	endpointReasonRequested endpointStopReason = iota + 1
	endpointReasonEOF
	endpointReasonProtocol
	endpointReasonLifetime
	endpointReasonCustody
	endpointReasonPolicyIncomplete
	endpointReasonCleanup
	endpointReasonTimeout
)

type endpointStatsBody struct {
	Reason          endpointStopReason
	Counts          endpointLoopCounts
	CleanupVerified bool
}

func (b endpointStatsBody) encode() ([]byte, error) {
	if b.Reason < endpointReasonRequested || b.Reason > endpointReasonTimeout {
		return nil, errEndpointFrame
	}
	p := make([]byte, endpointStatsBytes)
	binary.BigEndian.PutUint16(p, 1)
	binary.BigEndian.PutUint16(p[2:], uint16(b.Reason))
	if b.CleanupVerified {
		p[4] = 1
	}
	values := []uint64{b.Counts.Turns, b.Counts.Read, b.Counts.Written, b.Counts.PolicyDrops, b.Counts.KernelDrops, b.Counts.QueueDrops, b.Counts.Yields, b.Counts.Fatal}
	for i, v := range values {
		binary.BigEndian.PutUint64(p[8+i*8:], v)
	}
	return p, nil
}
func decodeEndpointStats(p []byte) (b endpointStatsBody, e error) {
	if len(p) != endpointStatsBytes || binary.BigEndian.Uint16(p) != 1 || p[4] > 1 {
		return b, errEndpointFrame
	}
	for _, v := range append(append([]byte(nil), p[5:8]...), p[72:]...) {
		if v != 0 {
			return b, errEndpointFrame
		}
	}
	b.Reason = endpointStopReason(binary.BigEndian.Uint16(p[2:]))
	b.CleanupVerified = p[4] == 1
	values := []*uint64{&b.Counts.Turns, &b.Counts.Read, &b.Counts.Written, &b.Counts.PolicyDrops, &b.Counts.KernelDrops, &b.Counts.QueueDrops, &b.Counts.Yields, &b.Counts.Fatal}
	for i, v := range values {
		*v = binary.BigEndian.Uint64(p[8+i*8:])
	}
	_, e = b.encode()
	return b, e
}
