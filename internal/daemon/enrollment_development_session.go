//go:build mnet_enrollment_development

package daemon

// Explicit finite disposable network ownership. No normal Up/startup caller,
// OS-TUN fallback, reconnect, renewal, screen or input authority is added.
import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/netip"
	"sync"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/netmap"
	"golang.zx2c4.com/wireguard/tun"
)

type DevelopmentNetworkOptions struct {
	Limits controlclient.RegistrarLimits
	// Device must supply an already configured, disposable in-memory device.
	// The session owns/ closes the returned device. A nil device is refused.
	Device func(localIPv4 string) (tun.Device, error)
	Run    func(context.Context, *DevelopmentNetworkSession) error
}

type DevelopmentNetworkPeer struct {
	ID, IPv4, PublicKey, Name string
	HasEndpoints              bool
}

type DevelopmentNetworkSession struct {
	endpointPublication endpointPublicationState // session.mu; explicit publication only
	d                   *Daemon
	ctx                 context.Context
	cancel              context.CancelFunc
	limits              controlclient.RegistrarLimits
	done                chan struct{}
	selfIPv4            string
	revision            [32]byte
	resourceMu          sync.Mutex
	resources           []io.Closer
	peerScopes          []*DevelopmentPeerScope
	mu                  sync.Mutex // serializes explicit map refresh with session teardown
}

func validDevelopmentNetworkOptions(o *DevelopmentNetworkOptions) bool {
	return o != nil && o.Device != nil && o.Run != nil && o.Limits.Timeout > 0 && o.Limits.Timeout <= 3*time.Second && o.Limits.MaxConcurrent == 1 && o.Limits.MaxRequests >= 4 && o.Limits.MaxRequests <= 16
}

func (d *Daemon) runDevelopmentNetwork(ctx context.Context, m *netmap.Map, o *DevelopmentNetworkOptions) error {
	if !validDevelopmentNetworkOptions(o) || ctx == nil || ctx.Err() != nil || m == nil || !m.Self.Approved || m.Self.KeyExpiry == nil {
		return errEnrollmentContextStale
	}
	d.mu.Lock()
	client, rev, err := d.nativeClientLocked(o.Limits)
	if err == nil {
		err = d.checkNativeCurrentAfterLocked(ctx, rev)
	}
	var deadline time.Time
	if err == nil {
		deadline = time.UnixMilli(d.stateOwner.saved.native.Document.Receipt.AccessExpiresAtMS)
	}
	d.mu.Unlock()
	if err != nil || client == nil || !time.Now().Before(deadline) || !m.Self.KeyExpiry.Equal(deadline) || !validDevelopmentNetworkMap(m) {
		return errEnrollmentContextStale
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	s := &DevelopmentNetworkSession{d: d, ctx: bounded, cancel: cancel, limits: o.Limits, done: make(chan struct{}), revision: rev, selfIPv4: m.Self.IPv4}
	d.lifecycleMu.Lock()
	d.mu.Lock()
	err = d.checkNativeCurrentAfterLocked(bounded, rev)
	d.mu.Unlock()
	d.deviceMu.RLock()
	running := d.dev != nil
	d.deviceMu.RUnlock()
	if err != nil || running || d.cfg.TUN != nil || d.cfg.ManagedIface || d.cfg.LocalDNS {
		d.lifecycleMu.Unlock()
		cancel()
		return errEnrollmentContextStale
	}
	injected, err := o.Device(m.Self.IPv4)
	if err != nil || injected == nil {
		if injected != nil {
			_ = injected.Close()
		}
		d.lifecycleMu.Unlock()
		cancel()
		return fmt.Errorf("disposable network device unavailable")
	}
	d.cfg.TUN = injected // mandatory injection; startDevice must never create an OS TUN
	err = d.startDevice()
	if err == nil {
		d.deviceMu.RLock()
		port := d.bind.Port()
		d.deviceMu.RUnlock()
		err = d.nativeCurrentEndpoints(bounded, []string{fmt.Sprintf("127.0.0.1:%d", port)}, o.Limits)
	}
	if err == nil {
		err = s.refreshLocked()
	}
	if err != nil {
		d.stopDevice()
		d.lifecycleMu.Unlock()
		cancel()
		return err
	}
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	d.loops.Add(1)
	go func() { defer d.loops.Done(); d.expiryLoop(bounded) }()
	d.lifecycleMu.Unlock()
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-bounded.Done():
			case <-ticker.C:
				d.mu.Lock()
				valid := d.checkNativeCurrentAfterLocked(bounded, rev) == nil
				d.mu.Unlock()
				if valid {
					continue
				}
			}
			cancel()
			s.closeResources()
			s.mu.Lock()
			d.Down()
			s.mu.Unlock()
			return
		}
	}()
	defer s.Close()
	err = o.Run(bounded, s)
	if err == nil && bounded.Err() != nil {
		return bounded.Err()
	}
	return err
}

