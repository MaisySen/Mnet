package daemon

import (
	"context"
	"encoding/json"
	"golang.zx2c4.com/wireguard/tun/tuntest"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

// Expiry already present in a received map must not depend on another
// successful control request. Check actual WireGuard allowed-IP state.
func TestApplyMapRejectsExpiredAuthorization(t *testing.T) {
	for _, self := range []bool{false, true} {
		name := "peer"
		if self {
			name = "self"
		}
		t.Run(name, func(t *testing.T) {
			d := testDaemon(t)
			key := pubKeyOf(t, 0x41)
			expiry := time.Now().Add(-time.Second)
			m := &netmap.Map{Version: 1, Self: netmap.Node{IPv4: "100.64.0.1"}, Peers: []netmap.Node{{PubKey: key, IPv4: "100.64.0.2"}}}
			if self {
				m.Self.KeyExpiry = &expiry
			} else {
				m.Peers[0].KeyExpiry = &expiry
			}
			d.applyMap(m)
			if ips := allowedIPsOf(t, d, key); len(ips) != 0 {
				t.Fatalf("expired %s authorized in WireGuard: %v", name, ips)
			}
			if len(d.PeerStatuses()) != 0 {
				t.Fatal("expired authorization retained in status")
			}
		})
	}
}

func startExpiryGuard(t *testing.T, d *Daemon) {
	t.Helper()
	d.mapChanged = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); d.expiryLoop(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("expiry guard did not stop")
		}
	})
}

func awaitPeerRemoval(t *testing.T, d *Daemon, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(allowedIPsOf(t, d, key)) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("expiry did not remove WireGuard peer while no new maps arrived")
}

func TestLocalExpiryWithoutControlResponse(t *testing.T) {
	for _, self := range []bool{false, true} {
		name := "peer"
		if self {
			name = "self"
		}
		t.Run(name, func(t *testing.T) {
			d := testDaemon(t)
			d.status = "up"
			d.state.UseExitNode = "gateway"
			key := pubKeyOf(t, 0x42)
			survivor := pubKeyOf(t, 0x43)
			expiry := time.Now().Add(120 * time.Millisecond)
			m := &netmap.Map{Version: 7, Self: netmap.Node{IPv4: "100.64.0.1"}, Peers: []netmap.Node{{Name: "gateway", ExitNode: true, Routes: []string{"10.9.0.0/24"}, PubKey: key, IPv4: "100.64.0.2"}, {PubKey: survivor, IPv4: "100.64.0.3"}}}
			if self {
				m.Self.KeyExpiry = &expiry
			} else {
				m.Peers[0].KeyExpiry = &expiry
			}
			d.applyMap(m)
			if len(allowedIPsOf(t, d, key)) != 4 {
				t.Fatal("future-expiry peer/exit/subnet authorization not installed")
			}
			startExpiryGuard(t, d)
			awaitPeerRemoval(t, d, key)
			if len(m.Peers) != 2 {
				t.Fatal("guard mutated source map")
			}
			d.refreshPaths()
			d.applyMap(m) // A repeated old response must not resurrect an expired key.
			if len(allowedIPsOf(t, d, key)) != 0 {
				t.Fatal("stale map/path refresh resurrected expired key")
			}
			if self {
				if len(allowedIPsOf(t, d, survivor)) != 0 || d.Status().Status != "key-expired" {
					t.Fatal("self expiry retained authorization/status")
				}
			} else if len(allowedIPsOf(t, d, survivor)) != 1 {
				t.Fatal("unrelated peer was removed")
			}
		})
	}
}

func TestExpiryGuardRenewsAndWakesFromNoDeadline(t *testing.T) {
	d := testDaemon(t)
	d.status = "up"
	key := pubKeyOf(t, 0x44)
	startExpiryGuard(t, d) // No map and no timer initially.
	oldDeadline := time.Now().Add(80 * time.Millisecond)
	d.applyMap(&netmap.Map{Version: 1, Self: netmap.Node{Approved: true, KeyExpiry: &oldDeadline}, Peers: []netmap.Node{{PubKey: key, IPv4: "100.64.0.2"}}})
	newDeadline := time.Now().Add(350 * time.Millisecond)
	d.applyMap(&netmap.Map{Version: 2, Self: netmap.Node{Approved: true, KeyExpiry: &newDeadline}, Peers: []netmap.Node{{PubKey: key, IPv4: "100.64.0.2"}}})
	time.Sleep(150 * time.Millisecond)
	if len(allowedIPsOf(t, d, key)) != 1 || d.Status().Status != "up" {
		t.Fatal("old deadline removed renewed authorization")
	}
	awaitPeerRemoval(t, d, key)
	future := time.Now().Add(time.Hour)
	d.applyMap(&netmap.Map{Version: 3, Self: netmap.Node{Approved: true, KeyExpiry: &future}, Peers: []netmap.Node{{PubKey: key, IPv4: "100.64.0.2"}}})
	if len(allowedIPsOf(t, d, key)) != 1 || d.Status().Status != "up" {
		t.Fatal("fresh authorized map did not restore renewed identity")
	}
}

func TestExpiryEqualityAndNoDeadlineCompatibility(t *testing.T) {
	now := time.Now()
	key := pubKeyOf(t, 0x45)
	m := &netmap.Map{Self: netmap.Node{}, Peers: []netmap.Node{{PubKey: key, KeyExpiry: &now}, {PubKey: "legacy"}}}
	got := unexpiredMap(m, now)
	if len(got.Peers) != 1 || got.Peers[0].PubKey != "legacy" {
		t.Fatal("expiry at exact boundary accepted or legacy rejected")
	}
	if !nextMapExpiry(m, now).Equal(now) {
		t.Fatal("elapsed deadline was lost while arming timer")
	}
	if !nextMapExpiry(got, now).IsZero() {
		t.Fatal("no-expiry map armed a timer")
	}
}

// Exercise actual New/Up/Down, not just a directly started guard. All map
// polls after the initial response stay blocked until their context ends.
func TestDaemonExpiryWithBlockedControlAndRestart(t *testing.T) {
	key := pubKeyOf(t, 0x46)
	var polls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/join":
			json.NewEncoder(w).Encode(netmap.JoinResponse{NodeID: "fixture", NodeToken: "test-token", Approved: true})
		case "/api/map":
			if r.URL.Query().Get("since") == "0" {
				expiry := time.Now().Add(250 * time.Millisecond)
				json.NewEncoder(w).Encode(netmap.Map{Version: 1, Network: netmap.Network{Name: "test"}, Self: netmap.Node{Approved: true, IPv4: "100.64.0.1", KeyExpiry: &expiry}, Peers: []netmap.Node{{Name: "peer", PubKey: key, IPv4: "100.64.0.2"}}})
			} else {
				polls.Add(1)
				<-r.Context().Done()
			}
		default:
			http.Error(w, "test control unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer ts.Close()
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Down()
	for cycle := 0; cycle < 2; cycle++ {
		ct := tuntest.NewChannelTUN()
		if err := d.SetTUN(ct.TUN()); err != nil {
			t.Fatal(err)
		}
		if err := d.Up(ts.URL, "test", "test", ""); err != nil {
			t.Fatal(err)
		}
		awaitPeerRemoval(t, d, key)
		if d.Status().Status != "key-expired" {
			t.Fatal("self expiry did not change status")
		}
		stopped := make(chan struct{})
		go func() { d.Down(); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("Down blocked with expired identity and blocked control poll")
		}
		if d.Status().Status != "down" {
			t.Fatal("Down did not finish")
		}
	}
	if polls.Load() < 2 {
		t.Fatal("fixture did not block control polls")
	}
}
