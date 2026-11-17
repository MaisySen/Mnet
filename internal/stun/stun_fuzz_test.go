package stun

import (
	"encoding/binary"
	"testing"
)

// Public STUN replies are untrusted datagrams. Parsing must remain bounded and
// cannot accept a response for a different transaction ID.
func FuzzParseResponse(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 1})
	var id TxID
	copy(id[:], []byte("fixture-id12"))
	reply := make([]byte, 32)
	binary.BigEndian.PutUint16(reply[0:], bindingResponse)
	binary.BigEndian.PutUint16(reply[2:], 12)
	binary.BigEndian.PutUint32(reply[4:], magicCookie)
	copy(reply[8:], id[:])
	binary.BigEndian.PutUint16(reply[20:], attrMapped)
	binary.BigEndian.PutUint16(reply[22:], 8)
	reply[25] = 1
	binary.BigEndian.PutUint16(reply[26:], 3478)
	copy(reply[28:], []byte{192, 0, 2, 1})
	f.Add(reply)
	f.Fuzz(func(t *testing.T, packet []byte) {
		endpoint, err := ParseResponse(packet, id)
		if err != nil {
			return
		}
		declared := 20 + int(binary.BigEndian.Uint16(packet[2:]))
		// Trimming bytes outside the declared STUN message cannot invalidate
		// or change an accepted endpoint.
		trimmed, trimErr := ParseResponse(packet[:declared], id)
		if trimErr != nil || trimmed != endpoint {
			t.Fatal("accepted reply depended on trailing bytes")
		}
		if endpoint == "" {
			t.Fatal("accepted reply has no endpoint")
		}
		other := id
		other[0] ^= 0xff
		if _, err := ParseResponse(packet, other); err == nil {
			t.Fatal("accepted mismatched transaction")
		}
		if len(packet) < 20 || int(binary.BigEndian.Uint16(packet[2:])) > len(packet)-20 {
			t.Fatal("accepted truncated reply")
		}
	})
}
