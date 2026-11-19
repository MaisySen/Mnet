package dns

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
)

// Test-only dual-protocol allocation. UDP and TCP ephemeral pools are independent:
// a port free in one can be occupied or Windows-reserved in the other. Retry
// only a companion UDP port clash/WSAEACCES after closing its TCP reservation,
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
			if errors.Is(err, syscall.EADDRINUSE) || testUDPCompanionPortDenied(err) {
				continue
			}
			return err
		}
		return s.startBoundLocked(udp, tcp)
	}
	return fmt.Errorf("test DNS could not reserve both transports")
}

// WSAEACCES is specific to this test-only companion UDP bind. Do not retry
// generic permissions, TCP allocation errors, production binds or DNS queries.
func testUDPCompanionPortDenied(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "listen" && op.Net == "udp" && errors.Is(op.Err, syscall.Errno(10013))
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

func TestEphemeralSetupRetriesWindowsDeniedCompanionPort(t *testing.T) {
	s, _ := New("127.0.0.1")
	defer s.Close()
	calls := 0
	var first net.Listener
	listen := func(network, address string) (net.Listener, error) {
		l, err := net.Listen(network, address)
		if calls == 0 {
			first = l
		}
		return l, err
	}
	packet := func(network, address string) (net.PacketConn, error) {
		calls++
		if calls == 1 {
			return nil, &net.OpError{Op: "listen", Net: "udp", Err: syscall.Errno(10013)}
		}
		return net.ListenPacket(network, address)
	}
	if err := startTestDNSWith(s, listen, packet); err != nil {
		t.Fatal("test-only companion transport did not reselect a port", err)
	}
	if calls != 2 || !s.Ready() {
		t.Fatal("unexpected allocation result")
	}
	if _, err := first.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatal("denied candidate retained TCP listener")
	}
}

func TestEphemeralSetupDeniedCompanionPortExhaustionClosesAll(t *testing.T) {
	s, _ := New("127.0.0.1")
	defer s.Close()
	var allocated []net.Listener
	listen := func(network, address string) (net.Listener, error) {
		l, err := net.Listen(network, address)
		if err == nil {
			allocated = append(allocated, l)
		}
		return l, err
	}
	calls := 0
	err := startTestDNSWith(s, listen, func(string, string) (net.PacketConn, error) {
		calls++
		return nil, &net.OpError{Op: "listen", Net: "udp", Err: syscall.Errno(10013)}
	})
	if err == nil || calls != 8 || s.Ready() {
		t.Fatal("fixture denial was hidden or unbounded", calls, err)
	}
	for _, l := range allocated {
		if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
			t.Fatal("exhausted fixture retained TCP listener")
		}
	}
}

func TestEphemeralSetupDenialClassificationIsNarrow(t *testing.T) {
	for _, err := range []error{
		syscall.Errno(10013),
		&net.OpError{Op: "dial", Net: "udp", Err: syscall.Errno(10013)},
		&net.OpError{Op: "listen", Net: "tcp", Err: syscall.Errno(10013)},
		&net.OpError{Op: "listen", Net: "udp", Err: syscall.EACCES},
	} {
		if testUDPCompanionPortDenied(err) {
			t.Fatal("unrelated error classified as denied companion port", err)
		}
	}
	s, _ := New("127.0.0.1")
	defer s.Close()
	calls := 0
	denied := &net.OpError{Op: "listen", Net: "tcp", Err: syscall.Errno(10013)}
	err := startTestDNSWith(s, func(string, string) (net.Listener, error) { calls++; return nil, denied }, net.ListenPacket)
	if !errors.Is(err, denied) || calls != 1 || s.Ready() {
		t.Fatal("TCP denial retried or hidden")
	}
}
