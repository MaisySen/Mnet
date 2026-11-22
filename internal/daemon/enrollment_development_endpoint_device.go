//go:build mnet_enrollment_development

package daemon

// Private source factory for the resident endpoint mode. The generic injected
// Device callback is never accepted as plaintext provenance by this owner.
import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/netmap"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

const developmentEndpointQueue = 16

var errPrivateEndpoint = errors.New("private-endpoint-device-owner-unavailable")

type developmentEndpointNotifier interface {
	File() *os.File
	Signal() error
	Drain() error
	Close() error
}
type developmentEndpointFactory struct{ used atomic.Bool }

// Selects the private factory only. Does not enroll, start a device, open an OS
// TUN, approve a peer or authorize a registrar operation. Existing callers of
// Device remain unchanged and cannot obtain a private endpoint bridge owner.
func PrivateEndpointNetworkOptions(limits controlclient.RegistrarLimits, run func(context.Context, *DevelopmentNetworkSession) error) DevelopmentNetworkOptions {
	return DevelopmentNetworkOptions{Limits: limits, Run: run, endpointFactory: &developmentEndpointFactory{}}
}

type developmentEndpointDevice struct {
	session    *DevelopmentNetworkSession
	tun        *developmentEndpointTun
	mu         sync.Mutex
	dev        *device.Device
	generation uint64
	bridge     *DevelopmentEndpointBridge
	closed     bool
}
type DevelopmentEndpointBridgeSnapshot struct {
	Generation                     uint64
	ConfigDigest                   [32]byte // non-secret canonical public pair/config correlation
	OwnKey, PeerKey, OwnIP, PeerIP string
}
type DevelopmentEndpointBridge struct {
	slot      *developmentEndpointDevice
	peer      *DevelopmentEndpointPeer
	pin       developmentEndpointPin
	ctx       context.Context
	cancel    context.CancelFunc
	snapshot  DevelopmentEndpointBridgeSnapshot
	closeOnce sync.Once
	closeErr  error
}
type DevelopmentEndpointBridgeCounts struct{ QueuedToPeer, QueuedFromPeer, Dropped, Backpressure uint64 }
type developmentEndpointTun struct {
	mu       sync.Mutex
	closed   bool
	done     chan struct{}
	events   chan tun.Event
	toPeer   chan []byte
	fromPeer [][]byte
	notify   developmentEndpointNotifier
	grant    *DevelopmentEndpointBridge
	counts   DevelopmentEndpointBridgeCounts
	once     sync.Once
	closeErr error
}

