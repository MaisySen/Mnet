// Package bind is the WireGuard socket for Mnet: real UDP plus a relay
// path through the control server, in one conn.Bind. Peers roam between
// direct and relayed paths the same way WireGuard roams between networks.
// STUN answers on the same socket are intercepted and handed to the
// daemon, so endpoint discovery and WireGuard share one NAT mapping.
package bind

import (
	"encoding/base64"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
)

// RelaySender ships one already-encrypted WireGuard packet to a peer
// pubkey through the relay.
type RelaySender func(dstPubkey string, pkt []byte) error

// StunHandler gets every STUN response seen on the WireGuard socket.
type StunHandler func(msg []byte)

// RelayEndpoint addresses a peer through the relay by its public key.
type RelayEndpoint struct {
	Pubkey string
	raw    []byte
}

func (e *RelayEndpoint) ClearSrc()           {}
func (e *RelayEndpoint) SrcToString() string { return "relay" }
func (e *RelayEndpoint) DstToString() string { return "relay:" + e.Pubkey }
func (e *RelayEndpoint) DstToBytes() []byte  { return e.raw }
func (e *RelayEndpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (e *RelayEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }

// RelayPrefix marks an endpoint string as relayed: relay:<base64 pubkey>.
const RelayPrefix = "relay:"

// IsStunMessage reports whether a packet on our socket is a STUN response
// rather than a WireGuard message.
func IsStunMessage(msg []byte) bool {
	// STUN: top two bits zero, magic cookie at bytes 4..8.
	// WireGuard: first byte 1..4 with three reserved zero bytes.
	return len(msg) >= 20 && msg[0]&0xC0 == 0 &&
		msg[4] == 0x21 && msg[5] == 0x12 && msg[6] == 0xa4 && msg[7] == 0x42
}

// Bind multiplexes UDP and relay transport.
type Bind struct {
	mu     sync.Mutex
	udp    *net.UDPConn
	send   RelaySender
	stun   StunHandler
	recv   chan relayPacket
	done   chan struct{}
	closed bool
	port   uint16
}

type relayPacket struct {
	data []byte
	ep   *RelayEndpoint
}

var _ conn.Bind = (*Bind)(nil)

// New makes a bind. Open it before handing it to the device.
func New() *Bind {
	return &Bind{closed: true}
}

// SetRelaySender installs the relay sender.
func (b *Bind) SetRelaySender(s RelaySender) {
	b.mu.Lock()
	b.send = s
	b.mu.Unlock()
}

// SetStunHandler installs the STUN response hook.
func (b *Bind) SetStunHandler(h StunHandler) {
	b.mu.Lock()
	b.stun = h
	b.mu.Unlock()
}

// Inject delivers a packet that arrived from the relay.
func (b *Bind) Inject(srcPubkey string, pkt []byte) {
	raw, err := decodeKey(srcPubkey)
	if err != nil {
		return
	}
	cp := make([]byte, len(pkt))
	copy(cp, pkt)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.udp == nil {
		return
	}
	select {
	case b.recv <- relayPacket{data: cp, ep: &RelayEndpoint{Pubkey: srcPubkey, raw: raw}}:
	default:
		// Full queue: drop, like UDP under load. Admission and queue selection
		// share Close/Open's lock, so a packet cannot seed a later generation.
	}
}

func decodeKey(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil, errors.New("bad wireguard key")
	}
	return b, nil
}

// Open starts the UDP socket and returns the receive functions.
// wireguard-go closes and reopens the bind on Up and on listen port
// changes, so Open must work again after Close: reset the lifecycle.
func (b *Bind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.udp != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}
	u, err := net.ListenUDP("udp4", &net.UDPAddr{Port: int(port)})
	if err != nil {
		return nil, 0, err
	}
	// Every Open owns distinct sockets, cancellation and relay buffering.
	// Returned callbacks capture that generation, never mutable b.udp/b.recv.
	done := make(chan struct{})
	recv := make(chan relayPacket, 256)
	b.closed, b.done, b.recv, b.udp = false, done, recv, u
	b.port = uint16(u.LocalAddr().(*net.UDPAddr).Port)
	return []conn.ReceiveFunc{
		func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
			return b.receiveUDP(u, done, packets, sizes, eps)
		},
		func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
			return receiveRelay(recv, done, packets, sizes, eps)
		},
	}, b.port, nil
}

