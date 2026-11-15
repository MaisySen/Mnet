package stun

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// TestRoundTrip answers a real binding request over UDP with a hand built
// XOR-MAPPED-ADDRESS response and checks the parse.
func TestRoundTrip(t *testing.T) {
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	go func() {
		buf := make([]byte, 512)
		n, src, err := srv.ReadFromUDP(buf)
		if err != nil {
			return
		}
		msg := buf[:n]
		if binary.BigEndian.Uint16(msg[0:]) != bindingRequest {
			t.Errorf("not a binding request")
			return
		}
		// XOR-MAPPED-ADDRESS for 203.0.113.9:51234.
		resp := make([]byte, 20, 32)
		copy(resp, msg)
		binary.BigEndian.PutUint16(resp[0:], bindingResponse)
		attr := []byte{
			0x00, attrXorMapped, 0x00, 0x08,
			0x00, 0x01,
			0, 0, 0, 0, 0, 0,
		}
		port := uint16(51234) ^ uint16(magicCookie>>16)
		binary.BigEndian.PutUint16(attr[2:], 8)
		binary.BigEndian.PutUint16(attr[6:], port)
		ip := net.ParseIP("203.0.113.9").To4()
		cookie := [4]byte{0x21, 0x12, 0xa4, 0x42}
		for i := 0; i < 4; i++ {
			attr[8+i] = ip[i] ^ cookie[i]
		}
		resp = append(resp, attr...)
		binary.BigEndian.PutUint16(resp[2:], uint16(len(attr)))
		srv.WriteToUDP(resp, src)
	}()

	cli, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	req, txid := NewRequest()
	if len(req) != 20 {
		t.Fatalf("request length %d", len(req))
	}
	if _, err := cli.WriteToUDP(req, srv.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 512)
	n, _, err := cli.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := ParseResponse(buf[:n], txid)
	if err != nil {
		t.Fatal(err)
	}
	if ep != "203.0.113.9:51234" {
		t.Fatalf("endpoint %q", ep)
	}

	// Wrong transaction ID is rejected.
	if _, err := ParseResponse(buf[:n], TxID{}); err == nil {
		t.Fatal("accepted wrong txid")
	}
}
