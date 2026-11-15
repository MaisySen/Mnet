package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/relay"
	"fintex.au/mnet/internal/session"
)

// Opt-in stress over the actual userspace mesh, without an X11 dependency.
// This is diagnostic coverage, not an end-user platform validation.
func TestMeshFileStress(t *testing.T) {
	if os.Getenv("MNET_MESH_STRESS") != "1" {
		t.Skip("set MNET_MESH_STRESS=1 for userspace mesh stress")
	}
	dir := t.TempDir()
	store, err := control.Open(filepath.Join(dir, "control.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.CreateNetwork("stress")
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAuthKey(network.ID, "stress", true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewUnstartedServer(nil)
	server, _, err := control.NewServer(store, relay.NewHub(), "http://"+httpServer.Listener.Addr().String(), "testpassword")
	if err != nil {
		t.Fatal(err)
	}
	httpServer.Config.Handler = server.Handler()
	httpServer.Start()
	defer httpServer.Close()
	host, err := newProofNode(dir, "host", httpServer.URL, "stress", "stress-host", key.Key, false)
	if err != nil {
		t.Fatal(err)
	}
	defer host.d.Down()
	viewer, err := newProofNode(dir, "viewer", httpServer.URL, "stress", "stress-viewer", key.Key, false)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.d.Down()
	ln, err := host.net.ListenTCPAddrPort(netip.AddrPortFrom(host.ip, proofPort))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hostMux := make(chan *session.Mux, 1)
	hostErr := make(chan error, 1)
	received := filepath.Join(dir, "received")
	go func() {
		m, e := session.Listen(ln, "stress-host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		if e != nil {
			hostErr <- e
			return
		}
		m.Keepalive(time.Second, 60*time.Second)
		m.OnStream(func(s session.Stream) {
			if s.ID >= session.StreamFile {
				session.FileReceiver{Mux: m, Dir: received}.Handle(s)
			}
		})
		hostMux <- m
	}()
	deadline := time.Now().Add(30 * time.Second)
	var client *session.Mux
	for {
		conn, e := viewer.net.DialTCPAddrPort(netip.AddrPortFrom(host.ip, proofPort))
		if e == nil {
			client, e = session.Dial(conn, "stress-viewer")
			if e == nil {
				break
			}
			conn.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("mesh dial: %v", e)
		}
		time.Sleep(time.Second)
	}
	defer client.Close()
	client.Keepalive(time.Second, 60*time.Second)
	select {
	case m := <-hostMux:
		defer m.Close()
	case err := <-hostErr:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("host setup")
	}
	for i := 0; i < 100; i++ {
		size := 256*1024 + i*97
		data := bytes.Repeat([]byte{byte(i)}, size)
		name := fmt.Sprintf("payload-%03d.bin", i)
		src := filepath.Join(dir, name)
		if err := os.WriteFile(src, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := session.SendFile(client, src); err != nil {
			t.Fatalf("transfer %d: %v; host=%+v viewer=%+v", i, err, host.d.PeerStatuses(), viewer.d.PeerStatuses())
		}
		got, err := os.ReadFile(filepath.Join(received, name))
		if err != nil || !bytes.Equal(data, got) {
			t.Fatalf("transfer %d mismatched: %v", i, err)
		}
	}
}
