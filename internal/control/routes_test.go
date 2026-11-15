package control_test

import (
	"encoding/json"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

// Joins that advertise exit-node duty or subnet routes must carry those
// into the map peers see.
func TestJoinCarriesRoutesIntoMap(t *testing.T) {
	ts, pw := testEnv(t)
	base := ts.URL
	cookie := adminLogin(t, base, pw)
	auth := map[string]string{"Cookie": cookie.String()}

	code, body := post(t, base+"/admin/api/networks", map[string]string{"name": "routes"}, auth)
	if code != 200 {
		t.Fatalf("create network: %d %s", code, body)
	}

	join := func(req netmap.JoinRequest) netmap.JoinResponse {
		code, body := post(t, base+"/api/join", req, nil)
		if code != 200 {
			t.Fatalf("join %s: %d %s", req.Name, code, body)
		}
		var jr netmap.JoinResponse
		json.Unmarshal(body, &jr)
		post(t, base+"/admin/api/nodes/approve", map[string]string{"node_id": jr.NodeID}, auth)
		return jr
	}
	gw := join(netmap.JoinRequest{
		Network: "routes", Name: "gateway", PubKey: fakePubKey(0x01),
		ExitNode: true, Routes: []string{"10.9.0.0/24", "not-a-cidr"},
	})
	cl := join(netmap.JoinRequest{Network: "routes", Name: "client", PubKey: fakePubKey(0x02)})

	headers := map[string]string{"X-Node-ID": cl.NodeID, "X-Node-Token": cl.NodeToken}
	code, m := getMap(t, base, cl, headers)
	if code != 200 {
		t.Fatalf("map: %d", code)
	}
	_ = gw
	if len(m.Peers) != 1 {
		t.Fatalf("peers: %+v", m.Peers)
	}
	p := m.Peers[0]
	if !p.ExitNode {
		t.Fatal("exit node flag lost")
	}
	if len(p.Routes) != 1 || p.Routes[0] != "10.9.0.0/24" {
		t.Fatalf("routes lost or not validated: %v", p.Routes)
	}
}
