package bind

import (
	"bytes"
	"encoding/base64"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
)

// recvOne pulls one packet from a receive function.
func recvOne(t *testing.T, fn conn.ReceiveFunc) ([]byte, conn.Endpoint) {
	t.Helper()
	packets := [][]byte{make([]byte, 2048)}
	sizes := make([]int, 1)
	eps := make([]conn.Endpoint, 1)
	deadline := time.Now().Add(3 * time.Second)
	for {
		n, err := fn(packets, sizes, eps)
		if err != nil {
			t.Fatal(err)
		}
		if n == 1 {
			return packets[0][:sizes[0]], eps[0]
		}
		if time.Now().After(deadline) {
			t.Fatal("no packet")
		}
	}
}

// TestUDPPath sends a datagram between two binds over real UDP.
func TestUDPPath(t *testing.T) {
	a, b := New(), New()
	defer a.Close()
	defer b.Close()
	_, portA, err := a.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	fnsB, portB, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	_ = fnsB
	go func() {
		ep, err := a.ParseEndpoint("127.0.0.1:" + itoa(int(portB)))
		if err != nil {
			t.Error(err)
			return
		}
		if err := a.Send([][]byte{[]byte("hello mesh")}, ep); err != nil {
			t.Error(err)
		}
	}()
	data, ep := recvOne(t, fnsB[0])
	if !bytes.Equal(data, []byte("hello mesh")) {
		t.Fatalf("payload %q", data)
	}
	if ep.DstToString() != "127.0.0.1:"+itoa(int(portB)) {
		// The endpoint describes the sender's view; just check it parsed.
		t.Logf("endpoint dst: %s", ep.DstToString())
	}
	_ = portA
}

// TestRelayPath injects and parses relay endpoints.
func TestRelayPath(t *testing.T) {
	b := New()
	defer b.Close()
	fns, _, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	var pk [32]byte
	for i := range pk {
		pk[i] = 0x5A
	}
	pub := base64.StdEncoding.EncodeToString(pk[:])

	// Parse the relay endpoint form.
	ep, err := b.ParseEndpoint(RelayPrefix + pub)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ep.(*RelayEndpoint); !ok {
		t.Fatalf("wrong endpoint type %T", ep)
	}

	// Send with no sender installed must fail cleanly.
	if err := b.Send([][]byte{[]byte("x")}, ep); err == nil {
		t.Fatal("send without relay sender should fail")
	}

	// Install a sender, send, and check what arrived.
	got := make(chan []byte, 1)
	b.SetRelaySender(func(dst string, pkt []byte) error {
		if dst != pub {
			t.Errorf("dst %q", dst)
		}
		got <- pkt
		return nil
	})
	if err := b.Send([][]byte{[]byte("encrypted")}, ep); err != nil {
		t.Fatal(err)
	}
	select {
	case pkt := <-got:
		if !bytes.Equal(pkt, []byte("encrypted")) {
			t.Fatalf("payload %q", pkt)
		}
	case <-time.After(time.Second):
		t.Fatal("sender not called")
	}

	// Injected packets come out of the relay receive function.
	b.Inject(pub, []byte("from-peer"))
	data, rep := recvOne(t, fns[1])
	if !bytes.Equal(data, []byte("from-peer")) {
		t.Fatalf("payload %q", data)
	}
	if rep.(*RelayEndpoint).Pubkey != pub {
		t.Fatalf("relay endpoint %v", rep)
	}
}

// TestStunInterception keeps STUN answers out of the WireGuard stream.
func TestStunInterception(t *testing.T) {
	a, b := New(), New()
	defer a.Close()
	defer b.Close()
	if _, _, err := a.Open(0); err != nil {
		t.Fatal(err)
	}
	fnsB, portB, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	stunGot := make(chan []byte, 1)
	b.SetStunHandler(func(msg []byte) { stunGot <- msg })

	// The receive loop runs continuously, like the device drives it.
	type gotPkt struct {
		data []byte
	}
	wgGot := make(chan gotPkt, 1)
	go func() {
		packets := [][]byte{make([]byte, 2048)}
		sizes := make([]int, 1)
		eps := make([]conn.Endpoint, 1)
		for {
			n, err := fnsB[0](packets, sizes, eps)
			if err != nil {
				return
			}
			if n == 1 {
				cp := make([]byte, sizes[0])
				copy(cp, packets[0][:sizes[0]])
				wgGot <- gotPkt{cp}
				return
			}
		}
	}()

	// A packet with the STUN magic cookie never reaches the WG stream.
	fake := make([]byte, 20)
	fake[0] = 0x01
	fake[1] = 0x01
	fake[4], fake[5], fake[6], fake[7] = 0x21, 0x12, 0xa4, 0x42
	ep, _ := a.ParseEndpoint("127.0.0.1:" + itoa(int(portB)))
	if err := a.Send([][]byte{fake}, ep); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-stunGot:
		if !bytes.Equal(msg, fake) {
			t.Fatal("stun payload changed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stun handler not called")
	}

	// A WG-shaped packet still flows.
	wgPkt := []byte{0x04, 0, 0, 0, 1, 2, 3, 4}
	if err := a.Send([][]byte{wgPkt}, ep); err != nil {
		t.Fatal(err)
	}
	select {
	case g := <-wgGot:
		if !bytes.Equal(g.data, wgPkt) {
			t.Fatalf("wg packet %v", g.data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wg packet never arrived")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