// Port is the bound UDP port, for endpoint reporting and STUN.
func (b *Bind) Port() uint16 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.port
}

// WriteStun sends a STUN request from the WireGuard socket.
func (b *Bind) WriteStun(msg []byte, server string) error {
	b.mu.Lock()
	u := b.udp
	b.mu.Unlock()
	if u == nil {
		return errors.New("bind not open")
	}
	raddr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return err
	}
	_, err = u.WriteToUDP(msg, raddr)
	return err
}

func (b *Bind) receiveUDP(u *net.UDPConn, done <-chan struct{}, packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
	if len(packets) == 0 {
		return 0, nil
	}
	for {
		select {
		case <-done:
			return 0, net.ErrClosed
		default:
		}
		_ = u.SetReadDeadline(time.Now().Add(time.Second))
		n, src, err := u.ReadFromUDP(packets[0])
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			select {
			case <-done:
				return 0, net.ErrClosed
			default:
				return 0, err
			}
		}
		select {
		case <-done:
			return 0, net.ErrClosed
		default:
		}
		msg := packets[0][:n]
		if IsStunMessage(msg) {
			b.mu.Lock()
			h := b.stun
			b.mu.Unlock()
			if h != nil {
				cp := make([]byte, n)
				copy(cp, msg)
				go h(cp)
			}
			continue
		}
		sizes[0] = n
		eps[0] = &conn.StdNetEndpoint{AddrPort: src.AddrPort()}
		return 1, nil
	}
}

func receiveRelay(recv <-chan relayPacket, done <-chan struct{}, packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
	select {
	case <-done:
		return 0, net.ErrClosed
	default:
	}
	select {
	case p := <-recv:
		// Cancellation wins over a buffered old-generation packet.
		select {
		case <-done:
			return 0, net.ErrClosed
		default:
		}
		if len(packets) == 0 {
			return 0, nil
		}
		n := copy(packets[0], p.data)
		sizes[0] = n
		eps[0] = p.ep
		return 1, nil
	case <-done:
		return 0, net.ErrClosed
	}
}

// Close shuts everything down.
func (b *Bind) Close() error {
	b.mu.Lock()
	already := b.closed
	if !already {
		b.closed = true
		close(b.done)
	}
	u := b.udp
	b.udp = nil
	b.port = 0
	b.mu.Unlock()
	if already {
		return nil
	}
	if u != nil {
		return u.Close()
	}
	return nil
}

// SetMark is a no-op for the plain socket.
func (b *Bind) SetMark(mark uint32) error { return nil }

// Send routes by endpoint type.
func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) error {
	for _, buf := range bufs {
		var err error
		switch e := ep.(type) {
		case *RelayEndpoint:
			b.mu.Lock()
			send := b.send
			closed := b.closed
			b.mu.Unlock()
			if closed {
				return net.ErrClosed
			}
			if send == nil {
				return errors.New("relay not connected")
			}
			err = send(e.Pubkey, buf)
		case *conn.StdNetEndpoint:
			b.mu.Lock()
			u := b.udp
			closed := b.closed
			b.mu.Unlock()
			if u == nil || closed {
				return net.ErrClosed
			}
			_, err = u.WriteTo(buf, net.UDPAddrFromAddrPort(e.AddrPort))
		default:
			return conn.ErrWrongEndpointType
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ParseEndpoint accepts host:port for UDP or relay:<pubkey>.
func (b *Bind) ParseEndpoint(s string) (conn.Endpoint, error) {
	if strings.HasPrefix(s, RelayPrefix) {
		pk := strings.TrimPrefix(s, RelayPrefix)
		raw, err := decodeKey(pk)
		if err != nil {
			return nil, err
		}
		return &RelayEndpoint{Pubkey: pk, raw: raw}, nil
	}
	ua, err := net.ResolveUDPAddr("udp", s)
	if err != nil {
		return nil, err
	}
	return &conn.StdNetEndpoint{AddrPort: ua.AddrPort()}, nil
}

// BatchSize is one: every receive call returns a single packet.
func (b *Bind) BatchSize() int { return 1 }
