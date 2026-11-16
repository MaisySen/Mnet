package control

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestExplicitMembershipGatesIdentityDeviceQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	network, _ := s.CreateNetwork("one")
	other, _ := s.CreateNetwork("other")
	joined, err := s.Join(netmap.JoinRequest{Network: "one", Name: "device", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	issuer, subject := "https://issuer.invalid", "subject"
	if err := s.ApproveNodeIdentity(joined.NodeID, "display@example.invalid", issuer, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IdentityDevices(network.ID, issuer, subject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("approval became implicit membership")
	}
	if err := s.SetNetworkMember(network.ID, issuer, subject, true); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := s.IdentityDevices(network.ID, issuer, subject)
	if err != nil || len(devices) != 1 || devices[0].ID != joined.NodeID {
		t.Fatalf("explicit member query failed: %+v %v", devices, err)
	}
	for _, scope := range [][3]string{{other.ID, issuer, subject}, {network.ID, "https://other.invalid", subject}, {network.ID, issuer, "other"}, {"missing", issuer, subject}, {network.ID, "", subject}} {
		if _, err := s.IdentityDevices(scope[0], scope[1], scope[2]); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("scope accepted: %+v", scope)
		}
	}
	// A member with no owned approvals cannot see somebody else's devices.
	if err := s.SetNetworkMember(network.ID, issuer, "other", true); err != nil {
		t.Fatal(err)
	}
	if own, err := s.IdentityDevices(network.ID, issuer, "other"); err != nil || len(own) != 0 {
		t.Fatal("membership exposed other principal's devices")
	}
	if err := s.SetNetworkMember(network.ID, issuer, subject, false); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IdentityDevices(network.ID, issuer, subject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked member retained access")
	}
	if _, err := s.AuthorizeNode(joined.NodeID, joined.NodeToken); err != nil {
		t.Fatal("membership revocation silently changed device transport authority")
	}
}

func TestMembershipPersistenceRollbackAndTupleKeys(t *testing.T) {
	if memberIndex("ab", "c", "d") == memberIndex("a", "bc", "d") || memberIndex("a", "b\x00c", "d") == memberIndex("a", "b", "c\x00d") {
		t.Fatal("principal tuple collision")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	network, _ := s.CreateNetwork("one")
	for _, scope := range [][3]string{{"", "i", "s"}, {network.ID, "", "s"}, {network.ID, "i", ""}, {"missing", "i", "s"}} {
		if err := s.SetNetworkMember(scope[0], scope[1], scope[2], true); err == nil {
			t.Fatal("invalid scope accepted")
		}
	}
	before, _ := json.Marshal(s.data)
	s.path = filepath.Join(path, "invalid", "state.json")
	if err := s.SetNetworkMember(network.ID, "issuer", "subject", true); err == nil {
		t.Fatal("grant persistence unexpectedly succeeded")
	}
	after, _ := json.Marshal(s.data)
	if string(before) != string(after) {
		t.Fatal("failed grant changed live state")
	}
	s.path = path
	if err := s.SetNetworkMember(network.ID, "issuer", "subject", true); err != nil {
		t.Fatal(err)
	}
	before, _ = json.Marshal(s.data)
	s.path = filepath.Join(path, "invalid", "state.json")
	if err := s.SetNetworkMember(network.ID, "issuer", "subject", false); err == nil {
		t.Fatal("revoke persistence unexpectedly succeeded")
	}
	after, _ = json.Marshal(s.data)
	if string(before) != string(after) {
		t.Fatal("failed revoke changed live state")
	}
}
