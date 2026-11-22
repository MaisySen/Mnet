package control

import (
	"fintex.au/mnet/internal/netmap"
	"testing"
	"time"
)

func TestRegistrarEndpointProfileActualPublicationWithdrawalAndReopen(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	c := netmap.EndpointCapability{Version: 1, Mode: netmap.EndpointModeV1, MTU: 1280}
	eps := []string{"192.0.2.33:51820"}
	if err := s.setRegistrarCredentialEndpointProfile(ref, secret, eps, &c); err != nil {
		t.Fatal(err)
	}
	first, err := s.registrarCredentialMap(ref, secret)
	if err != nil || first.Self.EndpointProfile == nil {
		t.Fatal("actual publication absent", err)
	}
	pin := *first.Self.EndpointProfile
	if !pin.Matches(first.Self, time.Now()) || pin.Revision > first.Version {
		t.Fatal("publication binding")
	}
	c.MTU = 9000
	first.Self.EndpointProfile.Capability.MTU = 42
	again, err := s.registrarCredentialMap(ref, secret)
	if err != nil || again.Self.EndpointProfile == nil || *again.Self.EndpointProfile != pin {
		t.Fatal("publication aliases caller/snapshot", err)
	}
	legacy, err := s.Map(ref.NodeID)
	if err != nil || legacy.Self.EndpointProfile != nil {
		t.Fatal("profile leaked into legacy map", err)
	}
	// Existing endpoint-only operation means withdrawal, even same endpoint set.
	if err := s.setRegistrarCredentialEndpoints(ref, secret, eps); err != nil {
		t.Fatal(err)
	}
	gone, err := s.registrarCredentialMap(ref, secret)
	if err != nil || gone.Self.EndpointProfile != nil {
		t.Fatal("withdrawal missing", err)
	}
	c.MTU = 1280
	if err := s.setRegistrarCredentialEndpointProfile(ref, secret, eps, &c); err != nil {
		t.Fatal(err)
	}
	next, err := s.registrarCredentialMap(ref, secret)
	if err != nil || next.Self.EndpointProfile == nil || next.Self.EndpointProfile.PublicationID == pin.PublicationID || next.Self.EndpointProfile.Revision <= pin.Revision {
		t.Fatal("republish reused incarnation", err)
	}
	s = reopenRegistrarFixture(t, s)
	after, err := s.registrarCredentialMap(ref, secret)
	if err != nil || after.Self.EndpointProfile != nil || len(after.Self.Endpoints) != 1 {
		t.Fatal("restart restored volatile claim or lost endpoints", err)
	}
}
func TestRegistrarEndpointProfileRefusesForeignAuthorityAndDrift(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	c := netmap.EndpointCapability{Version: 1, Mode: netmap.EndpointModeV1, MTU: 1280}
	eps := []string{"192.0.2.34:51820"}
	wrong := ref
	wrong.Scope.ServiceID += "-foreign"
	if err := s.setRegistrarCredentialEndpointProfile(wrong, secret, eps, &c); err == nil {
		t.Fatal("foreign qualified scope accepted")
	}
	if err := s.setRegistrarCredentialEndpointProfile(ref, ref.ReceiptDigest, eps, &c); err == nil {
		t.Fatal("digest accepted as credential")
	}
	if len(s.endpointProfiles) != 0 {
		t.Fatal("failed auth published")
	}
	c.MTU = 1500
	if err := s.setRegistrarCredentialEndpointProfile(ref, secret, eps, &c); err == nil {
		t.Fatal("unsupported profile accepted")
	}
	c.MTU = 1280
	if err := s.setRegistrarCredentialEndpointProfile(ref, secret, eps, &c); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.data.Nodes[ref.NodeID].IPv4 = "100.64.0.99"
	s.mu.Unlock()
	m, err := s.registrarCredentialMap(ref, secret)
	if (err == nil && (m == nil || m.Self.EndpointProfile != nil)) || (err != nil && m != nil) {
		t.Fatal("IP drift retained usable old profile", err)
	}
	s.mu.Lock()
	s.data.Nodes[ref.NodeID].IPv4 = "100.64.0.1"
	s.mu.Unlock()
	if err := s.ExpireNode(ref.NodeID); err != nil {
		t.Fatal(err)
	}
	if m, err := s.registrarCredentialMap(ref, secret); err == nil || m != nil {
		t.Fatal("revocation retained publication access")
	}
}
