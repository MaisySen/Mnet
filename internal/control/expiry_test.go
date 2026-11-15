package control_test

import (
	"encoding/json"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

// Node keys expire: the control plane refuses expired nodes, and
// rejoining renews the key.
func TestKeyExpiry(t *testing.T) {
	ts, pw := testEnv(t)
	base := ts.URL
	cookie := adminLogin(t, base, pw)
	auth := map[string]string{"Cookie": cookie.String()}

	post(t, base+"/admin/api/networks", map[string]string{"name": "exp"}, auth)

	var headers map[string]string
	join := func() netmap.JoinResponse {
		code, body := post(t, base+"/api/join", netmap.JoinRequest{
			Network: "exp", Name: "laptop", PubKey: fakePubKey(0x77)}, headers)
		if code != 200 {
			t.Fatalf("join: %d %s", code, body)
		}
		var jr netmap.JoinResponse
		json.Unmarshal(body, &jr)
		return jr
	}
	j := join()
	post(t, base+"/admin/api/nodes/approve", map[string]string{"node_id": j.NodeID}, auth)

	headers = map[string]string{"X-Node-ID": j.NodeID, "X-Node-Token": j.NodeToken}
	if code, m := getMap(t, base, j, headers); code != 200 || m.Self.KeyExpiry == nil {
		t.Fatalf("map before expiry: %d %+v", code, m.Self)
	}

	// Force expiry: the node is refused.
	code, _ := post(t, base+"/admin/api/nodes/expire", map[string]string{"node_id": j.NodeID}, auth)
	if code != 200 {
		t.Fatalf("expire: %d", code)
	}
	if code, _ := getMap(t, base, j, headers); code != 401 {
		t.Fatalf("expired node should get 401, got %d", code)
	}

	// Authenticated rejoin preserves identity but cannot undo expiry approval.
	j2 := join()
	if j2.NodeID != j.NodeID {
		t.Fatalf("rejoin should keep node identity: %s vs %s", j2.NodeID, j.NodeID)
	}
	if j2.Approved {
		t.Fatal("saved credential bypassed admin expiry")
	}
	if code, _ := getMap(t, base, j2, headers); code != 403 {
		t.Fatalf("renewal without approval returned %d", code)
	}
	post(t, base+"/admin/api/nodes/approve", map[string]string{"node_id": j2.NodeID}, auth)
	if code, m := getMap(t, base, j2, headers); code != 200 {
		t.Fatalf("renewed node should pass: %d", code)
	} else if m.Self.KeyExpiry == nil || time.Until(*m.Self.KeyExpiry) < 24*time.Hour {
		t.Fatalf("renewal did not extend expiry: %+v", m.Self.KeyExpiry)
	}
}
