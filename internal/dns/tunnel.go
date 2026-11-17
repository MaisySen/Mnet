package dns

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

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
	"gvisor.dev/gvisor/pkg/waiter"
)

// LocalIPv4 is a tunnel-local resolver address, outside the mesh allocation and its network/broadcast addresses.
// Platform routes must send this /32 into the VPN. It must not be assigned to
// the OS interface itself, which would route DNS to the host's local stack.
const LocalIPv4 = "198.18.0.53"

// Tunnel intercepts only local IPv4 DNS traffic before WireGuard. Android and
// iOS app processes cannot bind privileged OS port53. This isolated userspace
// stack owns only the node's DNS endpoint; it is not an exit node or TCP proxy.
// UDP and TCP use the same DNS handler as desktop listeners. Peer traffic keeps
// the original tunnel path. All writes are serialized with resolver replies.
type Tunnel struct {
	tun.Device
	writeMu sync.Mutex
	mu      sync.RWMutex
	local   *localEndpoint
	udpSeen atomic.Bool
	pending [][]byte // bounded DNS packets received while the first map is loading
	closed  bool
	once    sync.Once
}
type localEndpoint struct {
	ip     [4]byte
	stack  *stack.Stack
	ep     *channel.Endpoint
	cancel context.CancelFunc
	done   chan struct{}
	server *Server
}

func NewTunnel(raw tun.Device) *Tunnel { return &Tunnel{Device: raw} }

// StartDNS binds userspace port53, never an OS privileged socket. The supplied
// server is owned until Tunnel.Close; ordinary Server.Close can also stop it.
func (t *Tunnel) StartDNS(s *Server, mtu int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.local != nil {
		return fmt.Errorf("local DNS tunnel unavailable")
	}
	ip, err := netip.ParseAddr(s.addr)
	if err != nil || s.addr != LocalIPv4 || mtu < 576 || mtu > 65535 {
		return fmt.Errorf("invalid local DNS endpoint")
	}
	ns := stack.New(stack.Options{NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol}, TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol}, HandleLocal: false})
	ep := channel.New(128, uint32(mtu), "")
	fail := func(e error) error { ep.Close(); ns.Destroy(); return e }
	sendBuffers := tcpip.TCPSendBufferSizeRangeOption{Min: 4096, Default: 65536, Max: 65536}
	receiveBuffers := tcpip.TCPReceiveBufferSizeRangeOption{Min: 4096, Default: 65536, Max: 65536}
	if e := ns.SetTransportProtocolOption(tcp.ProtocolNumber, &sendBuffers); e != nil {
		return fail(fmt.Errorf("local DNS TCP send buffers: %v", e))
	}
	if e := ns.SetTransportProtocolOption(tcp.ProtocolNumber, &receiveBuffers); e != nil {
		return fail(fmt.Errorf("local DNS TCP receive buffers: %v", e))
	}
	if e := ns.CreateNIC(1, ep); e != nil {
		return fail(fmt.Errorf("local DNS NIC: %v", e))
	}
	address := tcpip.AddrFromSlice(ip.AsSlice())
	if e := ns.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: address.WithPrefix()}, stack.AddressProperties{}); e != nil {
		return fail(fmt.Errorf("local DNS address: %v", e))
	}
	ns.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}})
	local := tcpip.FullAddress{NIC: 1, Addr: address, Port: 53}
	packet, e := gonet.DialUDP(ns, &local, nil, ipv4.ProtocolNumber)
	if e != nil {
		return fail(fmt.Errorf("local DNS UDP: %v", e))
	}
	listener, e := listenLocalTCP(ns, local)
	if e != nil {
		packet.Close()
		return fail(fmt.Errorf("local DNS TCP: %v", e))
	}
	s.mu.Lock()
	if s.closed || s.done != nil {
		s.mu.Unlock()
		packet.Close()
		listener.Close()
		return fail(fmt.Errorf("DNS server unavailable"))
	}
	s.requestSlots = make(chan struct{}, 64)
	err = s.startBoundLocked(packet, listener)
	s.mu.Unlock()
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	endpoint := &localEndpoint{ip: ip.As4(), stack: ns, ep: ep, cancel: cancel, done: make(chan struct{}), server: s}
	t.local = endpoint
	go func() {
		defer close(endpoint.done)
		for {
			pkt := ep.ReadContext(ctx)
			if pkt == nil {
				return
			}
			view := pkt.ToView()
			data := append([]byte(nil), view.AsSlice()...)
			view.Release()
			pkt.DecRef()
			if n, err := t.Write([][]byte{data}, 0); err != nil || n != 1 {
				s.Close()
				cancel()
				return
			}
		}
	}()
	for _, data := range t.pending {
		injectLocalDNS(ep, data)
	}
	t.pending = nil
	return nil
}

func injectLocalDNS(ep *channel.Endpoint, packet []byte) {
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte(nil), packet...))})
	ep.InjectInbound(ipv4.ProtocolNumber, pkt)
	pkt.DecRef()
}

