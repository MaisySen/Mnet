package daemon

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/relay"
	"golang.zx2c4.com/wireguard/tun/tuntest"
)

func TestDaemonDiscoversDirectCandidates(t *testing.T) {
	store, err := control.Open(t.TempDir() + "/control.json")
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.CreateNetwork("discovery")
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAuthKey(network.ID, "fixture", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	server, _, err := control.NewServer(store, relay.NewHub(), "http://fixture", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	var nodes []*Daemon
	for _, name := range []string{"a", "b"} {
		ct := tuntest.NewChannelTUN()
		d, err := New(Config{StateDir: t.TempDir(), TUN: ct.TUN()})
		if err != nil {
			t.Fatal(err)
		}
		if err = d.Up(ts.URL, network.Name, name, key.Key); err != nil {
			t.Fatal(err)
		}
		defer d.Down()
		nodes = append(nodes, d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for {
		ready := true
		for _, d := range nodes {
			m := d.currentMap()
			if m == nil || len(m.Peers) != 1 || len(m.Peers[0].Endpoints) == 0 {
				ready = false
			}
		}
		if ready {
			return
		}
		select {
		case <-ctx.Done():
			for _, d := range nodes {
				m := d.currentMap()
				if m != nil {
					t.Logf("map peers: %+v error: %s", m.Peers, d.Status().LastError)
				}
			}
			t.Fatal("daemon did not publish/discover direct candidates")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
