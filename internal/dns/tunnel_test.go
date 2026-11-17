package dns

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
	"golang.zx2c4.com/wireguard/tun"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

// The client is a second, independent IP stack. Its wire packets cross the
// simulated OS TUN and production multiplexer. No host privileged sockets,
// DNS-handler shortcuts, proofnet tuning, network access or timing sleeps.
type testRawTUN struct {
	in   chan []byte
	out  func([]byte)
	done chan struct{}
	once sync.Once
}

func (r *testRawTUN) File() *os.File           { return nil }
func (r *testRawTUN) Name() (string, error)    { return "dns-test", nil }
func (r *testRawTUN) MTU() (int, error)        { return 1280, nil }
func (r *testRawTUN) Events() <-chan tun.Event { return nil }
func (r *testRawTUN) BatchSize() int           { return 1 }
func (r *testRawTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	select {
	case p := <-r.in:
		sizes[0] = copy(bufs[0][offset:], p)
		return 1, nil
	case <-r.done:
		return 0, io.EOF
	}
}
func (r *testRawTUN) Write(bufs [][]byte, offset int) (int, error) {
	for i, p := range bufs {
		select {
		case <-r.done:
			return i, io.EOF
		default:
		}
		r.out(append([]byte(nil), p[offset:]...))
	}
	return len(bufs), nil
}
func (r *testRawTUN) Close() error { r.once.Do(func() { close(r.done) }); return nil }

