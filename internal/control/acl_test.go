package control_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func aclEnv(t *testing.T) (base string, auth map[string]string, ids map[string]netmap.JoinResponse) {
	t.Helper()
	ts, pw := testEnv(t)
	base = ts.URL
	auth = map[string]string{"Cookie": adminLogin(t, base, pw).String()}
	code, body := post(t, base+"/admin/api/networks", map[string]string{"name": "acl"}, auth)
	if code != 200 {
		t.Fatalf("create network: %d %s", code, body)
	}
	var net netmap.Network
	json.Unmarshal(body, &net)
	ids = map[string]netmap.JoinResponse{}
	for i, name := range []string{"alice", "bob", "router"} {
		req := netmap.JoinRequest{Network: "acl", Name: name, PubKey: fakePubKey(byte(0x10 + i))}
		if name == "router" {
			req.ExitNode = true
			req.Routes = []string{"10.9.0.0/24"}
		}
		code, body := post(t, base+"/api/join", req, nil)
		if code != 200 {
			t.Fatalf("join %s: %d %s", name, code, body)
		}
		var jr netmap.JoinResponse
		json.Unmarshal(body, &jr)
		post(t, base+"/admin/api/nodes/approve", map[string]string{"node_id": jr.NodeID}, auth)
		ids[name] = jr
	}
	ids["network"] = netmap.JoinResponse{NodeID: net.ID}
	return
}

func mapPeers(t *testing.T, base string, j netmap.JoinResponse) []netmap.Node {
	t.Helper()
	h := map[string]string{"X-Node-ID": j.NodeID, "X-Node-Token": j.NodeToken}
	code, m := getMap(t, base, j, h)
	if code != 200 {
		t.Fatalf("map: %d", code)
	}
	return m.Peers
}

func putACL(t *testing.T, base string, auth map[string]string, netID string, rules []netmap.ACLRule) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, base+"/admin/api/acl", jsonBody(map[string]any{"network_id": netID, "rules": rules}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", auth["Cookie"])
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestACLPolicy(t *testing.T) {
	base, auth, ids := aclEnv(t)
	netID := ids["network"].NodeID

	// Default: allow all, routes and exit offers visible.
	peers := mapPeers(t, base, ids["alice"])
	if len(peers) != 2 {
		t.Fatalf("default policy should show 2 peers: %+v", peers)
	}

	// Default deny once any rule exists.
	if c := putACL(t, base, auth, netID, []netmap.ACLRule{{Src: "alice", Dst: "bob"}}); c != 200 {
		t.Fatalf("put acl: %d", c)
	}
	peers = mapPeers(t, base, ids["alice"])
	if len(peers) != 1 || peers[0].Name != "bob" {
		t.Fatalf("alice should only see bob: %+v", peers)
	}
	// bob sees alice back (reverse direction opens the pair).
	peers = mapPeers(t, base, ids["bob"])
	if len(peers) != 1 || peers[0].Name != "alice" {
		t.Fatalf("bob should only see alice: %+v", peers)
	}
	// router sees nobody.
	if peers = mapPeers(t, base, ids["router"]); len(peers) != 0 {
		t.Fatalf("router should see no peers: %+v", peers)
	}

	// Route access opens the mesh path to the router and keeps the CIDR.
	putACL(t, base, auth, netID, []netmap.ACLRule{
		{Src: "alice", Dst: "bob"},
		{Src: "alice", Dst: "10.9.0.0/24"},
	})
	peers = mapPeers(t, base, ids["alice"])
	if len(peers) != 2 {
		t.Fatalf("alice should see bob and router: %+v", peers)
	}
	for _, p := range peers {
		if p.Name == "router" {
			if len(p.Routes) != 1 || p.Routes[0] != "10.9.0.0/24" {
				t.Fatalf("route filtered out: %+v", p)
			}
			if p.ExitNode {
				t.Fatal("exit offer must be hidden without internet access")
			}
		}
	}

	// Wildcard dst restores exit access.
	putACL(t, base, auth, netID, []netmap.ACLRule{{Src: "*", Dst: "*"}})
	peers = mapPeers(t, base, ids["alice"])
	for _, p := range peers {
		if p.Name == "router" && !p.ExitNode {
			t.Fatal("wildcard should show the exit offer")
		}
	}

	// Validation: unknown src node and bad dst rejected.
	if c := putACL(t, base, auth, netID, []netmap.ACLRule{{Src: "nobody", Dst: "*"}}); c != 400 {
		t.Fatalf("unknown src accepted: %d", c)
	}
	if c := putACL(t, base, auth, netID, []netmap.ACLRule{{Src: "*", Dst: "junk"}}); c != 400 {
		t.Fatalf("bad dst accepted: %d", c)
	}

	// Clearing restores allow-all.
	if c := putACL(t, base, auth, netID, nil); c != 200 {
		t.Fatalf("clear acl: %d", c)
	}
	if peers = mapPeers(t, base, ids["alice"]); len(peers) != 2 {
		t.Fatalf("cleared policy should allow all: %+v", peers)
	}
}
