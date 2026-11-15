package daemon_test

// Mobile flow: Prepare joins to learn the mesh address, the app builds
// the OS tunnel with it, then Up joins again on the injected tunnel.
// A one-time auth key is consumed by Prepare, so Up must re-join as the
// already known node instead of failing with 403.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.zx2c4.com/wireguard/tun/tuntest"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

func TestPrepareThenUpWithOneTimeKey(t *testing.T) {
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
	// reusable left false: a one-time key, the mobile default.
	keyBody := adminPost(t, base, cookie, "/admin/api/authkeys", map[string]any{"network_id": network.ID})
	var key netmap.AuthKey
	json.Unmarshal(keyBody, &key)

	d, err := daemon.New(daemon.Config{StateDir: t.TempDir(), ManagedIface: false})
	if err != nil {
		t.Fatal(err)
	}
	ip, err := d.Prepare(base, "home", "phone", key.Key)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if ip == "" {
		t.Fatal("prepare returned no address")
	}

	ct := tuntest.NewChannelTUN()
	if err := d.SetTUN(ct.TUN()); err != nil {
		t.Fatal(err)
	}
	// Same one-time key: Prepare already consumed it.
	if err := d.Up(base, "home", "phone", key.Key); err != nil {
		t.Fatalf("up after prepare with one-time key: %v", err)
	}
	defer d.Down()
	if st := d.Status(); st.Status != "up" || st.IPv4 != ip {
		t.Fatalf("status %+v, want up on %s", st, ip)
	}
}
