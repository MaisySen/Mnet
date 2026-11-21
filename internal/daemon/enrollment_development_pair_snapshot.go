//go:build mnet_enrollment_development

package daemon

import "time"

// DevelopmentPairSnapshot is public diagnostic DATA within the existing finite
// development session. No credential, private key, new authority or refresh.
type DevelopmentPairSnapshot struct {
	SelfIPv4      string
	SelfPublicKey string
	SelfExpiry    time.Time
	PeerExpiry    time.Time
}

func (s *DevelopmentNetworkSession) DevelopmentPairSnapshot(pin SessionPeer) (DevelopmentPairSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out DevelopmentPairSnapshot
	if s.ctx.Err() != nil || !developmentExactPeerPresent(s.sessionPeerSnapshot(), pin) {
		return out, errEnrollmentContextStale
	}
	m := s.d.currentMap()
	if m == nil || m.Self.KeyExpiry == nil {
		return out, errEnrollmentContextStale
	}
	out.SelfIPv4 = m.Self.IPv4
	out.SelfPublicKey = m.Self.PubKey
	out.SelfExpiry = *m.Self.KeyExpiry
	for _, p := range m.Peers {
		if p.PubKey == pin.PublicKey && p.IPv4 == pin.IPv4 && p.KeyExpiry != nil {
			out.PeerExpiry = *p.KeyExpiry
		}
	}
	if out.PeerExpiry.IsZero() {
		return DevelopmentPairSnapshot{}, errEnrollmentContextStale
	}
	return out, nil
}
