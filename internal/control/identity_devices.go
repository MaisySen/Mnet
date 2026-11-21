package control

import (
	"fmt"
	"sort"
	"time"
)

// IdentityDevice is a private account-workflow summary. It deliberately omits
// tokens, keys, endpoints and display email. It is not an authorization grant.
type IdentityDevice struct {
	ID   string
	Name string
	IPv4 string
}

// ApprovedDevicesForIdentity is internal groundwork for future account flows.
// The caller must already have authenticated this issuer/subject and must
// independently authorize the requested network before disclosing results.
// HTTP account reads use IdentityDevices instead, with its explicit membership gate.
// Email labels and legacy approvals never match.
func (s *Store) ApprovedDevicesForIdentity(networkID, issuer, subject string) ([]IdentityDevice, error) {
	if networkID == "" || issuer == "" || subject == "" {
		return nil, fmt.Errorf("network and verified issuer/subject required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	if s.data.Networks[networkID] == nil {
		return nil, ErrNotFound
	}
	return s.approvedDevicesForIdentityLocked(networkID, issuer, subject), nil
}

func (s *Store) approvedDevicesForIdentityLocked(networkID, issuer, subject string) []IdentityDevice {
	now := time.Now()
	var result []IdentityDevice
	for id, identity := range s.data.ApprovalIdentities {
		if s.registrarNodeLocked(id) != nil {
			continue
		}
		if identity.Issuer != issuer || identity.Subject != subject {
			continue
		}
		node := s.data.Nodes[id]
		if node == nil || node.NetworkID != networkID || !node.Approved || (node.KeyExpiry != nil && !now.Before(*node.KeyExpiry)) {
			continue
		}
		result = append(result, IdentityDevice{ID: node.ID, Name: node.Name, IPv4: node.IPv4})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
