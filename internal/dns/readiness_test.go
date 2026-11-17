package dns

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"sync"
	"testing"
	"time"
)

func TestStartWaitsForBothDNSListeners(t *testing.T) {
	s, _ := New("127.0.0.1")
	s.Update(testMap())
	if s.Ready() {
		t.Fatal("ready before start")
	}
	if err := s.startAt("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Ready() {
		t.Fatal("Start returned before ready")
	}
	for _, transport := range []string{"udp", "tcp"} {
		q := new(dns.Msg)
		q.SetQuestion("server.home.mnet.", dns.TypeA)
		r, _, err := (&dns.Client{Net: transport, Timeout: time.Second}).Exchange(q, s.listener.Addr().String())
		if err != nil || r == nil || len(r.Answer) != 1 {
			t.Fatalf("%s: %+v / %v", transport, r, err)
		}
	}
	if err := s.startAt("127.0.0.1:0"); err == nil {
		t.Fatal("double start accepted")
	}
}
func TestTCPBindFailureClosesPartialUDPListener(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	s, _ := New("127.0.0.1")
	if err := s.startAt(occupied.Addr().String()); err == nil {
		t.Fatal("occupied TCP listener accepted")
	}
	if s.Ready() {
		t.Fatal("partial listener reports ready")
	}
	packet, err := net.ListenPacket("udp", occupied.Addr().String())
	if err != nil {
		t.Fatalf("partial UDP leaked: %v", err)
	}
	packet.Close()
	s.Close()
}
func TestUDPBindFailureNeverStartsTCP(t *testing.T) {
	occupied, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	s, _ := New("127.0.0.1")
	if err := s.startAt(occupied.LocalAddr().String()); err == nil {
		t.Fatal("occupied UDP listener accepted")
	}
	if s.Ready() {
		t.Fatal("failed start ready")
	}
	listener, err := net.Listen("tcp", occupied.LocalAddr().String())
	if err != nil {
		t.Fatalf("TCP leaked: %v", err)
	}
	listener.Close()
	s.Close()
}
func TestClosePreventsStartAndReleasesBothSockets(t *testing.T) {
	s, _ := New("127.0.0.1")
	s.Close()
	if err := s.startAt("127.0.0.1:0"); err == nil {
		t.Fatal("closed server revived")
	}
	s, _ = New("127.0.0.1")
	if err := s.startAt("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	addr := s.listener.Addr().String()
	s.Close()
	s.Close()
	if s.Ready() {
		t.Fatal("ready after close")
	}
	packet, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer packet.Close()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
}
func TestTransportFailureClearsReadyAndStopsOtherTransport(t *testing.T) {
	s, _ := New("127.0.0.1")
	if err := s.startAt("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	addr := s.listener.Addr().String()
	s.listener.Close()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("DNS did not stop after TCP failed")
	}
	if s.Ready() {
		t.Fatal("ready after transport loss")
	}
	packet, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("other transport leaked: %v", err)
	}
	packet.Close()
}
func TestConcurrentDNSStartAndCloseCannotLeakOrRevive(t *testing.T) {
	for i := 0; i < 20; i++ {
		s, _ := New("127.0.0.1")
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.startAt("127.0.0.1:0") }()
		go func() { defer wg.Done(); s.Close() }()
		wg.Wait()
		s.Close()
		if s.Ready() {
			t.Fatal(fmt.Sprint("ready after concurrent close ", i))
		}
	}
}

func TestKnownIPv4NameHasNoAAAAInsteadOfNXDOMAIN(t *testing.T) {
	s, _ := New("127.0.0.1")
	s.Update(testMap())
	if err := s.startAt("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, transport := range []string{"udp", "tcp"} {
		for _, kind := range []uint16{dns.TypeAAAA, dns.TypeTXT, dns.TypeA} {
			q := new(dns.Msg)
			q.SetQuestion("server.home.mnet.", kind)
			r, _, err := (&dns.Client{Net: transport, Timeout: time.Second}).Exchange(q, s.listener.Addr().String())
			if err != nil || r == nil || r.Rcode != dns.RcodeSuccess {
				t.Fatalf("known name denied for type%d: %v / %v", kind, r, err)
			}
			want := 0
			if kind == dns.TypeA {
				want = 1
			}
			if len(r.Answer) != want {
				t.Fatalf("type%d answer count%d", kind, len(r.Answer))
			}
		}
	}
}
