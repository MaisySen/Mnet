package dns

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"strconv"
	"syscall"
	"testing"
)

const testDNSPortAttempts = 32

// Test-only dual-protocol allocation. TCP and UDP have independent port pools.
// Sample across the dynamic range because consecutive OS-assigned TCP ports can
// all fall inside the same Windows UDP exclusion. Own both sockets before use.
func reserveTestDNSPair(t *testing.T) (net.Listener, net.PacketConn) {
	t.Helper()
	tcp, udp, err := reserveTestDNSPairWith(net.Listen, net.ListenPacket)
	if err != nil {
		t.Fatal(err)
	}
	return tcp, udp
}

func reserveTestDNSPairWith(listen func(string, string) (net.Listener, error), packet func(string, string) (net.PacketConn, error)) (net.Listener, net.PacketConn, error) {
	var last error
	for attempt := 0; attempt < testDNSPortAttempts; attempt++ {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(49152+rand.IntN(16384)))
		tcp, err := listen("tcp", address)
		if err != nil {
			if !testPairPortUnavailable(err, "tcp") {
				return nil, nil, err
			}
			last = err
			continue
		}
		udp, err := packet("udp", tcp.Addr().String())
		if err == nil {
			return tcp, udp, nil
		}
		tcp.Close()
		if !testPairPortUnavailable(err, "udp") {
			return nil, nil, err
		}
		last = err
	}
	return nil, nil, fmt.Errorf("test DNS could not reserve both transports: %w", last)
}

// Retry only loopback fixture bind conflicts or Windows port exclusions.
func testPairPortUnavailable(err error, transport string) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "listen" && op.Net == transport &&
		(errors.Is(op.Err, syscall.Errno(10013)) || errors.Is(op.Err, syscall.Errno(10048)))
}

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
	tcp, udp, err := reserveTestDNSPairWith(listen, packet)
	if err != nil {
		return err
	}
	return s.startBoundLocked(udp, tcp)
}

func TestEphemeralSetupRetriesOnlyUnusableCandidates(t *testing.T) {
	for _, udpError := range []error{syscall.EADDRINUSE, &net.OpError{Op: "listen", Net: "udp", Err: syscall.Errno(10013)}} {
		t.Run(udpError.Error(), func(t *testing.T) {
			s, _ := New("127.0.0.1")
			defer s.Close()
			first, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			goodTCP, goodUDP := reserveTestDNSPair(t)
			defer goodTCP.Close()
			defer goodUDP.Close()

			var tcpCalls, udpCalls int
			listen := func(_, _ string) (net.Listener, error) {
				tcpCalls++
				switch tcpCalls {
				case 1:
					return nil, &net.OpError{Op: "listen", Net: "tcp", Err: syscall.Errno(10013)}
				case 2:
					return first, nil
				default:
					return goodTCP, nil
				}
			}
			packet := func(_, _ string) (net.PacketConn, error) {
				udpCalls++
				if udpCalls == 1 {
					return nil, udpError
				}
				return goodUDP, nil
			}
			if err := startTestDNSWith(s, listen, packet); err != nil {
				t.Fatal(err)
			}
			if tcpCalls != 3 || udpCalls != 2 || !s.Ready() {
				t.Fatal("setup did not recover from unusable fixture ports")
			}
			if _, err := first.Accept(); !errors.Is(err, net.ErrClosed) {
				t.Fatal("failed UDP candidate retained its TCP listener")
			}
		})
	}
}

func TestEphemeralSetupDoesNotRetryOtherErrors(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, _ := New("127.0.0.1")
			defer s.Close()
			denied := &net.OpError{Op: "listen", Net: transport, Err: syscall.EACCES}
			var tcpCalls, udpCalls int
			err := startTestDNSWith(s, func(network, _ string) (net.Listener, error) {
				tcpCalls++
				if transport == "tcp" {
					return nil, denied
				}
				return net.Listen(network, "127.0.0.1:0")
			}, func(_, _ string) (net.PacketConn, error) {
				udpCalls++
				return nil, denied
			})
			wantUDP := 0
			if transport == "udp" {
				wantUDP = 1
			}
			if !errors.Is(err, denied) || tcpCalls != 1 || udpCalls != wantUDP || s.Ready() {
				t.Fatal("unrelated bind failure was retried or hidden")
			}
		})
	}
}

func TestEphemeralSetupDeniedCompanionPortExhaustionClosesAll(t *testing.T) {
	s, _ := New("127.0.0.1")
	defer s.Close()
	var allocated []net.Listener
	listen := func(network, _ string) (net.Listener, error) {
		l, err := net.Listen(network, "127.0.0.1:0")
		if err == nil {
			allocated = append(allocated, l)
		}
		return l, err
	}
	denied := &net.OpError{Op: "listen", Net: "udp", Err: syscall.Errno(10013)}
	err := startTestDNSWith(s, listen, func(_, _ string) (net.PacketConn, error) {
		return nil, denied
	})
	if !errors.Is(err, denied) || len(allocated) != testDNSPortAttempts || s.Ready() {
		t.Fatal("fixture exhaustion was hidden or unbounded", len(allocated), err)
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
		if testPairPortUnavailable(err, "udp") {
			t.Fatal("unrelated error classified as unusable UDP port", err)
		}
	}
}
