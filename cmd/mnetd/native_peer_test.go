package main

// A proof-only userspace peer for the native kernel-interface CI fixture.
// It has no OS TCP listener: connections must cross WireGuard to proofnet.
import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"golang.zx2c4.com/wireguard/tun"
	"io"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
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
	trace := &nativePeerTrace{}
	peer, err := newProofNodeWithTUN(config.Root, "userspace-peer", config.Server, config.Network, "userspace-peer", config.Key, false, func(dev tun.Device) tun.Device { trace.Device = dev; return trace })
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
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			data, _ := json.Marshal(trace.snapshot())
			// Private staging file is read only through a fixed boolean allowlist.
			_ = os.WriteFile(filepath.Join(config.Root, "peer-packets.json"), data, 0o600)
		}
	}
}

func nativePeerResponse() []byte {
	return bytes.Repeat([]byte("reverse-native-peer-proof"), (4<<20)/25+1)[:4<<20]
}

func serveNativePeerHash(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	hash := sha256.New()
	if _, err := io.CopyN(hash, conn, 4<<20); err != nil {
		return
	}
	if _, err := conn.Write(hash.Sum(nil)); err != nil {
		return
	}
	response := nativePeerResponse()
	if _, err := io.Copy(conn, bytes.NewReader(response)); err != nil {
		return
	}
	got := make([]byte, sha256.Size)
	expected := sha256.Sum256(response)
	if _, err := io.ReadFull(conn, got); err != nil || !bytes.Equal(got, expected[:]) {
		return
	}
	_, _ = conn.Write([]byte("OK"))
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
		response := make([]byte, 4<<20)
		if _, err := io.ReadFull(client, response); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(response, nativePeerResponse()) {
			t.Fatal("reverse payload differs")
		}
		reverseHash := sha256.Sum256(response)
		if _, err := client.Write(reverseHash[:]); err != nil {
			t.Fatal(err)
		}
		ack := make([]byte, 2)
		if _, err := io.ReadFull(client, ack); err != nil || string(ack) != "OK" {
			t.Fatal("reverse receiver hash not acknowledged")
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

func TestNativePeerHashRejectsReverseMismatch(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go serveNativePeerHash(server)
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.CopyN(client, bytes.NewReader(make([]byte, 4<<20)), 4<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(io.Discard, client, sha256.Size+(4<<20)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(make([]byte, sha256.Size)); err != nil {
		t.Fatal(err)
	}
	ack := make([]byte, 2)
	if n, err := client.Read(ack); n != 0 || err == nil {
		t.Fatal("invalid reverse receiver hash accepted")
	}
}

// Traces only the disposable fixture's packet boundaries, never packet bytes.
type nativePeerTrace struct {
	tun.Device
	inboundTCP, outboundTCP, inboundSYN, outboundSYNACK atomic.Bool
}

func nativePeerTCP(packet []byte) (tcp, syn, synack bool) {
	if len(packet) < 40 || packet[0]>>4 != 4 || packet[9] != 6 {
		return
	}
	h := int(packet[0]&15) * 4
	if h < 20 || len(packet) < h+20 {
		return
	}
	src, dst := int(packet[h])<<8|int(packet[h+1]), int(packet[h+2])<<8|int(packet[h+3])
	if src != 45993 && dst != 45993 {
		return
	}
	flags := packet[h+13]
	return true, flags&2 != 0 && flags&16 == 0, flags&18 == 18
}
func (p *nativePeerTrace) Read(b [][]byte, sizes []int, offset int) (int, error) {
	n, err := p.Device.Read(b, sizes, offset)
	for i := 0; i < n; i++ {
		tcp, _, ack := nativePeerTCP(b[i][offset : offset+sizes[i]])
		if tcp {
			p.outboundTCP.Store(true)
		}
		if ack {
			p.outboundSYNACK.Store(true)
		}
	}
	return n, err
}
func (p *nativePeerTrace) Write(b [][]byte, offset int) (int, error) {
	for _, packet := range b {
		tcp, syn, _ := nativePeerTCP(packet[offset:])
		if tcp {
			p.inboundTCP.Store(true)
		}
		if syn {
			p.inboundSYN.Store(true)
		}
	}
	return p.Device.Write(b, offset)
}
func (p *nativePeerTrace) snapshot() map[string]bool {
	return map[string]bool{"peerInboundTCP": p.inboundTCP.Load(), "peerOutboundTCP": p.outboundTCP.Load(), "peerInboundSYN": p.inboundSYN.Load(), "peerOutboundSYNACK": p.outboundSYNACK.Load()}
}
func TestNativePeerTCPFlags(t *testing.T) {
	packet := make([]byte, 40)
	packet[0] = 0x45
	packet[9] = 6
	packet[22] = byte(45993 >> 8)
	packet[23] = byte(45993 & 255)
	packet[33] = 2
	if tcp, syn, ack := nativePeerTCP(packet); !tcp || !syn || ack {
		t.Fatal("SYN classification")
	}
	packet[33] = 18
	if tcp, syn, ack := nativePeerTCP(packet); !tcp || syn || !ack {
		t.Fatal("SYNACK classification")
	}
	for _, bad := range [][]byte{nil, packet[:30], make([]byte, 40)} {
		if tcp, _, _ := nativePeerTCP(bad); tcp {
			t.Fatal("invalid packet accepted")
		}
	}
	if len((&nativePeerTrace{}).snapshot()) != 4 {
		t.Fatal("diagnostic disclosure changed")
	}
}