func newTunnelPair(t *testing.T) (*Tunnel, *Server, *stack.Stack, <-chan []byte) {
	return newTunnelPairState(t, true)
}
func newTunnelPairState(t *testing.T, start bool) (*Tunnel, *Server, *stack.Stack, <-chan []byte) {
	t.Helper()
	client := stack.New(stack.Options{NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol}, TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol}})
	ep := channel.New(128, 1280, "")
	if err := client.CreateNIC(1, ep); err != nil {
		t.Fatal(err)
	}
	if err := client.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: tcpip.AddrFrom4([4]byte{100, 64, 0, 1}).WithPrefix()}, stack.AddressProperties{}); err != nil {
		t.Fatal(err)
	}
	client.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}})
	raw := &testRawTUN{in: make(chan []byte, 128), done: make(chan struct{}), out: func(p []byte) {
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(p)})
		ep.InjectInbound(ipv4.ProtocolNumber, pkt)
		pkt.DecRef()
	}}
	wrapped := NewTunnel(raw)
	s, _ := New(LocalIPv4)
	s.Update(testMap())
	if start {
		if err := wrapped.StartDNS(s, 1280); err != nil {
			t.Fatal(err)
		}
	}
	forwarded := make(chan []byte, 16)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			p := ep.ReadContext(ctx)
			if p == nil {
				return
			}
			v := p.ToView()
			b := append([]byte(nil), v.AsSlice()...)
			v.Release()
			p.DecRef()
			select {
			case raw.in <- b:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			buf := make([]byte, 65536)
			sizes := []int{0}
			n, err := wrapped.Read([][]byte{buf}, sizes, 16)
			if err != nil {
				return
			}
			if n == 1 {
				select {
				case forwarded <- append([]byte(nil), buf[16:16+sizes[0]]...):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	t.Cleanup(func() { cancel(); wrapped.Close(); ep.Close(); client.Destroy(); wg.Wait() })
	return wrapped, s, client, forwarded
}
func testDNSQuery(t *testing.T, client *stack.Stack, transport, name string) *mdns.Msg {
	t.Helper()
	addr := tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4([4]byte{100, 64, 0, 0}), Port: 53}
	var c net.Conn
	var err error
	if transport == "udp" {
		c, err = gonet.DialUDP(client, nil, &addr, ipv4.ProtocolNumber)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c, err = gonet.DialContextTCP(ctx, client, addr, ipv4.ProtocolNumber)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	conn := &mdns.Conn{Conn: c}
	q := new(mdns.Msg)
	q.SetQuestion(name, mdns.TypeA)
	if err = conn.WriteMsg(q); err != nil {
		t.Fatal(err)
	}
	response, err := conn.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if response.Id != q.Id {
		t.Fatal("query id mismatch")
	}
	return response
}
func TestTunnelResolvesUDPAndTCPAndUpdates(t *testing.T) {
	_, s, client, forwarded := newTunnelPair(t)
	if !s.Ready() {
		t.Fatal("DNS not ready")
	}
	for _, transport := range []string{"udp", "tcp"} {
		r := testDNSQuery(t, client, transport, "server.home.mnet.")
		if len(r.Answer) != 1 || r.Answer[0].(*mdns.A).A.String() != "100.64.0.2" {
			t.Fatalf("%s answer %v", transport, r)
		}
	}
	m := testMap()
	m.Peers = nil
	s.Update(m)
	for _, transport := range []string{"udp", "tcp"} {
		if r := testDNSQuery(t, client, transport, "server.home.mnet."); r.Rcode != mdns.RcodeNameError {
			t.Fatalf("stale answer %v", r)
		}
	}
	select {
	case <-forwarded:
		t.Fatal("DNS leaked to WireGuard")
	default:
	}
}
func TestTunnelPassesNonDNSAndSerializesReplies(t *testing.T) {
	wrapped, _, client, forwarded := newTunnelPair(t)
	// Ordinary mesh packets must pass unchanged, even with nonzero headroom.
	packet := make([]byte, 28)
	packet[0] = 0x45
	packet[9] = 17
	copy(packet[16:20], []byte{100, 64, 0, 2})
	binary.BigEndian.PutUint16(packet[22:24], 53)
	raw := wrapped.Device.(*testRawTUN)
	raw.in <- packet
	select {
	case got := <-forwarded:
		if string(got) != string(packet) {
			t.Fatal("changed mesh packet")
		}
	case <-time.After(time.Second):
		t.Fatal("mesh packet blocked")
	}
	// Reply path is exercised concurrently with normal WireGuard writes.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			wrapped.Write([][]byte{{0}}, 0)
		}
	}()
	testDNSQuery(t, client, "udp", "laptop.home.mnet.")
	<-done
}
func TestTunnelCloseAndStartGuards(t *testing.T) {
	wrapped, s, _, _ := newTunnelPair(t)
	second, _ := New(LocalIPv4)
	if wrapped.StartDNS(second, 1280) == nil {
		t.Fatal("duplicate start")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); wrapped.Close() }()
	}
	wg.Wait()
	if s.Ready() {
		t.Fatal("ready after close")
	}
	if wrapped.StartDNS(second, 1280) == nil {
		t.Fatal("revived closed tunnel")
	}
}
func TestLocalDNSClassification(t *testing.T) {
	ip := [4]byte{100, 64, 0, 0}
	for n := 0; n < 48; n++ {
		if localDNSPacket(make([]byte, n), ip) {
			t.Fatal("zero packet classified")
		}
	}
	packet := make([]byte, 28)
	packet[0] = 0x45
	packet[9] = 17
	copy(packet[16:20], ip[:])
	binary.BigEndian.PutUint16(packet[22:24], 53)
	if !localDNSPacket(packet, ip) {
		t.Fatal("UDP not captured")
	}
	packet[9] = 6
	if !localDNSPacket(packet, ip) {
		t.Fatal("TCP not captured")
	}
	packet[9] = 1
	if localDNSPacket(packet, ip) {
		t.Fatal("ICMP captured")
	}
	packet[9] = 17
	packet[23] = 54
	if localDNSPacket(packet, ip) {
		t.Fatal("non-DNS captured")
	}
	packet[6] = 0x20
	if !localDNSPacket(packet, ip) {
		t.Fatal("fragment escaped")
	}
	packet[0] = 0x41
	if localDNSPacket(packet, ip) {
		t.Fatal("invalid header captured")
	}
}

func TestMobileUpstreamCannotLoopBackIntoMesh(t *testing.T) {
	for _, config := range []string{"nameserver 100.64.0.0", "nameserver 100.65.0.1", "nameserver invalid", "nameserver 100.127.255.1"} {
		if got := chooseUpstream(config); got != "1.1.1.1:53" {
			t.Fatalf("recursive resolver selected: %s", got)
		}
	}
	if got := chooseUpstream("nameserver 100.64.0.0\nnameserver 9.9.9.9"); got != "9.9.9.9:53" {
		t.Fatal(got)
	}
	if got := chooseUpstream("nameserver 2001:4860:4860::8888"); got != "[2001:4860:4860::8888]:53" {
		t.Fatal(got)
	}
}
func FuzzLocalDNSPacket(f *testing.F) {
	f.Add([]byte{0x45})
	f.Add(make([]byte, 28))
	f.Fuzz(func(t *testing.T, p []byte) { localDNSPacket(p, [4]byte{100, 64, 0, 0}); localDNSDestination(p) })
}

