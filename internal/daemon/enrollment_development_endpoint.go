//go:build mnet_enrollment_development

package daemon

import (
	"context"
	"fintex.au/mnet/internal/netmap"
	"time"
)

type developmentEndpointPin struct{ self, peer netmap.EndpointProfile }

func (p *developmentEndpointPin) current(m *netmap.Map) bool {
	if p == nil || m == nil || m.Self.EndpointProfile == nil || *m.Self.EndpointProfile != p.self || !p.self.Matches(m.Self, time.Now()) || p.self.Revision > m.Version || p.peer.Revision > m.Version {
		return false
	}
	matches := 0
	for _, n := range m.Peers {
		if n.ID == p.peer.NodeID {
			if n.EndpointProfile == nil || *n.EndpointProfile != p.peer || !p.peer.Matches(n, time.Now()) {
				return false
			}
			matches++
		}
	}
	return matches == 1 && p.self.NetworkID == p.peer.NetworkID && p.self.NodeID != p.peer.NodeID && p.self.PublicKey != p.peer.PublicKey && p.self.IPv4 != p.peer.IPv4
}

// Compatibility only. Caller must first derive the local declaration from its
// supported prepared configuration. This does NOT claim the local kernel is
// ready, create a TUN, extend membership, or grant screen/input authority.
// Explicit finite native request, no automatic publication/polling/renewal.
func (s *DevelopmentNetworkSession) PublishEndpointCapability(capability *netmap.EndpointCapability) error {
	if s == nil {
		return errEnrollmentContextStale
	}
	s.mu.Lock()
	m := s.d.currentMap()
	var err error
	if s.ctx.Err() != nil || m == nil {
		err = errEnrollmentContextStale
	} else {
		err = s.endpointPublication.publish(capability, func(c *netmap.EndpointCapability) error {
			return s.d.nativeCurrentEndpointProfile(s.ctx, append([]string(nil), m.Self.Endpoints...), s.limits, c)
		})
		if err == nil {
			err = s.refreshLocked()
		}
	}
	s.mu.Unlock()
	if err != nil {
		s.Close()
	}
	return err
}

// Best effort only: finite request budget and remote failures can prevent a
// withdrawal acknowledgement. Local cancellation/teardown never waits on a
// retry and cannot rely on this request succeeding. Server original expiry is
// always a hard cap. No new request allowance or renewed credential is minted.
func (s *DevelopmentNetworkSession) withdrawEndpointBeforeClose() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.endpointPublication.mayBePublished || s.endpointPublication.closeAttempted {
		return
	}
	if s.ctx.Err() != nil {
		return
	}
	m := s.d.currentMap()
	if m == nil {
		return
	}
	_ = s.endpointPublication.withdrawForClose(func(c *netmap.EndpointCapability) error {
		return s.d.nativeCurrentEndpointProfile(s.ctx, append([]string(nil), m.Self.Endpoints...), s.limits, c)
	})
}

// No public constructor accepts profile DATA. This source owner is minted only
// from the actual current native map and peer scope. Still a review boundary,
// not a claim that arbitrary code inside this package cannot manufacture one.
type DevelopmentEndpointPeer struct {
	session *DevelopmentNetworkSession
	scope   *DevelopmentPeerScope
	pin     *developmentEndpointPin
}

func (p *DevelopmentEndpointPeer) Context() context.Context {
	if p == nil || p.scope == nil {
		return nil
	}
	return p.scope.Context()
}
func (p *DevelopmentEndpointPeer) Close() error {
	if p == nil || p.scope == nil {
		return errEnrollmentContextStale
	}
	return p.scope.Close()
}

// Preparation needs current approved membership, not yet the peer's MTU/profile.
// This avoids requiring both peers to publish before either can prepare locally.
func (s *DevelopmentNetworkSession) BindEndpointPreparation(peer SessionPeer) (*DevelopmentEndpointPeer, error) {
	scope, err := s.BindPeer(peer)
	if err != nil {
		return nil, err
	}
	return &DevelopmentEndpointPeer{session: s, scope: scope}, nil
}
func (s *DevelopmentNetworkSession) BindEndpointPeer(peer SessionPeer) (*DevelopmentEndpointPeer, error) {
	out, err := s.BindEndpointPreparation(peer)
	if err != nil {
		return nil, err
	}
	if err = out.BindProfiles(); err != nil {
		_ = out.Close()
		return nil, err
	}
	return out, nil
}

