package control

import (
	"os"
	"reflect"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestMapSnapshotOwnsSlices(t *testing.T) {
	s := scaleStore(t, 2)
	s.data.Networks["scale"].ACL = &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "*", Dst: "*"}}}
	s.data.Nodes["node-000000"].Routes = []string{"10.0.0.0/24"}
	s.data.Nodes["node-000001"].Routes = []string{"10.1.0.0/24"}
	m, err := s.Map("node-000000")
	if err != nil {
		t.Fatal(err)
	}
	m.Self.Endpoints[0] = "changed:1"
	m.Self.Routes[0] = "changed"
	m.Peers[0].Endpoints[0] = "changed:2"
	m.Peers[0].Routes[0] = "changed"
	m.Network.ACL.Rules[0].Src = "changed"
	again, err := s.Map("node-000000")
	if err != nil {
		t.Fatal(err)
	}
	if again.Self.Endpoints[0] != "192.0.2.1:12345" || again.Peers[0].Endpoints[0] != "192.0.2.1:12345" || again.Self.Routes[0] != "10.0.0.0/24" || again.Peers[0].Routes[0] != "10.1.0.0/24" || again.Network.ACL.Rules[0].Src != "*" {
		t.Fatal("returned map aliases live store data")
	}
}

func TestEndpointSetNoopAvoidsWriteAndWake(t *testing.T) {
	s := scaleStore(t, 2)
	if err := s.SetEndpoints("node-000000", []string{"192.0.2.2:12345", "192.0.2.1:12345"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	version := s.Version()
	if err = s.SetEndpoints("node-000000", []string{"192.0.2.1:12345", "192.0.2.2:12345", "192.0.2.1:12345"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || s.Version() != version {
		t.Fatal("same endpoint set persisted or woke map consumers")
	}
	m, err := s.Map("node-000000")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Self.Endpoints, []string{"192.0.2.1:12345", "192.0.2.2:12345"}) {
		t.Fatalf("uncanonical endpoints: %v", m.Self.Endpoints)
	}
	if err = s.SetEndpoints("node-000000", []string{"192.0.2.3:12345"}); err != nil {
		t.Fatal(err)
	}
	if s.Version() <= version {
		t.Fatal("changed endpoints did not publish a version")
	}
}