func (s *DevelopmentNetworkSession) Done() <-chan struct{} { return s.done }
func (s *DevelopmentNetworkSession) Close() {
	if s != nil {
		s.withdrawEndpointBeforeClose()
		s.cancel()
		<-s.done
	}
}

// Refresh is explicit and finite, not a background registrar poll/retry. On any
// refusal the entire session stops rather than retaining potentially stale peers.
func (s *DevelopmentNetworkSession) Refresh() error {
	s.mu.Lock()
	err := s.refreshLocked()
	s.mu.Unlock()
	if err != nil {
		s.Close()
	}
	return err
}
func (s *DevelopmentNetworkSession) refreshLocked() error {
	if s.ctx.Err() != nil {
		return errEnrollmentContextStale
	}
	m, err := s.d.nativeCurrentMap(s.ctx, s.limits)
	if err != nil {
		return err
	}
	if m.Self.IPv4 != s.selfIPv4 || !validDevelopmentNetworkMap(m) {
		return errEnrollmentContextStale
	}
	s.d.applyMap(m)
	s.d.mu.Lock()
	applyErr := s.d.lastErr
	s.d.mu.Unlock()
	if applyErr != "" {
		return fmt.Errorf("disposable network map application unavailable")
	}
	s.retireChangedPeerScopes()
	return nil
}

func (s *DevelopmentNetworkSession) Peers() []DevelopmentNetworkPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.mu.Lock()
	valid := s.d.checkNativeCurrentAfterLocked(s.ctx, s.revision) == nil
	s.d.mu.Unlock()
	if !valid {
		return nil
	}
	m := s.d.currentMap()
	if m == nil {
		return nil
	}
	m = unexpiredMap(m, time.Now())
	peers := make([]DevelopmentNetworkPeer, 0, len(m.Peers))
	for _, p := range m.Peers {
		if p.Approved {
			peers = append(peers, DevelopmentNetworkPeer{p.ID, p.IPv4, p.PubKey, p.Name, len(p.Endpoints) > 0})
		}
	}
	return peers
}

func developmentLoopbackEndpoint(raw string) bool {
	address, err := netip.ParseAddrPort(raw)
	return err == nil && address.Addr().Is4() && address.Addr().IsLoopback() && address.Port() != 0
}

// Own closes each registered listener/connection/protocol owner on stop. Packet
// denial alone is insufficient to unblock a TCP accept/read or session Mux.
func (s *DevelopmentNetworkSession) Own(resource io.Closer) error {
	if resource == nil {
		return errEnrollmentContextStale
	}
	s.resourceMu.Lock()
	defer s.resourceMu.Unlock()
	if s.ctx.Err() != nil {
		_ = resource.Close()
		return errEnrollmentContextStale
	}
	s.resources = append(s.resources, resource)
	return nil
}
func (s *DevelopmentNetworkSession) closeResources() {
	s.resourceMu.Lock()
	resources := s.resources
	s.resources = nil
	s.resourceMu.Unlock()
	for i := len(resources) - 1; i >= 0; i-- {
		_ = resources[i].Close()
	}
}

