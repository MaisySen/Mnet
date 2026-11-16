package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/netmap"
)

func TestMapAuthorizationDenialWithdrawsAndRecovers(t *testing.T) {
	for _, mode := range []string{"forbidden", "unapproved-map"} {
		t.Run(mode, func(t *testing.T) {
			d := testDaemon(t)
			d.status = "up"
			key := pubKeyOf(t, 0x53)
			approved := netmap.Map{Version: 7, Self: netmap.Node{Approved: true, IPv4: "100.64.0.1"}, Peers: []netmap.Node{{PubKey: key, IPv4: "100.64.0.2"}}}
			d.applyMap(&approved)
			if len(allowedIPsOf(t, d, key)) != 1 {
				t.Fatal("fixture peer not installed")
			}
			var calls atomic.Int32
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n == 1 {
					if mode == "forbidden" {
						w.WriteHeader(403)
						return
					}
					denied := approved
					denied.Version = 8
					denied.Self.Approved = false
					// Even if a denied map contains peers, none may be installed.
					json.NewEncoder(w).Encode(denied)
					return
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if n == 2 {
					restored := approved
					restored.Version = 9
					json.NewEncoder(w).Encode(restored)
					return
				}
				<-r.Context().Done()
			}))
			defer srv.Close()
			d.cc = &controlclient.Client{Base: srv.URL}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); d.mapLoop(ctx) }()
			defer func() { cancel(); <-done }()
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if len(allowedIPsOf(t, d, key)) == 0 && d.Status().Status == "waiting-approval" {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if len(allowedIPsOf(t, d, key)) != 0 || d.Status().Status != "waiting-approval" {
				t.Fatal("received denial preserved old peer authorization or up status")
			}
			d.refreshPaths()
			if len(allowedIPsOf(t, d, key)) != 0 {
				t.Fatal("path refresh restored denied peer")
			}
			close(release)
			deadline = time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if len(allowedIPsOf(t, d, key)) == 1 && d.Status().Status == "up" {
					return
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatal("fresh approved map did not restore authorization/status")
		})
	}
}
