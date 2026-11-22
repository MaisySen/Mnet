//go:build mnet_enrollment_development

package daemon

import (
	"encoding/base64"
	"fintex.au/mnet/internal/netmap"
	"testing"
	"time"
)

func TestDevelopmentEndpointPinRejectsProfileDrift(t *testing.T) {
	until := time.Now().Add(time.Minute).Truncate(time.Millisecond)
	makeNode := func(id, ip, key string) netmap.Node {
		return netmap.Node{ID: id, NetworkID: "network", IPv4: ip, PubKey: key, Approved: true, KeyExpiry: &until}
	}
	self := makeNode("self", "100.64.0.1", "self-full-key")
	peer := makeNode("peer", "100.64.0.2", "peer-full-key")
	makeProfile := func(n netmap.Node, b byte) netmap.EndpointProfile {
		raw := make([]byte, 32)
		raw[0] = b
		return netmap.EndpointProfile{Capability: netmap.EndpointCapability{Version: 1, Mode: netmap.EndpointModeV1, MTU: 1280}, PublicationID: base64.RawURLEncoding.EncodeToString(raw), NetworkID: n.NetworkID, NodeID: n.ID, PublicKey: n.PubKey, IPv4: n.IPv4, Revision: 2, AccessUntilMS: until.UnixMilli()}
	}
	a, b := makeProfile(self, 1), makeProfile(peer, 2)
	self.EndpointProfile = &a
	peer.EndpointProfile = &b
	pin := &developmentEndpointPin{self: a, peer: b}
	baseline := netmap.Map{Version: 2, Self: self, Peers: []netmap.Node{peer}}
	if !pin.current(&baseline) {
		t.Fatal("exact profile model refused")
	}
	for _, kind := range []string{"missing", "id", "key", "ip", "network", "mtu", "publication", "revision", "expiry", "self-withdraw", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			m := baseline
			m.Peers = append([]netmap.Node(nil), baseline.Peers...)
			changed := b
			m.Peers[0].EndpointProfile = &changed
			switch kind {
			case "missing":
				m.Peers = nil
			case "id":
				m.Peers[0].ID += "x"
			case "key":
				m.Peers[0].PubKey += "x"
			case "ip":
				m.Peers[0].IPv4 = "100.64.0.3"
			case "network":
				m.Peers[0].NetworkID = "other"
			case "mtu":
				changed.Capability.MTU = 1500
			case "publication":
				changed.PublicationID = a.PublicationID
			case "revision":
				changed.Revision++
			case "expiry":
				expired := time.Now().Add(-time.Second)
				m.Peers[0].KeyExpiry = &expired
			case "self-withdraw":
				m.Self.EndpointProfile = nil
			case "duplicate":
				m.Peers = append(m.Peers, m.Peers[0])
			}
			if pin.current(&m) {
				t.Fatal("changed declaration accepted")
			}
		})
	}
}
