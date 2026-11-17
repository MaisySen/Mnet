package stun

import (
	"encoding/binary"
	"testing"
)

func mappedFixture() ([]byte, TxID) {
	packet := make([]byte, 32)
	var id TxID
	copy(id[:], []byte("bounds-id-12"))
	binary.BigEndian.PutUint16(packet, bindingResponse)
	binary.BigEndian.PutUint16(packet[2:], 12)
	binary.BigEndian.PutUint32(packet[4:], magicCookie)
	copy(packet[8:], id[:])
	binary.BigEndian.PutUint16(packet[20:], attrMapped)
	binary.BigEndian.PutUint16(packet[22:], 8)
	packet[25] = 1
	binary.BigEndian.PutUint16(packet[26:], 3478)
	copy(packet[28:], []byte{192, 0, 2, 1})
	return packet, id
}
func TestAttributeCannotConsumeTrailingDatagramBytes(t *testing.T) {
	for _, length := range []uint16{0, 4, 8, 9, 10, 11} {
		packet, id := mappedFixture()
		binary.BigEndian.PutUint16(packet[2:], length)
		if _, err := ParseResponse(packet, id); err == nil {
			t.Fatalf("accepted address outside declared%dbyte body", length)
		}
	}
	packet, id := mappedFixture()
	packet = append(packet, 1, 2, 3, 4)
	if endpoint, err := ParseResponse(packet, id); err != nil || endpoint != "192.0.2.1:3478" {
		t.Fatalf("valid declared message changed: %s %v", endpoint, err)
	}
}
func TestAttributePaddingAndRemainderValidatedBeforeSuccess(t *testing.T) {
	packet, id := mappedFixture()
	// A successful first attribute must not hide a malformed declared second.
	packet = append(packet, 0, 9, 0, 8)
	binary.BigEndian.PutUint16(packet[2:], 16)
	if _, err := ParseResponse(packet, id); err == nil {
		t.Fatal("ignored malformed declared suffix")
	}
	// Valid unknown one-byte attribute with its3padding bytes before mapped.
	mapped, id := mappedFixture()
	packet = append(append([]byte{}, mapped[:20]...), 0, 9, 0, 1, 7, 0, 0, 0)
	packet = append(packet, mapped[20:]...)
	binary.BigEndian.PutUint16(packet[2:], 20)
	if endpoint, err := ParseResponse(packet, id); err != nil || endpoint != "192.0.2.1:3478" {
		t.Fatalf("padded attribute broke address: %s %v", endpoint, err)
	}
}
