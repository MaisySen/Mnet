package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
	"github.com/gorilla/websocket"
)

func TestAdminRelayPresenceUsesNodeNotPublicKey(t *testing.T) {
	st := scaleStore(t, 2)
	a, b := st.data.Nodes["node-000000"], st.data.Nodes["node-000001"]
	b.PubKey = a.PubKey
	b.NetworkID = "other"
	st.data.Networks["other"] = &netmap.Network{ID: "other", Name: "other"}
	hub := relay.NewHub()
	app, _, err := NewServer(st, hub, "http://localhost", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	app.sessions["fixture-session"] = time.Now().Add(time.Minute)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	connect := func(id string) *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/relay/ws?node_id="+id+"&token=synthetic-benchmark-token", nil)
		if err != nil {
			t.Fatal(err)
		}
		return ws
	}
	check := func(wantA, wantB bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			req := httptest.NewRequest("GET", "http://localhost/admin/api/state", nil)
			req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: "fixture-session"})
			rec := httptest.NewRecorder()
			app.Handler().ServeHTTP(rec, req)
			var body struct {
				OnlineByNode map[string]bool `json:"online_by_node"`
			}
			if rec.Code != 200 {
				t.Fatalf("state: %d", rec.Code)
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.OnlineByNode[a.ID] == wantA && body.OnlineByNode[b.ID] == wantB {
				count := 0
				if wantA {
					count++
				}
				if wantB {
					count++
				}
				if len(body.OnlineByNode) != count {
					t.Fatal("unexpected presence", body.OnlineByNode)
				}
				// Returned maps must not mutate the live hub.
				body.OnlineByNode["injected"] = true
				if hub.OnlineNodes()["injected"] {
					t.Fatal("presence snapshot aliases hub")
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("presence %v want %t/%t", body.OnlineByNode, wantA, wantB)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	check(false, false)
	first := connect(a.ID)
	defer first.Close()
	check(true, false)
	second := connect(b.ID)
	defer second.Close()
	check(true, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://localhost/admin/api/metrics", nil)
	req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: "fixture-session"})
	app.Handler().ServeHTTP(rec, req)
	var metrics map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics["nodes_online_now"] != float64(2) {
		t.Fatalf("identical keys collapsed metric: %s", rec.Body.String())
	}
	first.Close()
	check(false, true)
	second.Close()
	check(false, false)
}
