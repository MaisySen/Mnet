// Package stun builds and parses minimal RFC 5389 binding messages. The
// daemon sends requests from the WireGuard socket itself, so the NAT
// mapping it discovers is the mapping WireGuard uses.
package stun

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const (
	bindingRequest  = 0x0001
	bindingResponse = 0x0101
	magicCookie     = 0x2112A442
	attrXorMapped   = 0x0020
	attrMapped      = 0x0001
)

// TxID identifies one request.
type TxID [12]byte

// DefaultServers are public STUN servers tried in order.
var DefaultServers = []string{
	"stun.l.google.com:19302",
	"stun.cloudflare.com:3478",
	"stun1.l.google.com:19302",
}

// NewRequest builds a binding request and its transaction ID.
func NewRequest() ([]byte, TxID) {
	var txid TxID
	rand.Read(txid[:])
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:], bindingRequest)
	binary.BigEndian.PutUint16(req[2:], 0)
	binary.BigEndian.PutUint32(req[4:], magicCookie)
	copy(req[8:], txid[:])
	return req, txid
}

// ParseResponse extracts the reflexive address from a binding response
// matching txid.
func ParseResponse(msg []byte, txid TxID) (string, error) {
	if len(msg) < 20 || binary.BigEndian.Uint16(msg[0:]) != bindingResponse {
		return "", errors.New("not a binding response")
	}
	if binary.BigEndian.Uint32(msg[4:]) != magicCookie {
		return "", errors.New("bad cookie")
	}
	var got TxID
	copy(got[:], msg[8:20])
	if got != txid {
		return "", errors.New("transaction mismatch")
	}
	length := int(binary.BigEndian.Uint16(msg[2:]))
	if len(msg) < 20+length {
		return "", errors.New("short message")
	}
	off := 20
	for off+4 <= 20+length {
		typ := binary.BigEndian.Uint16(msg[off:])
		l := int(binary.BigEndian.Uint16(msg[off+2:]))
		if off+4+l > len(msg) {
			break
		}
		val := msg[off+4 : off+4+l]
		switch typ {
		case attrXorMapped:
			if len(val) >= 8 && val[1] == 0x01 { // IPv4
				port := binary.BigEndian.Uint16(val[2:]) ^ uint16(magicCookie>>16)
				ip := make(net.IP, 4)
				cookie := [4]byte{0x21, 0x12, 0xa4, 0x42}
				for i := 0; i < 4; i++ {
					ip[i] = val[4+i] ^ cookie[i]
				}
				return fmt.Sprintf("%s:%d", ip, port), nil
			}
		case attrMapped:
			if len(val) >= 8 && val[1] == 0x01 {
				port := binary.BigEndian.Uint16(val[2:])
				return fmt.Sprintf("%s:%d", net.IP(val[4:8]), port), nil
			}
		}
		off += 4 + l
		if l%4 != 0 {
			off += 4 - l%4
		}
	}
	return "", errors.New("no mapped address in response")
}
