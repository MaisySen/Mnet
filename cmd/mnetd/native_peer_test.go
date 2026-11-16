package main

// A proof-only userspace peer for the native kernel-interface CI fixture.
// It has no OS TCP listener: connections must cross WireGuard to proofnet.
import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestNativeKernelPeerFixture(t *testing.T) {
	path := os.Getenv("MNET_NATIVE_PEER_CONFIG")
	if path == "" {
		t.Skip("disposable native kernel CI fixture only")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("requires disposable GitHub Actions runner")
	}
	var config struct{ Server, Network, Key, Root string }
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read private fixture configuration")
	}
	if json.Unmarshal(data, &config) != nil || config.Server == "" || config.Network == "" || config.Key == "" || config.Root == "" {
		t.Fatal("invalid private fixture configuration")
	}
	peer, err := newProofNode(config.Root, "userspace-peer", config.Server, config.Network, "userspace-peer", config.Key, false)
	if err != nil {
		t.Fatal("proof peer setup failed")
	}
	defer peer.d.Down()
	listener, err := peer.net.ListenTCPAddrPort(netip.AddrPortFrom(peer.ip, 45993))
	if err != nil {
		t.Fatal("proof peer listen failed")
	}
	defer listener.Close()
	ready, _ := json.Marshal(map[string]any{"ipv4": peer.ip.String(), "port": 45993})
	if os.WriteFile(filepath.Join(config.Root, "peer-ready.json"), ready, 0o600) != nil {
		t.Fatal("write private fixture readiness")
	}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(done)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveNativePeerHash(conn)
		}
	}()
	<-done
}

func serveNativePeerHash(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	hash := sha256.New()
	if _, err := io.CopyN(hash, conn, 4<<20); err == nil {
		_, _ = conn.Write(hash.Sum(nil))
	}
}

func TestNativePeerHashProtocol(t *testing.T) {
	t.Run("receiver hash acknowledges exact payload", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		go serveNativePeerHash(server)
		payload := bytes.Repeat([]byte("kernel-peer-proof"), (4<<20)/17+1)[:4<<20]
		_ = client.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := client.Write(payload); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, sha256.Size)
		if _, err := io.ReadFull(client, got); err != nil {
			t.Fatal(err)
		}
		expected := sha256.Sum256(payload)
		if !bytes.Equal(got, expected[:]) {
			t.Fatal("receiver hash differs")
		}
	})
	t.Run("truncated payload does not acknowledge", func(t *testing.T) {
		client, server := net.Pipe()
		done := make(chan struct{})
		go func() { serveNativePeerHash(server); close(done) }()
		_ = client.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := client.Write([]byte("short")); err != nil {
			t.Fatal(err)
		}
		client.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("truncated receiver leaked")
		}
	})
}
