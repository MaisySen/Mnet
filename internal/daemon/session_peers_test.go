package daemon

import (
	"fintex.au/mnet/internal/netmap"
	"testing"
	"time"
)

func TestSessionPeersRequireCurrentUpAndUnexpiredApproval(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	active := netmap.Node{Name: "viewer", IPv4: "100.64.0.2", PubKey: "fixture-key", Approved: true}
	d := &Daemon{status: "up", lastMap: &netmap.Map{Self: netmap.Node{Approved: true}, Peers: []netmap.Node{active, {Name: "pending", Approved: false}, {Name: "expired", Approved: true, KeyExpiry: &expired}}}}
	peers := d.SessionPeers()
	if len(peers) != 1 || peers[0].Name != active.Name || peers[0].PublicKey != active.PubKey {
		t.Fatal("did not isolate approved identity")
	}
	peers[0].Name = "changed"
	if d.lastMap.Peers[0].Name != active.Name {
		t.Fatal("identity snapshot aliases map")
	}
	d.status = "down"
	if len(d.SessionPeers()) != 0 {
		t.Fatal("down node exported admission identities")
	}
	d.status = "up"
	d.lastMap.Self.KeyExpiry = &expired
	if len(d.SessionPeers()) != 0 {
		t.Fatal("expired self exported admission identities")
	}
	d.lastMap = nil
	if len(d.SessionPeers()) != 0 {
		t.Fatal("missing map exported identities")
	}
}
