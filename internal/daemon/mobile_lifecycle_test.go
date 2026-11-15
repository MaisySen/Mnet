package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestPrepareSerializesIdentityAndPublishesStatus(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
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
		close(release)
		<-result
		t.Fatalf("Prepare status %q, want preparing", got)
	}
	if err := d.SetRouting(false, nil, ""); err == nil {
		t.Error("routing changed during Prepare")
	}
	downDone := make(chan struct{})
	go func() { d.Down(); close(downDone) }()
	select {
	case <-downDone:
		t.Error("Down did not wait for Prepare lifecycle")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-downDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Down blocked after Prepare")
	}
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