func TestTunnelReservedAddressFailsClosedOutsideDNS(t *testing.T) {
	wrapped, _, _, forwarded := newTunnelPair(t)
	raw := wrapped.Device.(*testRawTUN)
	// A reserved destination must not escape to a default-route exit peer.
	for _, protocol := range []byte{1, 6, 17} {
		p := make([]byte, 28)
		p[0] = 0x45
		p[9] = protocol
		copy(p[16:20], []byte{100, 64, 0, 0})
		binary.BigEndian.PutUint16(p[22:24], 9999)
		raw.in <- p
	}
	// This sentinel proves the Read loop consumed all preceding reserved data.
	sentinel := []byte{1, 2, 3}
	raw.in <- sentinel
	select {
	case got := <-forwarded:
		if string(got) != string(sentinel) {
			t.Fatal("reserved packet leaked")
		}
	case <-time.After(time.Second):
		t.Fatal("reader stopped")
	}
}
func TestTunnelRejectsInvalidEndpointAndMTU(t *testing.T) {
	for _, tc := range []struct {
		address string
		mtu     int
	}{{"100.64.0.1", 1280}, {"::1", 1280}, {LocalIPv4, -1}, {LocalIPv4, 65536}} {
		raw := &testRawTUN{done: make(chan struct{})}
		wrapped := NewTunnel(raw)
		s, _ := New(tc.address)
		if wrapped.StartDNS(s, tc.mtu) == nil {
			t.Fatal("invalid endpoint accepted")
		}
		if s.Ready() {
			t.Fatal("invalid endpoint ready")
		}
		wrapped.Close()
	}
}

func TestTunnelRequestWorkIsBounded(t *testing.T) {
	_, s, client, _ := newTunnelPair(t)
	for i := 0; i < cap(s.requestSlots); i++ {
		s.requestSlots <- struct{}{}
	}
	for _, transport := range []string{"udp", "tcp"} {
		if got := testDNSQuery(t, client, transport, "laptop.home.mnet."); got.Rcode != mdns.RcodeRefused {
			t.Fatal("overload not refused")
		}
	}
	for i := 0; i < cap(s.requestSlots); i++ {
		<-s.requestSlots
	}
	if got := testDNSQuery(t, client, "udp", "laptop.home.mnet."); len(got.Answer) != 1 {
		t.Fatal("work slots not recovered")
	}
}
func TestLimitedDNSListenerReleasesSlotsExactlyOnce(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	listener := &limitedListener{Listener: raw, slots: make(chan struct{}, 1)}
	first, err := net.Dial("tcp", raw.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	second, err := net.Dial("tcp", raw.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	done := make(chan error, 1)
	go func() {
		c, err := listener.Accept()
		if err == nil {
			c.Close()
		}
		done <- err
	}()
	second.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := second.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("overflow not closed: %v", err)
	}
	accepted.Close()
	accepted.Close()
	third, err := net.Dial("tcp", raw.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slot not recovered")
	}
	if len(listener.slots) != 0 {
		t.Fatal("connection slot leaked")
	}
}

func TestTunnelRetainsEarlyAndroidDNSUntilFirstMapIsReady(t *testing.T) {
	for _, transport := range []string{"udp", "tcp"} {
		t.Run(transport, func(t *testing.T) {
			wrapped, s, client, _ := newTunnelPairState(t, false)
			done := make(chan struct{})
			go func() {
				defer close(done)
				r := testDNSQuery(t, client, transport, "server.home.mnet.")
				if len(r.Answer) != 1 {
					t.Error("startup query lost")
				}
			}()
			deadline := time.Now().Add(time.Second)
			for {
				wrapped.mu.Lock()
				pending := len(wrapped.pending)
				wrapped.mu.Unlock()
				if pending > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("query not queued")
				}
				runtime.Gosched()
			}
			if err := wrapped.StartDNS(s, 1280); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("startup query not answered")
			}
			wrapped.mu.Lock()
			defer wrapped.mu.Unlock()
			if len(wrapped.pending) != 0 {
				t.Fatal("startup queue leaked")
			}
		})
	}
}
