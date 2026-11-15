package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestPrepareSerializesIdentityAndPublishesStatus(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/join":
			close(entered)
			<-release
			json.NewEncoder(w).Encode(netmap.JoinResponse{NodeID: "prepared", NodeToken: "fixture", Approved: true})
		case "/api/map":
			json.NewEncoder(w).Encode(netmap.Map{Version: 1, Self: netmap.Node{Approved: true, IPv4: "100.64.0.7"}})
		default:
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	defer unblock()
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := d.Prepare(server.URL, "fixture", "phone", ""); result <- err }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("Prepare did not join")
	}
	if got := d.Status().Status; got != "preparing" {
		unblock()
		<-result
		t.Fatalf("Prepare status %q, want preparing", got)
	}
	if err := d.SetRouting(false, nil, ""); err == nil {
		t.Error("routing changed during Prepare")
	}
	unblock()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	d.Down()
	if d.Status().Status != "down" {
		t.Fatal("Prepare did not finish down")
	}
	routes := []string{"10.8.0.0/24"}
	if err := d.SetRouting(false, routes, ""); err != nil {
		t.Fatal(err)
	}
	routes[0] = "10.7.0.0/24"
	snapshot := d.State()
	snapshot.AdvertiseRoutes[0] = "10.9.0.0/24"
	if d.State().AdvertiseRoutes[0] != "10.8.0.0/24" {
		t.Fatal("state snapshot aliases mutable routes")
	}
}

func TestDownCancelsBlockedPrepare(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := d.Prepare(server.URL, "fixture", "phone", ""); finished <- err }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("Prepare did not enter blocked join")
	}
	down := make(chan struct{})
	go func() { d.Down(); close(down) }()
	select {
	case <-down:
	case <-time.After(time.Second):
		d.mu.Lock()
		cancel := d.cancel
		d.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		server.CloseClientConnections()
		t.Fatal("Down did not cancel blocked Prepare")
	}
	if err := <-finished; err == nil {
		t.Fatal("cancelled Prepare succeeded")
	}
	if d.Status().Status != "down" {
		t.Fatal("cancelled Prepare not down")
	}
}