// CurrentSessionPeers is an explicit source check for a future disposable
// peer-bound consumer, not a cached membership grant. Budget exhaustion or
// registrar unavailability ends the network session and its registered owners.
func (s *DevelopmentNetworkSession) CurrentSessionPeers() ([]SessionPeer, error) {
	if err := s.Refresh(); err != nil {
		return nil, err
	}
	peers := s.Peers()
	out := make([]SessionPeer, 0, len(peers))
	for _, p := range peers {
		out = append(out, SessionPeer{Name: p.Name, IPv4: p.IPv4, PublicKey: p.PublicKey})
	}
	return out, nil
}

// DevelopmentPeerScope binds consumer lifetime to one exact currently approved
// peer. A later explicit map check removing/changing that peer cancels the
// context and closes its registered resources, even if the network stays up.
type DevelopmentPeerScope struct {
	endpointPin  *developmentEndpointPin // immutable; assigned and consumed under session.mu
	ctx          context.Context
	cancel       context.CancelFunc
	peer         SessionPeer
	peerDeadline time.Time
	reason       string
	done         chan struct{}
	mu           sync.Mutex
	resources    []io.Closer
}

func (p *DevelopmentPeerScope) Context() context.Context { return p.ctx }
func (p *DevelopmentPeerScope) Own(resource io.Closer) error {
	if resource == nil {
		return errEnrollmentContextStale
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		_ = resource.Close()
		return errEnrollmentContextStale
	}
	p.resources = append(p.resources, resource)
	return nil
}
func (p *DevelopmentPeerScope) Close() error {
	p.cancel()
	if p.done != nil {
		<-p.done
	} else {
		p.closeResources()
	}
	return nil
}
func (p *DevelopmentPeerScope) closeResources() {
	p.mu.Lock()
	resources := p.resources
	p.resources = nil
	if p.reason == "" {
		if !p.peerDeadline.IsZero() && !time.Now().Before(p.peerDeadline) {
			p.reason = "peer-expired"
		} else {
			p.reason = "session-ended"
		}
	}
	p.mu.Unlock()
	for i := len(resources) - 1; i >= 0; i-- {
		_ = resources[i].Close()
	}
}
func (p *DevelopmentPeerScope) Reason() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Context cancellation is observable before the cleanup goroutine runs.
	if p.reason == "" && p.ctx.Err() != nil {
		if !p.peerDeadline.IsZero() && !time.Now().Before(p.peerDeadline) {
			p.reason = "peer-expired"
		} else {
			p.reason = "session-ended"
		}
	}
	return p.reason
}

