package control

import (
	"encoding/base64"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExpiredPeersAreRemovedFromMaps(t *testing.T) {
	s := scaleStore(t, 2)
	if err := s.ExpireNode("node-000001"); err != nil {
		t.Fatal(err)
	}
	m, err := s.Map("node-000000")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Peers) != 0 {
		t.Fatal("expired peer still advertised")
	}
}

func TestMapCannotAuthorizeExpiredOrPendingSelf(t *testing.T) {
	for _, mode := range []string{"expired", "pending"} {
		t.Run(mode, func(t *testing.T) {
			s := scaleStore(t, 2)
			if mode == "expired" {
				past := time.Now().Add(-time.Minute)
				s.data.Nodes["node-000000"].KeyExpiry = &past
			} else {
				s.data.Nodes["node-000000"].Approved = false
			}
			if _, err := s.Map("node-000000"); err != ErrUnauthorized {
				t.Fatalf("unauthorized self got map: %v", err)
			}
		})
	}
}

func TestRelayAuthorizationRechecksScopeApprovalExpiryAndACL(t *testing.T) {
	for _, mode := range []string{"allowed", "other-network", "pending-source", "pending-destination", "expired-source", "expired-destination", "acl-denied", "deleted-destination"} {
		t.Run(mode, func(t *testing.T) {
			s := scaleStore(t, 3)
			source := s.data.Nodes["node-000000"]
			dst := s.data.Nodes["node-000001"]
			if !s.RelayAllowed(source.ID, dst.PubKey) {
				t.Fatal("fixture route denied")
			}
			switch mode {
			case "other-network":
				dst.NetworkID = "other"
				s.relayIndex = nil
			case "pending-source":
				source.Approved = false
			case "pending-destination":
				dst.Approved = false
			case "expired-source":
				past := time.Now().Add(-time.Second)
				source.KeyExpiry = &past
			case "expired-destination":
				past := time.Now().Add(-time.Second)
				dst.KeyExpiry = &past
			case "acl-denied":
				s.data.Networks["scale"].ACL = &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "node-000002", Dst: "node-000002"}}}
			case "deleted-destination":
				if err := s.DeleteNode(dst.ID); err != nil {
					t.Fatal(err)
				}
			}
			if got := s.RelayAllowed(source.ID, dst.PubKey); got != (mode == "allowed") {
				t.Fatalf("relay allowed=%v", got)
			}
		})
	}
}

func BenchmarkRelayAuthorization(b *testing.B) {
	s := scaleStore(b, 10000)
	if !s.RelayAllowed("node-000000", "synthetic-000001") {
		b.Fatal("fixture route denied")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !s.RelayAllowed("node-000000", "synthetic-000001") {
			b.Fatal("route denied")
		}
	}
}

func TestHTTPAdminExpiryClosesExistingRelay(t *testing.T) {
	s := scaleStore(t, 1)
	s.data.Nodes["node-000000"].PubKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	hub := relay.NewHub()
	app, _, err := NewServer(s, hub, "http://localhost", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/relay/ws?node_id=node-000000&token=synthetic-benchmark-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	// An echoed self-packet confirms Attach completed before expiration.
	frame := append(make([]byte, 32), []byte("before-expiry")...)
	if err = ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}
	ws.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err = ws.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	app.sessions["fixture-session"] = time.Now().Add(time.Minute)
	req, err := http.NewRequest("POST", server.URL+"/admin/api/nodes/expire", strings.NewReader(`{"node_id":"node-000000"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: "fixture-session"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expire status %d", resp.StatusCode)
	}
	_, _, err = ws.ReadMessage()
	if err == nil {
		t.Fatal("expired relay remained readable")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("expiry did not detach existing relay")
	}
}
