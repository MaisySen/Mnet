package dns

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
)

// Test-only dual-protocol allocation. UDP and TCP ephemeral pools are independent:
// a port free in one can be occupied in the other. Retry only that setup clash,
// before either DNS server starts. Fixed-port production bind tests keep startAt.
func startTestDNS(t *testing.T, s *Server) {
	t.Helper()
	if err := startTestDNSWith(s, net.Listen, net.ListenPacket); err != nil {
		t.Fatal(err)
	}
}
func startTestDNSWith(s *Server, listen func(string, string) (net.Listener, error), packet func(string, string) (net.PacketConn, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.done != nil {
		return fmt.Errorf("test DNS unavailable")
	}
	for attempt := 0; attempt < 8; attempt++ {
		tcp, err := listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		udp, err := packet("udp", tcp.Addr().String())
		if err != nil {
			tcp.Close()
			if errors.Is(err, syscall.EADDRINUSE) {
				continue
			}
			return err
		}
		return s.startBoundLocked(udp, tcp)
	}
	return fmt.Errorf("test DNS could not reserve both transports")
}

func TestEphemeralSetupClosesPartialListenerAndRetriesOnlyAddressInUse(t *testing.T) {
	s, _ := New("127.0.0.1")
	defer s.Close()
	calls := 0
	var first net.Listener
	listen := func(network, address string) (net.Listener, error) {
		l, e := net.Listen(network, address)
		if calls == 0 {
			first = l
		}
		return l, e
	}
	packet := func(network, address string) (net.PacketConn, error) {
		calls++
		if calls == 1 {
			return nil, syscall.EADDRINUSE
		}
		return net.ListenPacket(network, address)
	}
	if err := startTestDNSWith(s, listen, packet); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !s.Ready() {
		t.Fatal("setup did not recover exact clash")
	}
	if _, err := first.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatal("partial listener not closed")
	}
}
func TestEphemeralSetupDoesNotRetryOtherErrors(t *testing.T) {
	s, _ := New("127.0.0.1")
	defer s.Close()
	calls := 0
	err := startTestDNSWith(s, net.Listen, func(string, string) (net.PacketConn, error) { calls++; return nil, syscall.EACCES })
	if !errors.Is(err, syscall.EACCES) || calls != 1 || s.Ready() {
		t.Fatal("unrelated failure hidden")
	}
}