// Binding is explicit after the caller's bounded native Refresh observes both
// published declarations. No HTTP/poll/renewal is hidden in this method.
func (p *DevelopmentEndpointPeer) BindProfiles() error {
	if p == nil || p.session == nil || p.scope == nil {
		return errEnrollmentContextStale
	}
	s := p.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.scope.ctx.Err() != nil {
		return errEnrollmentContextStale
	}
	m := s.d.currentMap()
	var pin *developmentEndpointPin
	if m != nil && m.Self.EndpointProfile != nil {
		for _, n := range m.Peers {
			if n.Name == p.scope.peer.Name && n.IPv4 == p.scope.peer.IPv4 && n.PubKey == p.scope.peer.PublicKey && n.EndpointProfile != nil {
				pin = &developmentEndpointPin{self: *m.Self.EndpointProfile, peer: *n.EndpointProfile}
			}
		}
	}
	if pin == nil || !pin.current(m) {
		return errEnrollmentContextStale
	}
	if p.pin != nil && *p.pin != *pin {
		return errEnrollmentContextStale
	}
	p.pin = pin
	p.scope.endpointPin = pin
	return nil
}

// This is current membership DATA bound to the actual native owner, not MTU or
// local kernel proof. Copies do not expose the mutable installed map.
func (p *DevelopmentEndpointPeer) MembershipSnapshot() (netmap.Node, netmap.Node, error) {
	var zero netmap.Node
	if p == nil || p.session == nil || p.scope == nil {
		return zero, zero, errEnrollmentContextStale
	}
	s := p.session
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.mu.Lock()
	err := s.d.checkNativeCurrentAfterLocked(s.ctx, s.revision)
	s.d.mu.Unlock()
	m := s.d.currentMap()
	if err != nil || p.scope.ctx.Err() != nil || m == nil || !validDevelopmentNetworkMap(m) || !developmentExactPeerPresent(s.sessionPeerSnapshot(), p.scope.peer) {
		return zero, zero, errEnrollmentContextStale
	}
	var peer netmap.Node
	for _, n := range m.Peers {
		if n.PubKey == p.scope.peer.PublicKey && n.IPv4 == p.scope.peer.IPv4 && n.Name == p.scope.peer.Name {
			peer = n
		}
	}
	if peer.KeyExpiry == nil || m.Self.KeyExpiry == nil || !time.Now().Before(*peer.KeyExpiry) || !time.Now().Before(*m.Self.KeyExpiry) || !peer.KeyExpiry.Equal(p.scope.peerDeadline) {
		return zero, zero, errEnrollmentContextStale
	}
	copyNode := func(n netmap.Node) netmap.Node {
		n.Endpoints = append([]string(nil), n.Endpoints...)
		n.Routes = append([]string(nil), n.Routes...)
		if n.KeyExpiry != nil {
			v := *n.KeyExpiry
			n.KeyExpiry = &v
		}
		if n.EndpointProfile != nil {
			v := *n.EndpointProfile
			n.EndpointProfile = &v
		}
		return n
	}
	return copyNode(m.Self), copyNode(peer), nil
}

// Rechecks the same installed map and native credential revision. It does not
// poll a remote service on every packet or promise instant offline revocation.
// Refresh/expiry/withdrawal of an observed profile cancels the owned scope.
func (p *DevelopmentEndpointPeer) Snapshot() (netmap.EndpointProfile, netmap.EndpointProfile, error) {
	var zero netmap.EndpointProfile
	if p == nil || p.session == nil || p.scope == nil {
		return zero, zero, errEnrollmentContextStale
	}
	s := p.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.pin == nil {
		return zero, zero, errEnrollmentContextStale
	}
	s.d.mu.Lock()
	err := s.d.checkNativeCurrentAfterLocked(s.ctx, s.revision)
	s.d.mu.Unlock()
	if err != nil || p.scope.ctx.Err() != nil || p.scope.endpointPin != p.pin || !p.pin.current(s.d.currentMap()) {
		return zero, zero, errEnrollmentContextStale
	}
	return p.pin.self, p.pin.peer, nil
}
