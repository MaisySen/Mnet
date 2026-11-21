package daemon

import (
	"encoding/json"
	"fintex.au/mnet/internal/localapi"
	"fintex.au/mnet/internal/netmap"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionPeersRequireCurrentUpAndUnexpiredApproval(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	active := netmap.Node{Name: "viewer", IPv4: "100.64.0.2", PubKey: "fixture-key", Approved: true}
	d := &Daemon{status: "up", lastMap: &netmap.Map{Self: netmap.Node{Approved: true}, Peers: []netmap.Node{active, {Name: "pending", Approved: false}, {Name: "expired", Approved: true, KeyExpiry: &expired}}}}
	attachActualTestOwner(t, d)
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
			attachActualTestOwner(t, d)
			if len(d.SessionPeers()) != 0 {
				t.Fatal("non-up state exposed admission authority")
			}
		})
	}
	d := &Daemon{status: "up", lastMap: &netmap.Map{Self: netmap.Node{Approved: false}, Peers: []netmap.Node{{Approved: true, Name: "viewer", IPv4: "100.64.0.2", PubKey: "fixture"}}}}
	attachActualTestOwner(t, d)
	if len(d.SessionPeers()) != 0 {
		t.Fatal("unapproved self exposed admission authority")
	}
}

func TestSessionPeersSnapshotDoesNotReauthorizeRemovedOrExpiredPeer(t *testing.T) {
	expired := time.Now().Add(-time.Second)
	future := time.Now().Add(time.Hour)
	d := &Daemon{status: "up", lastMap: &netmap.Map{Self: netmap.Node{Approved: true}, Peers: []netmap.Node{{Approved: true, Name: "viewer", IPv4: "100.64.0.2", PubKey: "fixture", KeyExpiry: &future}}}}
	attachActualTestOwner(t, d)
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

func TestSessionEvidenceLocalCapabilityAndCurrentExpiry(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	d := &Daemon{status: "up", lastMap: &netmap.Map{Self: netmap.Node{Approved: true}, Peers: []netmap.Node{
		{Name: "valid", IPv4: "100.64.0.2", Approved: true, PubKey: "full-key"},
		{Name: "expired", Approved: true, KeyExpiry: &expired},
		{Name: "pending", Approved: false},
	}}}
	attachActualTestOwner(t, d)
	handler := localapi.RequireToken("fixture-local-capability", http.HandlerFunc(d.serveSessionPeers))
	for _, tc := range []struct {
		name, token, origin, method string
		code                        int
	}{
		{"missing", "", "", http.MethodGet, 401},
		{"wrong", "wrong", "", http.MethodGet, 401},
		{"browser", "fixture-local-capability", "https://outside.invalid", http.MethodGet, 403},
		{"mutation", "fixture-local-capability", "", http.MethodPost, 405},
		{"valid", "fixture-local-capability", "", http.MethodGet, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://mnetd/localapi/session-peers", nil)
			req.Header.Set(localapi.TokenHeader, tc.token)
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.code {
				t.Fatal(rec.Code)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("evidence cacheable")
			}
			if rec.Code == 200 {
				var peers []SessionPeer
				if json.Unmarshal(rec.Body.Bytes(), &peers) != nil || len(peers) != 1 || peers[0].PublicKey != "full-key" {
					t.Fatal("did not return only approved current evidence")
				}
			}
		})
	}
	d.lastMap.Self.Approved = false
	req := httptest.NewRequest(http.MethodGet, "http://mnetd/localapi/session-peers", nil)
	req.Header.Set(localapi.TokenHeader, "fixture-local-capability")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var peers []SessionPeer
	if json.Unmarshal(rec.Body.Bytes(), &peers) != nil || len(peers) != 0 {
		t.Fatal("denied self exposed session authority")
	}
}
