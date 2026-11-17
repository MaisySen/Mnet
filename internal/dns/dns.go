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
	// DNS names are case-insensitive. Accumulate every alias before choosing
	// records so a later peer cannot restore or override an ambiguous name.
	candidates := map[string]map[string]struct{}{}
	add := func(name, ip string) {
		name = strings.ToLower(dns.Fqdn(name))
		if candidates[name] == nil {
			candidates[name] = map[string]struct{}{}
		}
		candidates[name][ip] = struct{}{}
	}
	add(m.Network.Name+s.suffix, m.Self.IPv4)
	add(m.Self.FQDN(m.Network.Name), m.Self.IPv4)
	add(m.Self.Name+s.suffix, m.Self.IPv4)
	for _, p := range m.Peers {
		add(p.FQDN(m.Network.Name), p.IPv4)
		add(p.Name+s.suffix, p.IPv4)
	}
	recs := map[string]string{}
	for name, owners := range candidates {
		if len(owners) == 1 {
			for ip := range owners {
				recs[name] = ip
			}
		}
	}
	s.mu.Lock()
	s.records = recs
	s.mu.Unlock()
}

// Serve runs UDP and TCP listeners until Close.
func (s *Server) Serve() error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)
	udp := &dns.Server{Addr: s.addr + ":53", Net: "udp", Handler: mux}
	tcp := &dns.Server{Addr: s.addr + ":53", Net: "tcp", Handler: mux}
	s.mu.Lock()
	s.udp = udp
	s.tcp = tcp
	s.mu.Unlock()
	errc := make(chan error, 2)
	go func() { errc <- udp.ListenAndServe() }()
	go func() { errc <- tcp.ListenAndServe() }()
	return <-errc
}

// Close stops both listeners.
func (s *Server) Close() {
	s.mu.Lock()
	udp, tcp := s.udp, s.tcp
	s.mu.Unlock()
	if udp != nil {
		udp.Shutdown()
	}
	if tcp != nil {
		tcp.Shutdown()
	}
}

func (s *Server) handle(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = false
	m.RecursionAvailable = true

	if len(r.Question) != 1 {
		m.SetRcode(r, dns.RcodeFormatError)
		w.WriteMsg(m)
		return
	}
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
