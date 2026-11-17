// Package dns is Magic DNS for Mnet: a small resolver listening on the
// node's own mesh address that answers <node>.<network>.mnet from the
// network map and forwards everything else upstream.
package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"

	"fintex.au/mnet/internal/netmap"
)

// Server answers DNS on one mesh address.
type Server struct {
	addr string

	mu           sync.RWMutex
	requestSlots chan struct{}     // optional bounded mobile resolver work
	records      map[string]string // fqdn with trailing dot -> mesh IPv4
	suffix       string            // ".mnet"

	udp      *dns.Server
	tcp      *dns.Server
	packet   net.PacketConn
	listener net.Listener
	serveErr error
	ready    bool
	closed   bool
	done     chan struct{}
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

// Start returns only after both UDP and TCP listeners have activated. A partial
// bind is cleaned up, and Close before Start permanently prevents revival.
func (s *Server) Start() error { return s.startAt(net.JoinHostPort(s.addr, "53")) }

func (s *Server) startAt(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("DNS server is closed")
	}
	if s.done != nil {
		return fmt.Errorf("DNS server already started")
	}
	packet, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("DNS UDP bind: %w", err)
	}
	// Port zero is used only by isolated tests; TCP must use the same chosen port.
	listener, err := net.Listen("tcp", packet.LocalAddr().String())
	if err != nil {
		packet.Close()
		return fmt.Errorf("DNS TCP bind: %w", err)
	}
	return s.startBoundLocked(packet, listener)
}

// Caller holds s.mu and owns both listeners. Supports native or in-tunnel sockets.
func (s *Server) startBoundLocked(packet net.PacketConn, listener net.Listener) error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)
	ready := make(chan struct{}, 2)
	errors := make(chan error, 2)
	notify := func() { ready <- struct{}{} }
	udp := &dns.Server{PacketConn: packet, Handler: mux, NotifyStartedFunc: notify}
	tcp := &dns.Server{Listener: listener, Handler: mux, NotifyStartedFunc: notify}
	s.packet, s.listener, s.udp, s.tcp = packet, listener, udp, tcp
	s.done = make(chan struct{})
	go func() { errors <- udp.ActivateAndServe() }()
	go func() { errors <- tcp.ActivateAndServe() }()
	for n := 0; n < 2; n++ {
		select {
		case <-ready:
		case err := <-errors:
			packet.Close()
			listener.Close()
			s.closed = true
			go func() { shutdownDNS(udp, tcp); close(s.done) }()
			return fmt.Errorf("DNS activation: %v", err)
		}
	}
	s.ready = true
	go func() {
		err := <-errors
		s.mu.Lock()
		if !s.closed {
			s.serveErr = err
			if err == nil {
				s.serveErr = fmt.Errorf("DNS listener stopped")
			}
		}
		s.ready = false
		s.mu.Unlock()
		// A single transport failing makes the whole DNS service unavailable.
		packet.Close()
		listener.Close()
		shutdownDNS(udp, tcp)
		<-errors
		close(s.done)
	}()
	return nil
}

func shutdownDNS(servers ...*dns.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, server := range servers {
		if server != nil {
			_ = server.ShutdownContext(ctx)
		}
	}
}

// Ready reports listener activation, not proof of end-to-end DNS/peer traffic.
func (s *Server) Ready() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.ready && !s.closed }

// Serve retains the blocking lifecycle API, with synchronous startup failure.
func (s *Server) Serve() error {
	if err := s.Start(); err != nil {
		return err
	}
	s.mu.RLock()
	done := s.done
	s.mu.RUnlock()
	<-done
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serveErr
}

// Close stops both transports, including prebound sockets not yet serving.
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	s.ready = false
	packet, listener, udp, tcp, done := s.packet, s.listener, s.udp, s.tcp, s.done
	s.mu.Unlock()
	if packet != nil {
		packet.Close()
	}
	if listener != nil {
		listener.Close()
	}
	shutdownDNS(udp, tcp)
	if done != nil {
		<-done
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
	if s.requestSlots != nil {
		select {
		case s.requestSlots <- struct{}{}:
			defer func() { <-s.requestSlots }()
		default:
			m.SetRcode(r, dns.RcodeRefused)
			w.WriteMsg(m)
			return
		}
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
