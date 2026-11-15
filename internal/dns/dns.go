// Package dns is Magic DNS for Mnet: a small resolver listening on the
// node's own mesh address that answers <node>.<network>.mnet from the
// network map and forwards everything else upstream.
package dns

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/miekg/dns"

	"fintex.au/mnet/internal/netmap"
)

// Server answers DNS on one mesh address.
type Server struct {
	addr string

	mu      sync.RWMutex
	records map[string]string // fqdn with trailing dot -> mesh IPv4
	suffix  string            // ".mnet"

	udp *dns.Server
	tcp *dns.Server
}

// New binds nothing yet; Serve starts listeners on addr:53.
func New(addr string) (*Server, error) {
	if net.ParseIP(addr) == nil {
		return nil, fmt.Errorf("bad listen address %q", addr)
	}
	return &Server{
		addr:    addr,
		records: map[string]string{},
		suffix:  "." + netmap.DNSSuffix,
	}, nil
}

// Update rebuilds the record table from a network map.
func (s *Server) Update(m *netmap.Map) {
	recs := map[string]string{}
	zone := "." + m.Network.Name + s.suffix + "."
	recs[dns.Fqdn(m.Network.Name+s.suffix)] = m.Self.IPv4
	recs[dns.Fqdn(m.Self.Name+zone[:len(zone)-1])] = m.Self.IPv4
	recs[dns.Fqdn(m.Self.FQDN(m.Network.Name))] = m.Self.IPv4
	for _, p := range m.Peers {
		recs[dns.Fqdn(p.FQDN(m.Network.Name))] = p.IPv4
		// Short form: <node>.mnet when unambiguous.
		short := dns.Fqdn(p.Name + s.suffix)
		if _, clash := recs[short]; !clash {
			recs[short] = p.IPv4
		} else {
			delete(recs, short)
		}
	}
	short := dns.Fqdn(m.Self.Name + s.suffix)
	if _, clash := recs[short]; !clash {
		recs[short] = m.Self.IPv4
	}
	s.mu.Lock()
	s.records = recs
	s.mu.Unlock()
}

// Serve runs UDP and TCP listeners until Close.
func (s *Server) Serve() error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)
	s.udp = &dns.Server{Addr: s.addr + ":53", Net: "udp", Handler: mux}
	s.tcp = &dns.Server{Addr: s.addr + ":53", Net: "tcp", Handler: mux}
	errc := make(chan error, 2)
	go func() { errc <- s.udp.ListenAndServe() }()
	go func() { errc <- s.tcp.ListenAndServe() }()
	return <-errc
}

// Close stops both listeners.
func (s *Server) Close() {
	if s.udp != nil {
		s.udp.Shutdown()
	}
	if s.tcp != nil {
		s.tcp.Shutdown()
	}
}

func (s *Server) handle(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = false
	m.RecursionAvailable = true

	q := r.Question[0]
	name := strings.ToLower(q.Name)

	s.mu.RLock()
	ip, mine := s.records[name]
	s.mu.RUnlock()

	if mine && q.Qtype == dns.TypeA {
		m.Answer = append(m.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   net.ParseIP(ip).To4(),
		})
		w.WriteMsg(m)
		return
	}
	if strings.HasSuffix(name, s.suffix+".") {
		// Our zone but no such node: say so plainly.
		m.SetRcode(r, dns.RcodeNameError)
		w.WriteMsg(m)
		return
	}

	// Everything else goes upstream.
	resp, err := dns.Exchange(r, upstreamServer())
	if err != nil {
		m.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(m)
		return
	}
	w.WriteMsg(resp)
}
