package dns

import (
	"fintex.au/mnet/internal/netmap"
	"fmt"
	"testing"
)

func TestShortNamesRemainAmbiguousAfterThreeCollisions(t *testing.T) {
	for _, count := range []int{2, 3, 4, 5} {
		m := testMap()
		m.Peers = nil
		for i := 0; i < count; i++ {
			m.Peers = append(m.Peers, netmap.Node{ID: string(rune('a' + i)), Name: "duplicate", IPv4: fmt.Sprintf("100.64.0.%d", i+2)})
		}
		s, _ := New("127.0.0.1")
		s.Update(m)
		if ip, ok := s.records["duplicate.mnet."]; ok {
			t.Errorf("%d peers: ambiguous short name resolves to %s", count, ip)
		}
	}
}

func TestSelfShortNameDoesNotOverrideAmbiguousPeerName(t *testing.T) {
	m := testMap()
	m.Self.Name = "duplicate"
	m.Peers = []netmap.Node{{Name: "duplicate", IPv4: "100.64.0.2"}, {Name: "duplicate", IPv4: "100.64.0.3"}}
	s, _ := New("127.0.0.1")
	s.Update(m)
	if ip, ok := s.records["duplicate.mnet."]; ok {
		t.Fatalf("ambiguous self short name resolves to %s", ip)
	}
}

func TestAliasesCannotOverrideNetworkOrQualifiedNames(t *testing.T) {
	m := testMap()
	m.Peers = append(m.Peers, netmap.Node{Name: "HOME", IPv4: "100.64.0.9"})
	s, _ := New("127.0.0.1")
	s.Update(m)
	if ip, ok := s.records["home.mnet."]; ok {
		t.Fatalf("ambiguous network alias resolves to %s", ip)
	}
	if ip := s.records["home.home.mnet."]; ip != "100.64.0.9" {
		t.Fatalf("qualified record: %s", ip)
	}
	m.Peers = append(m.Peers, netmap.Node{Name: "server", IPv4: "100.64.0.10"})
	s.Update(m)
	if _, ok := s.records["server.home.mnet."]; ok {
		t.Fatal("ambiguous qualified name was retained")
	}
}

func TestDNSRecordNamesAreCaseInsensitive(t *testing.T) {
	m := testMap()
	m.Peers[0].Name = "Server"
	m.Peers = append(m.Peers, netmap.Node{Name: "SERVER", IPv4: "100.64.0.9"})
	s, _ := New("127.0.0.1")
	s.Update(m)
	if _, ok := s.records["server.mnet."]; ok {
		t.Fatal("case collision was not ambiguous")
	}
	m.Peers = m.Peers[:1]
	s.Update(m)
	if ip := s.records["server.home.mnet."]; ip != "100.64.0.2" {
		t.Fatalf("case-normalized record: %s", ip)
	}
}