func privateEndpointCount(v *uint64) {
	if *v != ^uint64(0) {
		*v++
	}
}
func (f *developmentEndpointFactory) createSource(s *DevelopmentNetworkSession, ip string) (*developmentEndpointDevice, error) {
	if f == nil || s == nil || s.ctx == nil || s.ctx.Err() != nil || !f.used.CompareAndSwap(false, true) {
		return nil, errPrivateEndpoint
	}
	a, e := netip.ParseAddr(ip)
	if e != nil || !a.Is4() || !netip.MustParsePrefix("100.64.0.0/10").Contains(a) {
		return nil, errPrivateEndpoint
	}
	notify, e := newDevelopmentEndpointNotifier()
	if e != nil {
		return nil, e
	}
	t := &developmentEndpointTun{done: make(chan struct{}), events: make(chan tun.Event, 1), toPeer: make(chan []byte, developmentEndpointQueue), notify: notify}
	t.events <- tun.EventUp
	return &developmentEndpointDevice{session: s, tun: t}, nil
}
func (s *developmentEndpointDevice) attachSource() error {
	d := s.session.d
	d.deviceMu.RLock()
	defer d.deviceMu.RUnlock()
	if d.dev == nil || d.tun != s.tun {
		return errPrivateEndpoint
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.dev != nil {
		return errPrivateEndpoint
	}
	s.dev = d.dev
	s.generation = 1
	return nil
}

// applyMu is already held. This branch never forwards a map into generic IPC.
// Before a bridge it installs metadata only, so the real WG device has NO
// peers. After sealing it permits no config writes, even for path refresh.
func (s *developmentEndpointDevice) applyEndpointMapLocked(d *Daemon, m *netmap.Map) {
	if m != nil {
		m = unexpiredMap(m, time.Now())
	}
	s.mu.Lock()
	b := s.bridge
	bad := s.closed || s.session.d != d || !validDevelopmentNetworkMap(m)
	if b != nil && !b.pin.current(m) {
		bad = true
	}
	s.mu.Unlock()
	if bad {
		d.setErr(errPrivateEndpoint)
		s.revokeEndpointDeviceLocked()
	}
	d.mu.Lock()
	d.lastMap = m
	d.mu.Unlock()
}
func (s *developmentEndpointDevice) revokeEndpointDeviceLocked() {
	s.mu.Lock()
	s.closed = true
	b := s.bridge
	retained := s.dev
	s.mu.Unlock()
	if b != nil {
		_ = b.Close()
		return
	}
	_ = s.tun.Close()
	if retained != nil {
		retained.Close()
	}
}

// Only the actual factory-created device may issue this owner. The source pins
// the real dev/tun pointers, a successful one-peer IPC write/readback and the
// actual current native peer. No constructor accepts a tun.Device or callback.
func (p *DevelopmentEndpointPeer) ClaimPrivateEndpointBridge() (*DevelopmentEndpointBridge, error) {
	self, other, e := p.Snapshot()
	if e != nil {
		return nil, e
	}
	session := p.session
	s := session.endpointSlot
	if s == nil {
		return nil, errPrivateEndpoint
	}
	d := session.d
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	d.deviceMu.RLock()
	defer d.deviceMu.RUnlock()
	d.mu.Lock()
	e = d.checkNativeCurrentAfterLocked(session.ctx, session.revision)
	privateValue := d.state.PrivateKey
	d.mu.Unlock()
	pin := developmentEndpointPin{self: self, peer: other}
	if e != nil || p.Context().Err() != nil || d.endpointDeviceMap != s || d.dev == nil || d.tun != s.tun || !pin.current(d.currentMap()) {
		return nil, errPrivateEndpoint
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.bridge != nil || s.dev != d.dev || s.generation != 1 {
		return nil, errPrivateEndpoint
	}
	var target *netmap.Node
	for _, n := range d.currentMap().Peers {
		if n.ID == other.NodeID {
			copy := n
			target = &copy
		}
	}
	if target == nil || len(target.Endpoints) == 0 || !developmentLoopbackEndpoint(target.Endpoints[0]) {
		return nil, errPrivateEndpoint
	}
	key, e := base64DecodeKey(other.PublicKey)
	if e != nil {
		return nil, e
	}
	// Source owns all IPC writers: generic applyMap is excluded by the private
	// slot, and no dev/tun pointer leaves daemon. replace_peers also removes any
	// unexpected old configuration before readback; unbound TUN drops all data.
	conf := "replace_peers=true\npublic_key=" + hex.EncodeToString(key) + "\nreplace_allowed_ips=true\nallowed_ip=" + other.IPv4 + "/32\npersistent_keepalive_interval=25\nendpoint=" + target.Endpoints[0] + "\n"
	if e = d.dev.IpcSetOperation(strings.NewReader(conf)); e != nil {
		s.closed = true
		_ = s.tun.Close()
		return nil, e
	}
	sealed := false
	defer func() {
		if !sealed {
			s.closed = true
			_ = s.tun.Close()
		}
	}()
	var readback bytes.Buffer
	if e = d.dev.IpcGetOperation(&readback); e != nil {
		s.closed = true
		_ = s.tun.Close()
		return nil, e
	}
	private, e := base64DecodeKey(privateValue)
	if e != nil {
		return nil, e
	}
	own, e := ecdh.X25519().NewPrivateKey(private)
	if e != nil {
		return nil, e
	}
	wantOwn, e := base64DecodeKey(self.PublicKey)
	if e != nil || !bytes.Equal(own.PublicKey().Bytes(), wantOwn) {
		return nil, errPrivateEndpoint
	}
	if !privateEndpointConfigMatches(readback.String(), hex.EncodeToString(private), hex.EncodeToString(key), other.IPv4) {
		s.closed = true
		_ = s.tun.Close()
		return nil, errPrivateEndpoint
	}
	d.mu.Lock()
	e = d.checkNativeCurrentAfterLocked(session.ctx, session.revision)
	d.mu.Unlock()
	if e != nil || p.Context().Err() != nil || !pin.current(d.currentMap()) {
		return nil, errPrivateEndpoint
	}
	// Readback (including private_key) stays in this function, never in a public
	// snapshot, wire body, report or error. Digest contains only public fields.
	s.generation++
	ctx, cancel := context.WithCancel(p.Context())
	snap := DevelopmentEndpointBridgeSnapshot{Generation: s.generation, OwnKey: self.PublicKey, PeerKey: other.PublicKey, OwnIP: self.IPv4, PeerIP: other.IPv4}
	snap.ConfigDigest = sha256.Sum256([]byte(fmt.Sprintf("mnet-private-endpoint-v1\n%d\n%s\n%s\n%s\n%s/32\n", snap.Generation, snap.OwnKey, snap.PeerKey, snap.OwnIP, snap.PeerIP)))
	b := &DevelopmentEndpointBridge{slot: s, peer: p, pin: pin, ctx: ctx, cancel: cancel, snapshot: snap}
	s.bridge = b
	s.tun.mu.Lock()
	s.tun.grant = b
	s.tun.mu.Unlock()
	// Scope registration is already finite. Avoid scope.Own while any daemon
	// lock is held: the cancellation worker closes this owner via context below.
	context.AfterFunc(p.Context(), func() { _ = b.Close() })
	sealed = true
	return b, nil
}
func privateEndpointConfigMatches(raw, private, peer, ip string) bool {
	peers, ips, privates := 0, 0, 0
	for _, line := range strings.Split(raw, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "private_key":
			privates++
			if v != private {
				return false
			}
		case "public_key":
			peers++
			if v != peer {
				return false
			}
		case "allowed_ip":
			ips++
			if peers != 1 || v != ip+"/32" {
				return false
			}
		case "fwmark":
			if v != "0" {
				return false
			}
		}
	}
	return peers == 1 && ips == 1 && privates == 1
}
func (b *DevelopmentEndpointBridge) Current(p *DevelopmentEndpointPeer) (DevelopmentEndpointBridgeSnapshot, error) {
	var zero DevelopmentEndpointBridgeSnapshot
	if b == nil || b.slot == nil || b.peer != p || b.ctx == nil || b.ctx.Err() != nil {
		return zero, errPrivateEndpoint
	}
	self, other, e := p.Snapshot()
	if e != nil || self != b.pin.self || other != b.pin.peer {
		return zero, errPrivateEndpoint
	}
	d := b.slot.session.d
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	d.deviceMu.RLock()
	defer d.deviceMu.RUnlock()
	b.slot.mu.Lock()
	defer b.slot.mu.Unlock()
	d.mu.Lock()
	e = d.checkNativeCurrentAfterLocked(b.ctx, b.slot.session.revision)
	d.mu.Unlock()
	if e != nil {
		return zero, errPrivateEndpoint
	}
	if b.ctx.Err() != nil || !b.pin.current(d.currentMap()) || b.slot.closed || b.slot.bridge != b || b.slot.generation != b.snapshot.Generation || d.endpointDeviceMap != b.slot || d.dev != b.slot.dev || d.tun != b.slot.tun {
		return zero, errPrivateEndpoint
	}
	return b.snapshot, nil
}
func (b *DevelopmentEndpointBridge) Context() context.Context {
	if b == nil {
		return nil
	}
	return b.ctx
}

// WakeFile is only an unprivileged event notifier, NOT a TUN or WG descriptor.
// Borrow it for poll only; queue methods own notification draining and payloads.
func (b *DevelopmentEndpointBridge) WakeFile() *os.File {
	if b == nil || b.slot == nil {
		return nil
	}
	return b.slot.tun.notify.File()
}
func (b *DevelopmentEndpointBridge) SignalWake() error {
	if b == nil || b.slot == nil {
		return errPrivateEndpoint
	}
	return b.slot.tun.notify.Signal()
}
func (b *DevelopmentEndpointBridge) Close() error {
	if b == nil {
		return nil
	}
	b.cancel()
	b.closeOnce.Do(func() {
		b.closeErr = b.slot.tun.Close()
		b.slot.mu.Lock()
		b.slot.closed = true
		retained := b.slot.dev
		b.slot.mu.Unlock()
		// Dispose the actual retained device even if daemon's current pointer was
		// replaced. Never close a different current device as this bridge's owner.
		// Slot/queue mutexes are released before WG stops its workers and bind.
		// A map caller can still own applyMu. WG TUN paths never acquire it;
		// the bound STUN callback uses only stunMu, not map/controller locks.
		if retained != nil {
			retained.Close()
		}
	})
	return b.closeErr
}
func (b *DevelopmentEndpointBridge) SendToPeer(p *DevelopmentEndpointPeer, packet []byte) error {
	if _, e := b.Current(p); e != nil {
		return e
	}
	if !privateEndpointPacket(packet, b.snapshot.OwnIP, b.snapshot.PeerIP) {
		return errPrivateEndpoint
	}
	payload := append([]byte(nil), packet...)
	select {
	case <-b.ctx.Done():
		return b.ctx.Err()
	case b.slot.tun.toPeer <- payload:
		return nil
	default:
		return ErrDevelopmentEndpointBackpressure
	}
}

var ErrDevelopmentEndpointBackpressure = errors.New("private-endpoint-queue-full")

func (b *DevelopmentEndpointBridge) TakeFromPeer(p *DevelopmentEndpointPeer) ([]byte, bool, error) {
	if _, e := b.Current(p); e != nil {
		return nil, false, e
	}
	t := b.slot.tun
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, false, io.EOF
	}
	// Drain the nonblocking event counter before examining the queue. A producer
	// signals AFTER unlocking; a racing late signal is just a harmless extra wake.
	if e := t.notify.Drain(); e != nil {
		t.mu.Unlock()
		return nil, false, e
	}
	if len(t.fromPeer) == 0 {
		t.mu.Unlock()
		return nil, false, nil
	}
	packet := t.fromPeer[0]
	t.fromPeer[0] = nil
	t.fromPeer = t.fromPeer[1:]
	more := len(t.fromPeer) > 0
	t.mu.Unlock()
	if more {
		if e := t.notify.Signal(); e != nil {
			return nil, false, e
		}
	}
	return packet, true, nil
}
func (b *DevelopmentEndpointBridge) Counts() DevelopmentEndpointBridgeCounts {
	if b == nil || b.slot == nil {
		return DevelopmentEndpointBridgeCounts{}
	}
	t := b.slot.tun
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts
}
func privateEndpointPacket(p []byte, src, dst string) bool {
	a, e := netip.ParseAddr(src)
	if e != nil || !a.Is4() {
		return false
	}
	z, e := netip.ParseAddr(dst)
	if e != nil || !z.Is4() {
		return false
	}
	return len(p) >= 20 && len(p) <= MTU && p[0] == 0x45 && int(binary.BigEndian.Uint16(p[2:])) == len(p) && [4]byte(p[12:16]) == a.As4() && [4]byte(p[16:20]) == z.As4()
}
func (t *developmentEndpointTun) File() *os.File           { return nil }
func (t *developmentEndpointTun) Name() (string, error)    { return "private-resident-endpoint", nil }
func (t *developmentEndpointTun) MTU() (int, error)        { return MTU, nil }
func (t *developmentEndpointTun) BatchSize() int           { return 1 }
func (t *developmentEndpointTun) Events() <-chan tun.Event { return t.events }
func (t *developmentEndpointTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if offset < 0 || len(bufs) == 0 || len(sizes) < len(bufs) {
		return 0, io.ErrShortBuffer
	}
	select {
	case <-t.done:
		return 0, io.EOF
	case packet := <-t.toPeer:
		t.mu.Lock()
		b := t.grant
		closed := t.closed
		t.mu.Unlock()
		if closed || b == nil || b.ctx.Err() != nil {
			return 0, io.EOF
		}
		if !privateEndpointPacket(packet, b.snapshot.OwnIP, b.snapshot.PeerIP) || offset > len(bufs[0]) || len(bufs[0])-offset < len(packet) {
			return 0, io.ErrShortBuffer
		}
		copy(bufs[0][offset:], packet)
		sizes[0] = len(packet)
		t.mu.Lock()
		privateEndpointCount(&t.counts.QueuedToPeer)
		t.mu.Unlock()
		return 1, nil
	}
}