func (s *DevelopmentNetworkSession) BindPeer(peer SessionPeer) (*DevelopmentPeerScope, error) {
	peers, err := s.CurrentSessionPeers()
	if err != nil {
		return nil, err
	}
	if !developmentExactPeerPresent(peers, peer) {
		return nil, errEnrollmentContextStale
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return nil, errEnrollmentContextStale
	}
	// Recheck the installed map under the same refresh lock before publishing.
	if !developmentExactPeerPresent(s.sessionPeerSnapshot(), peer) {
		return nil, errEnrollmentContextStale
	}
	var peerDeadline time.Time
	if m := s.d.currentMap(); m != nil {
		for _, node := range m.Peers {
			if node.PubKey == peer.PublicKey && node.IPv4 == peer.IPv4 && node.Name == peer.Name && node.KeyExpiry != nil {
				peerDeadline = *node.KeyExpiry
			}
		}
	}
	if peerDeadline.IsZero() || !time.Now().Before(peerDeadline) {
		return nil, errEnrollmentContextStale
	}
	scope := newDevelopmentPeerScope(s.ctx, peer, peerDeadline)
	cancel := scope.cancel
	if err := s.Own(scope); err != nil {
		cancel()
		return nil, err
	}
	s.resourceMu.Lock()
	s.peerScopes = append(s.peerScopes, scope)
	s.resourceMu.Unlock()
	return scope, nil
}
func developmentExactPeerPresent(peers []SessionPeer, pin SessionPeer) bool {
	if pin.Name == "" || pin.IPv4 == "" || pin.PublicKey == "" {
		return false
	}
	matches := 0
	for _, peer := range peers {
		if peer == pin {
			matches++
		} else if peer.PublicKey == pin.PublicKey || peer.IPv4 == pin.IPv4 {
			return false
		}
	}
	return matches == 1
}
func (s *DevelopmentNetworkSession) sessionPeerSnapshot() []SessionPeer {
	m := s.d.currentMap()
	if m == nil {
		return nil
	}
	m = unexpiredMap(m, time.Now())
	if !m.Self.Approved || expiredAt(m.Self, time.Now()) {
		return nil
	}
	out := make([]SessionPeer, 0, len(m.Peers))
	for _, p := range m.Peers {
		if p.Approved {
			out = append(out, SessionPeer{Name: p.Name, IPv4: p.IPv4, PublicKey: p.PubKey})
		}
	}
	return out
}
func (s *DevelopmentNetworkSession) retireChangedPeerScopes() {
	peers := s.sessionPeerSnapshot()
	s.resourceMu.Lock()
	scopes := append([]*DevelopmentPeerScope(nil), s.peerScopes...)
	s.resourceMu.Unlock()
	for _, scope := range scopes {
		sameDeadline := false
		if m := s.d.currentMap(); m != nil {
			for _, peer := range m.Peers {
				if peer.PubKey == scope.peer.PublicKey && peer.IPv4 == scope.peer.IPv4 && peer.Name == scope.peer.Name && peer.KeyExpiry != nil && peer.KeyExpiry.Equal(scope.peerDeadline) {
					sameDeadline = true
				}
			}
		}
		if !developmentExactPeerPresent(peers, scope.peer) || !sameDeadline || (scope.endpointPin != nil && !scope.endpointPin.current(s.d.currentMap())) {
			scope.mu.Lock()
			if scope.reason == "" {
				if !time.Now().Before(scope.peerDeadline) {
					scope.reason = "peer-expired"
				} else {
					scope.reason = "authorization-changed"
				}
			}
			scope.mu.Unlock()
			_ = scope.Close()
		}
	}
}

// Validate all values before they reach IPC configuration, source selection or
// netstack parsing. A valid current credential does not make arbitrary map
// strings safe, and duplicated key/address identity is not a selection choice.
func validDevelopmentNetworkMap(m *netmap.Map) bool {
	if m == nil || m.Network.ID == "" || m.Self.ID == "" || m.Self.NetworkID != m.Network.ID || m.RelayURL != "" || !m.Self.Approved || m.Self.ExitNode || len(m.Self.Routes) != 0 || m.Self.KeyExpiry == nil {
		return false
	}
	validIP := func(raw string) bool {
		ip, err := netip.ParseAddr(raw)
		return err == nil && ip.Is4() && ip.String() == raw && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLoopback()
	}
	validKey := func(raw string) bool {
		key, err := base64.StdEncoding.Strict().DecodeString(raw)
		return err == nil && len(key) == 32 && base64.StdEncoding.EncodeToString(key) == raw
	}
	if !validIP(m.Self.IPv4) || !validKey(m.Self.PubKey) {
		return false
	}
	keys := map[string]bool{m.Self.PubKey: true}
	ips := map[string]bool{m.Self.IPv4: true}
	ids := map[string]bool{m.Self.ID: true}
	for _, p := range m.Peers {
		if p.ID == "" || p.NetworkID != m.Network.ID || keys[p.PubKey] || ips[p.IPv4] || ids[p.ID] || !validKey(p.PubKey) || !validIP(p.IPv4) || !p.Approved || p.KeyExpiry == nil || p.ExitNode || len(p.Routes) != 0 {
			return false
		}
		keys[p.PubKey] = true
		ips[p.IPv4] = true
		ids[p.ID] = true
		for _, endpoint := range p.Endpoints {
			if !developmentLoopbackEndpoint(endpoint) {
				return false
			}
		}
	}
	return true
}

func newDevelopmentPeerScope(parent context.Context, peer SessionPeer, deadline time.Time) *DevelopmentPeerScope {
	ctx, cancel := context.WithDeadline(parent, deadline)
	scope := &DevelopmentPeerScope{ctx: ctx, cancel: cancel, peer: peer, peerDeadline: deadline, done: make(chan struct{})}
	go func() { defer close(scope.done); <-ctx.Done(); scope.closeResources() }()
	return scope
}
