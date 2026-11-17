//go:build linux

package main

// Local regression of the exact mobile fd adapter against real WireGuard and
// a userspace TCP peer. This is not Android kernel/VpnService evidence.
import (
	"context"
	"encoding/json"
	localdns "fintex.au/mnet/internal/dns"
	mdns "github.com/miekg/dns"
	"io"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/proofnet"
	"fintex.au/mnet/internal/relay"
	"fintex.au/mnet/mobile"
)

func TestMobileFDMeshTCP(t *testing.T) {
	dir := t.TempDir()
	store, err := control.Open(filepath.Join(dir, "control.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.CreateNetwork("fdmesh")
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAuthKey(network.ID, "fixture", true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewUnstartedServer(nil)
	server, _, err := control.NewServer(store, relay.NewHub(), "http://"+httpServer.Listener.Addr().String(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	httpServer.Config.Handler = server.Handler()
	httpServer.Start()
	defer httpServer.Close()
	peer, err := newProofNode(dir, "peer", httpServer.URL, "fdmesh", "peer", key.Key, false)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.d.Down()
	listener, err := peer.net.ListenTCPAddrPort(netip.AddrPortFrom(peer.ip, 45993))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		c, e := listener.Accept()
		if e == nil {
			defer c.Close()
			io.Copy(c, c)
		}
	}()
	node, err := mobile.NewNode(filepath.Join(dir, "mobile"))
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	ip, err := node.Prepare(httpServer.URL, "fdmesh", "mobile", key.Key)
	if err != nil {
		t.Fatal(err)
	}
	osTun, osNet, err := proofnet.CreateNetTUN([]netip.Addr{netip.MustParseAddr(ip)}, nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer osTun.Close()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Android VpnService supplies a nonblocking fd. Match that behavior.
	if err = syscall.SetNonblock(fds[0], true); err != nil {
		t.Fatal(err)
	}
	if err = syscall.SetNonblock(fds[1], true); err != nil {
		t.Fatal(err)
	}
	osFD := os.NewFile(uintptr(fds[1]), "fixture-os")
	defer osFD.Close()
	go func() {
		b := [][]byte{make([]byte, 65535)}
		sizes := make([]int, 1)
		for {
			n, e := osTun.Read(b, sizes, 0)
			if e != nil || n != 1 {
				return
			}
			if _, e = osFD.Write(b[0][:sizes[0]]); e != nil {
				return
			}
		}
	}()
	go func() {
		b := make([]byte, 65535)
		for {
			n, e := osFD.Read(b)
			if e != nil {
				return
			}
			if _, e = osTun.Write([][]byte{b[:n]}, 0); e != nil {
				return
			}
		}
	}()
	if err = node.Start(fds[0], httpServer.URL, "fdmesh", "mobile", key.Key); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var ready struct {
		DNSReady bool `json:"dns_ready"`
	}
	if json.Unmarshal([]byte(node.Status()), &ready) != nil || !ready.DNSReady {
		t.Fatal("mobile DNS readiness unavailable")
	}
	{
		transport := "tcp"
		resolver := netip.AddrPortFrom(netip.MustParseAddr(localdns.LocalIPv4), 53)
		c, err := osNet.DialContextTCPAddrPort(ctx, resolver)
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(2 * time.Second))
		dnsConn := &mdns.Conn{Conn: c}
		q := new(mdns.Msg)
		q.SetQuestion("peer.fdmesh.mnet.", mdns.TypeA)
		if err = dnsConn.WriteMsg(q); err != nil {
			c.Close()
			t.Fatal(err)
		}
		response, e := dnsConn.ReadMsg()
		c.Close()
		if e != nil || response == nil || len(response.Answer) != 1 || response.Answer[0].(*mdns.A).A.String() != peer.ip.String() {
			t.Fatalf("mobile %s DNS response failed: %v", transport, e)
		}
	}
	conn, err := osNet.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(peer.ip, 45993))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = conn.Write([]byte("fdmesh")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 6)
	if _, err = io.ReadFull(conn, got); err != nil || string(got) != "fdmesh" {
		t.Fatalf("TCP echo: %v", err)
	}
	var d map[string]bool
	json.Unmarshal([]byte(node.PacketDiagnostics()), &d)
	if !d["read_source_self"] || !d["read_destination_peer"] || !d["read_packet"] || !d["wrote_packet"] || d["read_failed"] || d["write_failed"] {
		t.Fatal("mobile fd packet evidence missing")
	}
}