// localDNSPacket checks only classification. The userspace stack validates IP,
// transport checksums, lengths and TCP state. Local fragments are kept in that
// stack for reassembly instead of accidentally sending them to a mesh peer.
func localDNSPacket(packet []byte, ip [4]byte) bool {
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return false
	}
	if packet[16] != ip[0] || packet[17] != ip[1] || packet[18] != ip[2] || packet[19] != ip[3] {
		return false
	}
	h := int(packet[0]&15) * 4
	if h < 20 || h > len(packet) {
		return false
	}
	if packet[9] != 6 && packet[9] != 17 {
		return false
	}
	if binary.BigEndian.Uint16(packet[6:8])&0x3fff != 0 {
		return true
	}
	return len(packet) >= h+4 && binary.BigEndian.Uint16(packet[h+2:h+4]) == 53
}

func (t *Tunnel) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	for {
		n, err := t.Device.Read(bufs, sizes, offset)
		if err != nil {
			return n, err
		}
		keep := 0
		for i := 0; i < n; i++ {
			packet := bufs[i][offset : offset+sizes[i]]
			t.mu.Lock()
			local := t.local
			// Never leak traffic for the reserved local endpoint to an exit
			// peer, including before activation or for malformed/non-DNS data.
			intercepted := localDNSDestination(packet)

			if intercepted && len(packet) > 9 && packet[9] == 17 {
				t.udpSeen.Store(true)
			}
			if intercepted && localDNSPacket(packet, [4]byte{198, 18, 0, 53}) {
				if local != nil {
					injectLocalDNS(local.ep, packet)
				} else if !t.closed && len(t.pending) < 32 && len(packet) <= 1280 {
					// Android netd can probe immediately after establish(),
					// before the first control map starts this resolver.
					// Retain only a small DNS startup window, never mesh data.
					t.pending = append(t.pending, append([]byte(nil), packet...))
				}
			}
			t.mu.Unlock()
			if !intercepted {
				if keep != i {
					copy(bufs[keep][offset:], packet)
					sizes[keep] = sizes[i]
				}
				keep++
			}
		}
		if keep > 0 {
			return keep, nil
		}
	}
}
func (t *Tunnel) Write(bufs [][]byte, offset int) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.Device.Write(bufs, offset)
}
func (t *Tunnel) Close() error {
	var err error
	t.once.Do(func() {
		t.mu.Lock()
		t.closed = true
		local := t.local
		t.local = nil
		t.pending = nil
		t.mu.Unlock()
		if local != nil {
			local.cancel()
			local.ep.Close()
		}
		// Unblock OS reads/writes before waiting on either DNS or the pump.
		err = t.Device.Close()
		if local != nil {
			local.stack.Close()
			local.server.Close()
			local.stack.Wait()
		}
		if local != nil {
			<-local.done
		}
	})
	return err
}

var _ tun.Device = (*Tunnel)(nil)

// Keep the listening backlog and active sessions small for this local service.
// gonet's general-purpose backlog is 4096; local DNS only needs 32.
func listenLocalTCP(ns *stack.Stack, address tcpip.FullAddress) (net.Listener, error) {
	var queue waiter.Queue
	endpoint, e := ns.NewEndpoint(tcp.ProtocolNumber, ipv4.ProtocolNumber, &queue)
	if e != nil {
		return nil, fmt.Errorf("local DNS TCP endpoint: %v", e)
	}
	if e = endpoint.Bind(address); e != nil {
		endpoint.Close()
		return nil, fmt.Errorf("local DNS TCP bind: %v", e)
	}
	if e = endpoint.Listen(32); e != nil {
		endpoint.Close()
		return nil, fmt.Errorf("local DNS TCP listen: %v", e)
	}
	return &limitedListener{Listener: gonet.NewTCPListener(ns, &queue, endpoint), slots: make(chan struct{}, 32)}, nil
}

type limitedListener struct {
	net.Listener
	slots chan struct{}
}

func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		c, e := l.Listener.Accept()
		if e != nil {
			return nil, e
		}
		select {
		case l.slots <- struct{}{}:
			return &limitedConn{Conn: c, release: func() { <-l.slots }}, nil
		default:
			c.Close()
		}
	}
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }
func localDNSDestination(p []byte) bool {
	return len(p) >= 20 && p[0]>>4 == 4 && p[16] == 198 && p[17] == 18 && p[18] == 0 && p[19] == 53
}

// Diagnostics contains only packet-path booleans, never names or addresses.
func (t *Tunnel) Diagnostics() map[string]bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := map[string]bool{"udp_seen": t.udpSeen.Load()}
	if t.local == nil {
		return result
	}
	stats := t.local.stack.Stats()
	result["udp_received"] = stats.UDP.PacketsReceived.Value() > 0
	result["udp_sent"] = stats.UDP.PacketsSent.Value() > 0
	result["udp_checksum_error"] = stats.UDP.ChecksumErrors.Value() > 0
	result["udp_malformed"] = stats.UDP.MalformedPacketsReceived.Value() > 0
	result["udp_unknown_port"] = stats.UDP.UnknownPortErrors.Value() > 0
	result["udp_buffer_error"] = stats.UDP.ReceiveBufferErrors.Value() > 0
	result["udp_send_error"] = stats.UDP.PacketSendErrors.Value() > 0
	result["ip_invalid_source"] = stats.IP.InvalidSourceAddressesReceived.Value() > 0
	result["ip_malformed"] = stats.IP.MalformedPacketsReceived.Value() > 0
	return result
}
