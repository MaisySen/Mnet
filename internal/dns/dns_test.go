package dns

import (
	"testing"
	"time"

	"github.com/miekg/dns"

	"fintex.au/mnet/internal/netmap"
)

func testMap() *netmap.Map {
	return &netmap.Map{
		Network: netmap.Network{ID: "net-1", Name: "home"},
		Self:    netmap.Node{ID: "node-1", Name: "laptop", IPv4: "100.64.0.1"},
		Peers: []netmap.Node{
			{ID: "node-2", Name: "server", IPv4: "100.64.0.2"},
			{ID: "node-3", Name: "pi", IPv4: "100.64.0.3"},
		},
	}
}

// TestRecords builds the table and queries it like a real resolver.
func TestRecords(t *testing.T) {
	s := &Server{records: map[string]string{}, suffix: ".mnet"}
	s.Update(testMap())

	want := map[string]string{
		"laptop.home.mnet.": "100.64.0.1",
		"server.home.mnet.": "100.64.0.2",
		"pi.home.mnet.":     "100.64.0.3",
		"server.mnet.":      "100.64.0.2", // short form
	}
	for name, ip := range want {
		if got := s.records[name]; got != ip {
			t.Errorf("%s: got %q want %q", name, got, ip)
		}
	}

	// Removal drops the record.
	m := testMap()
	m.Peers = m.Peers[1:]
	s.Update(m)
	if _, ok := s.records["server.home.mnet."]; ok {
		t.Error("removed node still resolves")
	}
}

// TestServe answers over UDP on a high port.
func TestServe(t *testing.T) {
	s, err := New("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	s.Update(testMap())
	// Bind a high port directly for the test.
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)
	srv := &dns.Server{Addr: "127.0.0.1:15353", Net: "udp", Handler: mux}
	go srv.ListenAndServe()
	defer srv.Shutdown()
	time.Sleep(200 * time.Millisecond)

	c := new(dns.Client)
	msg := new(dns.Msg)
	msg.SetQuestion("server.home.mnet.", dns.TypeA)
	r, _, err := c.Exchange(msg, "127.0.0.1:15353")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Answer) != 1 {
		t.Fatalf("answers: %d", len(r.Answer))
	}
	if a := r.Answer[0].(*dns.A); a.A.String() != "100.64.0.2" {
		t.Fatalf("A record %s", a.A)
	}

	// Unknown name inside our zone is NXDOMAIN.
	msg.SetQuestion("nope.home.mnet.", dns.TypeA)
	r, _, err = c.Exchange(msg, "127.0.0.1:15353")
	if err != nil {
		t.Fatal(err)
	}
	if r.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode %d", r.Rcode)
	}
}
