//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"errors"
)

// Linux6.17 UAPI DATA only. None of these structs/decoders is an ownership
// witness or a supported-kernel attestation. No SA/policy body is requested.
const (
	xfrmNewSADMetadata uint16 = 0x22
	xfrmGetSADMetadata uint16 = 0x23
	xfrmNewSPDMetadata uint16 = 0x24
	xfrmGetSPDMetadata uint16 = 0x25
	xfrmGetDefaults    uint16 = 0x28
)

var errXfrmMetadata = errors.New("xfrm metadata framing or profile unverified")
var errXfrmEnvironment = errors.New("xfrm metadata not stable empty-policy empty-state accept-default")

type xfrmSADData struct {
	Count uint32
	Hash  [2]uint32
}
type xfrmSPDData struct {
	Counts                 [6]uint32
	Hash                   [2]uint32
	Threshold4, Threshold6 [2]byte
}
type xfrmMetadataData struct {
	SAD      xfrmSADData
	SPD      xfrmSPDData
	Defaults [3]byte
}

func (d xfrmMetadataData) emptyAcceptDefaults() bool {
	if d.SAD.Count != 0 || d.Defaults != [3]byte{2, 2, 2} {
		return false
	}
	for _, n := range d.SPD.Counts {
		if n != 0 {
			return false
		}
	}
	return true
}

// Three GETs only. NLM_F_REQUEST, no ACK, DUMP, CREATE or mutation flags.
// Each expected nonmultipart data response is itself mandatory completion;
// an NLMSG_ERROR (including an unsolicited successful ACK) is never data.
func buildXfrmMetadataRequest(kind uint16, seq, port uint32) ([]byte, error) {
	if seq == 0 || port == 0 {
		return nil, errXfrmMetadata
	}
	size := 20
	switch kind {
	case xfrmGetSADMetadata, xfrmGetSPDMetadata, xfrmGetDefaults:
	default:
		return nil, errXfrmMetadata
	}
	b := make([]byte, 20)
	binary.NativeEndian.PutUint32(b, uint32(size))
	binary.NativeEndian.PutUint16(b[4:], kind)
	binary.NativeEndian.PutUint16(b[6:], 1)
	binary.NativeEndian.PutUint32(b[8:], seq)
	binary.NativeEndian.PutUint32(b[12:], port)
	return b, nil
}
func parseXfrmMetadataFrame(d Datagram, kind uint16, seq, port uint32) ([]byte, error) {
	if d.SenderPID != 0 || d.Truncated || seq == 0 || port == 0 || len(d.Bytes) < 16 || len(d.Bytes) > 4096 {
		return nil, errXfrmMetadata
	}
	b := d.Bytes
	n := int(binary.NativeEndian.Uint32(b))
	if n < 16 || n > len(b) || (n+3)&^3 != len(b) || binary.NativeEndian.Uint16(b[4:]) != kind || binary.NativeEndian.Uint16(b[6:]) != 0 || binary.NativeEndian.Uint32(b[8:]) != seq || binary.NativeEndian.Uint32(b[12:]) != port {
		return nil, errXfrmMetadata
	}
	for _, v := range b[n:] {
		if v != 0 {
			return nil, errXfrmMetadata
		}
	}
	return b[16:n], nil
}
func parseXfrmSADMetadata(d Datagram, seq, port uint32) (out xfrmSADData, err error) {
	b, e := parseXfrmMetadataFrame(d, xfrmNewSADMetadata, seq, port)
	if e != nil || len(b) < 4 || binary.NativeEndian.Uint32(b) != 0 {
		return out, errXfrmMetadata
	}
	a, e := decodeAttributes(b[4:])
	if e != nil || len(a) != 2 || len(a[1]) != 4 || len(a[2]) != 8 {
		return out, errXfrmMetadata
	}
	out.Count = binary.NativeEndian.Uint32(a[1])
	out.Hash = [2]uint32{binary.NativeEndian.Uint32(a[2]), binary.NativeEndian.Uint32(a[2][4:])}
	return out, nil
}
func parseXfrmSPDMetadata(d Datagram, seq, port uint32) (out xfrmSPDData, err error) {
	b, e := parseXfrmMetadataFrame(d, xfrmNewSPDMetadata, seq, port)
	if e != nil || len(b) < 4 || binary.NativeEndian.Uint32(b) != 0 {
		return out, errXfrmMetadata
	}
	a, e := decodeAttributes(b[4:])
	if e != nil || len(a) != 4 || len(a[1]) != 24 || len(a[2]) != 8 || len(a[3]) != 2 || len(a[4]) != 2 {
		return out, errXfrmMetadata
	}
	for i := range out.Counts {
		out.Counts[i] = binary.NativeEndian.Uint32(a[1][4*i:])
	}
	out.Hash = [2]uint32{binary.NativeEndian.Uint32(a[2]), binary.NativeEndian.Uint32(a[2][4:])}
	copy(out.Threshold4[:], a[3])
	copy(out.Threshold6[:], a[4])
	if out.Threshold4[0] > 32 || out.Threshold4[1] > 32 || out.Threshold6[0] > 128 || out.Threshold6[1] > 128 {
		return xfrmSPDData{}, errXfrmMetadata
	}
	return out, nil
}
func parseXfrmDefaults(d Datagram, seq, port uint32) (out [3]byte, err error) {
	b, e := parseXfrmMetadataFrame(d, xfrmGetDefaults, seq, port)
	// xfrm_get_default calls nlmsg_end after nlmsg_put(sizeof(struct)=3).
	// The aligned skb tail makes nlmsg_len20: three values plus one zero byte.
	if e != nil || len(b) != 4 || b[3] != 0 {
		return out, errXfrmMetadata
	}
	for i, v := range b[:3] {
		if v != 1 && v != 2 {
			return [3]byte{}, errXfrmMetadata
		}
		out[i] = v
	}
	return out, nil
}
