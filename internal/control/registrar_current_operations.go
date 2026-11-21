package control

// Disconnected existing-record operations. No handler/router/transport caller,
// node creation, enrollment transaction or durable native credential is added.
import "fintex.au/mnet/internal/netmap"

// Credential verification and snapshot linearize under SAME actual Store.mu.
// Returned map is a point-in-time DATA snapshot, not a continuous access lease.
func (s *Store) registrarCredentialMap(ref registrarCredentialReference, secret string) (*netmap.Map, error) {
	s.mu.Lock()
	if _, err := s.authorizeRegistrarCredentialLocked(ref, secret); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	m, peers, err := s.mapSnapshotLocked(ref.NodeID)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return finishMapSnapshot(m, peers), nil
}

// No verifier-clone -> unlocked mutation gap. Exact same retained record/secret
// must remain current through actual endpoint candidate publication. Legacy
// tokens, a prior verified node clone or public receipt digest cannot call it.
func (s *Store) setRegistrarCredentialEndpoints(ref registrarCredentialReference, secret string, eps []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.authorizeRegistrarCredentialLocked(ref, secret); err != nil {
		return err
	}
	return s.setEndpointsLocked(ref.NodeID, eps)
}
