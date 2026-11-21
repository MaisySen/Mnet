//go:build multies_native_fixture

package control

// An explicitly selected disposable TCP counterpart. Its WireGuard key is the
// same first peer already proved to the Store; no legacy registration occurs.
import (
	"crypto/sha256"
	"encoding/hex"
	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/proofnet"
	"golang.zx2c4.com/wireguard/device"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

type registrarTCPFixture struct {
	listener  net.Listener
	mu        sync.Mutex
	closed    bool
	conn      net.Conn
	exited    chan struct{}
	done      chan error
	exchanged chan struct{}
	once      sync.Once
}

func newRegistrarTCPTransportFixture(t *testing.T, key [32]byte, ip string) *registrarTransportFixture {
	t.Helper()
	injected, network, err := proofnet.CreateNetTUN([]netip.Addr{netip.MustParseAddr(ip)}, nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := network.ListenTCPAddrPort(netip.AddrPortFrom(netip.MustParseAddr(ip), 49173))
	if err != nil {
		injected.Close()
		t.Fatal(err)
	}
	tcp := &registrarTCPFixture{listener: listener, done: make(chan error, 1), exited: make(chan struct{}), exchanged: make(chan struct{}, 1)}
	f := &registrarTransportFixture{bind: bind.New(), tcp: tcp}
	f.dev = device.NewDevice(injected, f.bind, device.NewLogger(device.LogLevelSilent, "enrolled-tcp-peer: "))
	if err := f.dev.IpcSetOperation(strings.NewReader("private_key=" + hex.EncodeToString(key[:]) + "\nlisten_port=0\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.dev.Up(); err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(tcp.exited)
		conn, err := listener.Accept()
		if err != nil {
			tcp.done <- err
			return
		}
		tcp.mu.Lock()
		if tcp.closed {
			tcp.mu.Unlock()
			conn.Close()
			tcp.done <- net.ErrClosed
			return
		}
		tcp.conn = conn
		tcp.mu.Unlock()
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		var challenge [32]byte
		if _, err = io.ReadFull(conn, challenge[:]); err != nil {
			tcp.done <- err
			return
		}
		response := sha256.Sum256(challenge[:])
		if _, err = conn.Write(response[:]); err != nil {
			tcp.done <- err
			return
		}
		tcp.exchanged <- struct{}{}
		var final [1]byte
		_, err = conn.Read(final[:])
		tcp.done <- err
	}()
	t.Cleanup(func() {
		tcp.Close()
		f.dev.Close()
		select {
		case <-tcp.exited:
		case <-time.After(time.Second):
			t.Error("TCP counterpart owner did not join")
		}
	})
	return f
}
func (tcp *registrarTCPFixture) Close() {
	tcp.once.Do(func() {
		tcp.listener.Close()
		tcp.mu.Lock()
		tcp.closed = true
		if tcp.conn != nil {
			tcp.conn.Close()
		}
		tcp.mu.Unlock()
	})
}
