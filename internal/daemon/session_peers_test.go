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

func TestSessionPeersDenyEveryNonUpStatusAndUnapprovedSelf(t *testing.T) {
	for _, status := range []string{"", "down", "starting", "waiting-approval", "key-expired", "error"} {
		t.Run(status, func(t *testing.T) {
			d := &Daemon{status: status, lastMap: &netmap.Map{Self: netmap.Node{Approved: true}, Peers: []netmap.Node{{Approved: true, Name: "viewer", IPv4: "100.64.0.2", PubKey: "fixture"}}}}
			if len(d.SessionPeers()) != 0 {
				t.Fatal("non-up state exposed admission authority")
			}
		})
	}
	d := &Daemon{status: "up", lastMap: &netmap.Map{Self: netmap.Node{Approved: false}, Peers: []netmap.Node{{Approved: true, Name: "viewer", IPv4: "100.64.0.2", PubKey: "fixture"}}}}
	if len(d.SessionPeers()) != 0 {
		t.Fatal("unapproved self exposed admission authority")
	}
}

func TestSessionPeersSnapshotDoesNotReauthorizeRemovedOrExpiredPeer(t *testing.T) {
	expired := time.Now().Add(-time.Second)
	future := time.Now().Add(time.Hour)
	d := &Daemon{status: "up", lastMap: &netmap.Map{Self: netmap.Node{Approved: true}, Peers: []netmap.Node{{Approved: true, Name: "viewer", IPv4: "100.64.0.2", PubKey: "fixture", KeyExpiry: &future}}}}
	old := d.SessionPeers()
	if len(old) != 1 {
		t.Fatal("active peer missing")
	}
	d.lastMap.Peers[0].KeyExpiry = &expired
	if len(d.SessionPeers()) != 0 {
		t.Fatal("known expiry retained admission")
	}
	if old[0].Name != "viewer" {
		t.Fatal("map update mutated issued snapshot")
	}
	d.lastMap.Peers = nil
	old[0].Name = "old-copy"
	if len(d.SessionPeers()) != 0 {
		t.Fatal("old copy restored revoked authority")
	}
}
