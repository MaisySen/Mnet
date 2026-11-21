//go:build mnet_enrollment_development

package daemon

// Explicit finite disposable network ownership. No normal Up/startup caller,
// OS-TUN fallback, reconnect, renewal, screen or input authority is added.
import (
	"context"
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

type DevelopmentNetworkPeer struct{ ID, IPv4, PublicKey, Name string }

type DevelopmentNetworkSession struct {
	d          *Daemon
	ctx        context.Context
	cancel     context.CancelFunc
	limits     controlclient.RegistrarLimits
	done       chan struct{}
	revision   [32]byte
	resourceMu sync.Mutex
	resources  []io.Closer
	mu         sync.Mutex // serializes explicit map refresh with session teardown
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
	if err != nil || client == nil || !time.Now().Before(deadline) || !m.Self.KeyExpiry.Equal(deadline) {
		return errEnrollmentContextStale
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	s := &DevelopmentNetworkSession{d: d, ctx: bounded, cancel: cancel, limits: o.Limits, done: make(chan struct{}), revision: rev}
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
	// Network-only disposable proof never accepts routes/exit nodes or nonlocal
	// UDP destinations from an otherwise valid map.
	for _, p := range m.Peers {
		if p.ExitNode || len(p.Routes) != 0 {
			return errEnrollmentContextStale
		}
		for _, endpoint := range p.Endpoints {
			if !developmentLoopbackEndpoint(endpoint) {
				return errEnrollmentContextStale
			}
		}
	}
	s.d.applyMap(m)
	s.d.mu.Lock()
	applyErr := s.d.lastErr
	s.d.mu.Unlock()
	if applyErr != "" {
		return fmt.Errorf("disposable network map application unavailable")
	}
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
			peers = append(peers, DevelopmentNetworkPeer{p.ID, p.IPv4, p.PubKey, p.Name})
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
