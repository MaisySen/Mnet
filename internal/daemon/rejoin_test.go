package daemon_test

// Rejoin: when the control plane forgets a node (state wipe, admin
// delete), a daemon that joined with an auth key must notice the 401
// and register itself again without manual intervention.

import (
	"bytes"

	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/tun/tuntest"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

func TestDaemonRejoinsAfterNodeDelete(t *testing.T) {
	store, err := control.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	srv, _, err := control.NewServer(store, relay.NewHub(), "http://placeholder", "pw")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := ts.URL

	lb, _ := json.Marshal(map[string]string{"password": "pw"})
	lr, err := http.Post(base+"/admin/api/login", "application/json", bytes.NewReader(lb))
	if err != nil {
		t.Fatal(err)
	}
	cookie := lr.Cookies()[0].String()
	lr.Body.Close()
	netBody := adminPost(t, base, cookie, "/admin/api/networks", map[string]string{"name": "home"})
	var network netmap.Network
	json.Unmarshal(netBody, &network)
	keyBody := adminPost(t, base, cookie, "/admin/api/authkeys", map[string]any{"network_id": network.ID, "reusable": true})
	var key netmap.AuthKey
	json.Unmarshal(keyBody, &key)

	ct := tuntest.NewChannelTUN()
	d, err := daemon.New(daemon.Config{
		StateDir:     t.TempDir(),
		TUN:          ct.TUN(),
		ManagedIface: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Up(base, "home", "rejoiner", key.Key); err != nil {
		t.Fatalf("up: %v", err)
	}
	defer d.Down()

	state := func() []netmap.Node {
		req, _ := http.NewRequest(http.MethodPost, base+"/admin/api/state", nil)
		req.Header.Set("Cookie", cookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var snap struct {
			Nodes []netmap.Node `json:"nodes"`
		}
		json.NewDecoder(resp.Body).Decode(&snap)
		return snap.Nodes
	}

	nodes := state()
	if len(nodes) != 1 || nodes[0].Name != "rejoiner" {
		t.Fatalf("after join: %+v", nodes)
	}
	firstID := nodes[0].ID

	// The server forgets the node.
	adminPost(t, base, cookie, "/admin/api/nodes/delete", map[string]string{"node_id": firstID})

	// The daemon must re-join on its own: same pubkey, new node id.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range state() {
			if n.Name == "rejoiner" && n.ID != firstID && n.Approved {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("daemon did not re-join within 30s")
}

