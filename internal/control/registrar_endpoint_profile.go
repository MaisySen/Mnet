package control

import (
	"crypto/rand"
	"encoding/base64"
	"fintex.au/mnet/internal/netmap"
	"net/netip"
	"slices"
	"time"
)

type registrarEndpointPublication struct {
	owner     registrarCredentialReference
	profile   netmap.EndpointProfile
	endpoints []string
}

// Same Store.mu/current credential through actual publication. Nil withdraws.
// The peer sees a compatibility declaration, not a remotely verified kernel.
// No durable data schema, membership, renewal, production route or backend call.
func (s *Store) setRegistrarCredentialEndpointProfile(ref registrarCredentialReference, secret string, eps []string, capability *netmap.EndpointCapability) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.authorizeRegistrarCredentialLocked(ref, secret)
	if err != nil {
		return err
	}
	var publicationID string
	if capability != nil {
		if !capability.Supported() || len(eps) == 0 || len(eps) > 64 || n.ExitNode || len(n.Routes) != 0 {
			return ErrUnauthorized
		}
		for _, raw := range eps {
			a, e := netip.ParseAddrPort(raw)
			if e != nil || a.Port() == 0 || a.Addr().Is4In6() || a.Addr().Zone() != "" || a.String() != raw {
				return ErrUnauthorized
			}
		}
	}
	if capability != nil {
		var id [32]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		publicationID = base64.RawURLEncoding.EncodeToString(id[:])
	}
	if err = s.setEndpointsLocked(ref.NodeID, eps); err != nil {
		return err
	}
	if s.endpointProfiles == nil {
		s.endpointProfiles = map[string]registrarEndpointPublication{}
	}
	if capability == nil {
		if _, ok := s.endpointProfiles[ref.NodeID]; ok {
			delete(s.endpointProfiles, ref.NodeID)
			s.bumpLocked(n.NetworkID)
		}
		return nil
	}
	s.bumpLocked(n.NetworkID)
	s.endpointProfiles[ref.NodeID] = registrarEndpointPublication{owner: ref, profile: netmap.EndpointProfile{Capability: *capability, PublicationID: publicationID, NetworkID: n.NetworkID, NodeID: n.ID, PublicKey: n.PubKey, IPv4: n.IPv4, Revision: s.version, AccessUntilMS: n.KeyExpiry.UnixMilli()}, endpoints: append([]string(nil), s.data.Nodes[ref.NodeID].Endpoints...)}
	return nil
}

// Only native current-map invokes this before the existing ACL filter. No
// hidden peer descriptors escape separately from their authorized peer record.
// Original owner/revision/expiry/endpoints must still match under the same lock.
func (s *Store) attachRegistrarEndpointProfileLocked(n *netmap.Node) {
	n.EndpointProfile = nil
	p, ok := s.endpointProfiles[n.ID]
	if !ok {
		return
	}
	current, e := s.currentRegistrarPublicationOwnerLocked(p.owner)
	if e != nil || !p.profile.Matches(*current, time.Now()) || !p.profile.Matches(*n, time.Now()) || !slices.Equal(current.Endpoints, p.endpoints) {
		delete(s.endpointProfiles, n.ID)
		return
	}
	value := p.profile
	n.EndpointProfile = &value
}
