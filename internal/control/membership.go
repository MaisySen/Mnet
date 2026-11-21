package control

import (
	"encoding/json"
	"fmt"
	"sort"
)

// NetworkMember is an explicit operator-managed principal binding. Approval
// provenance, display email and matching domains never create membership.
// Only member access exists in this first model; no implied owner/admin role.
type NetworkMember struct {
	NetworkID string `json:"network_id"`
	Issuer    string `json:"issuer"`
	Subject   string `json:"subject"`
}

func memberIndex(networkID, issuer, subject string) string {
	// JSON tuple avoids delimiter collisions in untrusted identifiers.
	b, _ := json.Marshal([3]string{networkID, issuer, subject})
	return string(b)
}

// SetNetworkMember is internal operator/admin plumbing, not a self-service
// grant. The admin-only membership endpoint invokes it. Other callers must authorize management first.
// Revocation affects this account-query gate only: it does not revoke device
// credentials or existing mesh traffic. Use explicit device expiry for that.
func (s *Store) SetNetworkMember(networkID, issuer, subject string, allow bool) error {
	if networkID == "" || issuer == "" || subject == "" {
		return fmt.Errorf("network and verified issuer/subject required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return err
	}
	if s.data.Networks[networkID] == nil {
		return ErrNotFound
	}
	key := memberIndex(networkID, issuer, subject)
	previous, existed := s.data.Memberships[key]
	wasNil := s.data.Memberships == nil
	if allow {
		if existed {
			return nil
		}
		if s.data.Memberships == nil {
			s.data.Memberships = map[string]NetworkMember{}
		}
		s.data.Memberships[key] = NetworkMember{NetworkID: networkID, Issuer: issuer, Subject: subject}
	} else {
		if !existed {
			return nil
		}
		delete(s.data.Memberships, key)
	}
	return s.commitLocked(func() {
		if existed {
			s.data.Memberships[key] = previous
		} else {
			delete(s.data.Memberships, key)
		}
		if wasNil {
			s.data.Memberships = nil
		}
	})
}

// IdentityDevices requires an existing explicit membership and returns only
// this principal's approved devices, not every device in the network. The
// issuer/subject must come from verified authentication, never request JSON.
// The account endpoint obtains these values from a verified account session.
func (s *Store) IdentityDevices(networkID, issuer, subject string) ([]IdentityDevice, error) {
	if networkID == "" || issuer == "" || subject == "" {
		return nil, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	if s.data.Networks[networkID] == nil {
		return nil, ErrUnauthorized
	}
	member, ok := s.data.Memberships[memberIndex(networkID, issuer, subject)]
	if !ok || member.NetworkID != networkID || member.Issuer != issuer || member.Subject != subject {
		return nil, ErrUnauthorized
	}
	return s.approvedDevicesForIdentityLocked(networkID, issuer, subject), nil
}

// OperatorMemberships returns explicit grants for one exact existing network.
// It includes private principal identifiers for the authenticated operator,
// not for member/device routes, and never infers grants from approvals.
func (s *Store) OperatorMemberships(networkID string) ([]NetworkMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	if s.data.Networks[networkID] == nil {
		return nil, ErrNotFound
	}
	result := []NetworkMember{}
	for _, member := range s.data.Memberships {
		if member.NetworkID == networkID {
			result = append(result, member)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Issuer != result[j].Issuer {
			return result[i].Issuer < result[j].Issuer
		}
		return result[i].Subject < result[j].Subject
	})
	return result, nil
}