// Only the retained WireGuard receive worker has this private tun pointer.
// Pinned WG receive.go checks authenticated peer+AllowedIPs BEFORE this Write.
// No daemon public method exposes this Write or injects a fromPeer packet.
func (t *developmentEndpointTun) Write(bufs [][]byte, offset int) (int, error) {
	if offset < 0 || len(bufs) > 128 {
		return 0, io.ErrShortBuffer
	}
	for _, buf := range bufs {
		t.mu.Lock()
		b := t.grant
		if t.closed {
			t.mu.Unlock()
			return 0, io.EOF
		}
		if b == nil || b.ctx.Err() != nil || offset > len(buf) || !privateEndpointPacket(buf[offset:], b.snapshot.PeerIP, b.snapshot.OwnIP) {
			privateEndpointCount(&t.counts.Dropped)
			t.mu.Unlock()
			continue
		}
		if len(t.fromPeer) >= developmentEndpointQueue {
			privateEndpointCount(&t.counts.Backpressure)
			privateEndpointCount(&t.counts.Dropped)
			t.mu.Unlock()
			continue
		}
		t.fromPeer = append(t.fromPeer, append([]byte(nil), buf[offset:]...))
		privateEndpointCount(&t.counts.QueuedFromPeer)
		t.mu.Unlock()
		// Notification is nonblocking, has no payload, and no application mutex is
		// held across it. WG workers never write the resident bridge or original FD.
		if e := t.notify.Signal(); e != nil {
			_ = t.Close()
			return 0, e
		}
	}
	return len(bufs), nil
}
func (t *developmentEndpointTun) Close() error {
	t.once.Do(func() {
		t.mu.Lock()
		t.closed = true
		t.fromPeer = nil
		b := t.grant
		close(t.done)
		close(t.events)
		t.mu.Unlock()
		if b != nil {
			b.cancel()
		}
		t.closeErr = t.notify.Close()
	})
	return t.closeErr
}

var _ tun.Device = (*developmentEndpointTun)(nil)
