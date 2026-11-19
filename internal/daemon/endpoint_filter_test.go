package daemon

import (
	"fmt"
	"math/rand"
	"net/netip"
	"reflect"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

// Deliberately retain the original linear rule as an independent semantic
// reference, including normalization, invalid routes and scoped addresses.
func originalEndpointFilter(endpoints []string, peers []netmap.Node) []string {
	blocked := []netip.Prefix{netip.MustParsePrefix("100.64.0.0/10")}
	for _, peer := range peers {
		for _, raw := range peer.Routes {
			if p, err := netip.ParsePrefix(raw); err == nil {
				blocked = append(blocked, p.Masked())
			}
		}
	}
	var safe []string
	for _, raw := range endpoints {
		ep, err := netip.ParseAddrPort(raw)
		if err != nil {
			continue
		}
		recursive := false
		for _, prefix := range blocked {
			if prefix.Contains(ep.Addr().Unmap()) {
				recursive = true
				break
			}
		}
		if !recursive {
			safe = append(safe, raw)
		}
	}
	return safe
}

func TestEndpointRouteFilterMatchesOriginalPolicy(t *testing.T) {
	endpoints := []string{"100.63.255.255:1", "100.64.0.0:1", "100.127.255.255:1", "100.128.0.0:1", "192.0.2.7:12345", "10.9.0.7:22", "[2001:db8::7]:22", "[2001:db8::1234]:22", "[fe80::1%fixture]:22", "[::ffff:10.9.0.7]:22", "[::ffff:100.64.0.7]:22", "hostname:1", "bad", "[::]:0", "0.0.0.0:0", "192.0.2.7:12345"}
	sets := [][]netmap.Node{
		nil,
		{{Routes: []string{"bad", "10.9.0.123/24", "2001:db8::1234/64", "10.9.0.0/24"}}, {Routes: []string{"192.0.2.0/25"}}},
		{{Routes: []string{"0.0.0.0/0", "::/0"}}},
		{{Routes: []string{"::ffff:10.9.0.0/120", "fe80::/10"}}},
	}
	rng := rand.New(rand.NewSource(73))
	for iteration := 0; iteration < 50; iteration++ {
		var peers []netmap.Node
		for n := 0; n < 16; n++ {
			var raw [16]byte
			_, _ = rng.Read(raw[:])
			bits := rng.Intn(129)
			peers = append(peers, netmap.Node{Routes: []string{netip.PrefixFrom(netip.AddrFrom16(raw), bits).String()}})
			endpoints = append(endpoints, netip.AddrPortFrom(netip.AddrFrom16(raw), 53).String())
			var v4 [4]byte
			_, _ = rng.Read(v4[:])
			peers = append(peers, netmap.Node{Routes: []string{netip.PrefixFrom(netip.AddrFrom4(v4), rng.Intn(33)).String()}})
			endpoints = append(endpoints, netip.AddrPortFrom(netip.AddrFrom4(v4), 53).String())
		}
		sets = append(sets, peers)
	}
	// Exercise every prefix length, including host prefixes and defaults,
	// independently so an unrelated /0 cannot hide a boundary mismatch.
	for bits := 0; bits <= 128; bits++ {
		sets = append(sets, []netmap.Node{{Routes: []string{fmt.Sprintf("2001:db8::1234/%d", bits)}}})
	}
	for bits := 0; bits <= 32; bits++ {
		sets = append(sets, []netmap.Node{{Routes: []string{fmt.Sprintf("10.9.0.7/%d", bits)}}})
	}
	before := append([]string(nil), endpoints...)
	for n, peers := range sets {
		filter := newEndpointRouteFilter(peers)
		got := filter.safe(endpoints)
		want := originalEndpointFilter(endpoints, peers)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("policy set %d differs: got %v want %v", n, got, want)
		}
	}
	if !reflect.DeepEqual(endpoints, before) {
		t.Fatal("filter mutated caller endpoint candidates")
	}
}

func TestEndpointRouteFilterCurrentMapOwnsSafetySet(t *testing.T) {
	d := testDaemon(t)
	key := pubKeyOf(t, 0x51)
	router := pubKeyOf(t, 0x52)
	current := &netmap.Map{Version: 1, Peers: []netmap.Node{
		{PubKey: key, IPv4: "100.64.0.2", Endpoints: []string{"198.18.23.7:12345"}},
		{PubKey: router, IPv4: "100.64.0.3", Routes: []string{"198.18.23.123/24"}},
	}}
	d.applyMap(current)
	if len(d.peers[key].node.Endpoints) != 0 {
		t.Fatal("another peer's route did not force recursive candidate off direct path")
	}
	// A later map must rebuild from its current routes, not keep a stale cache.
	next := *current
	next.Version++
	next.Peers = append([]netmap.Node(nil), current.Peers...)
	next.Peers[1].Routes = []string{"203.0.113.0/24"}
	d.applyMap(&next)
	if !reflect.DeepEqual(d.peers[key].node.Endpoints, []string{"198.18.23.7:12345"}) {
		t.Fatal("retired route still blocks current direct candidate")
	}
	// Restoring the old map must block it again. Neither input is mutated.
	d.applyMap(current)
	if len(d.peers[key].node.Endpoints) != 0 {
		t.Fatal("new map's filter escaped its lifetime")
	}
	if !reflect.DeepEqual(current.Peers[0].Endpoints, []string{"198.18.23.7:12345"}) {
		t.Fatal("map input candidate list was changed")
	}
}
