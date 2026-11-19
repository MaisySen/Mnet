package dns

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func useTestUpstream(t *testing.T, handler dns.Handler) {
	t.Helper()
	var listener net.Listener
	var packet net.PacketConn
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		packet, err = net.ListenPacket("udp", listener.Addr().String())
		if err == nil {
			break
		}
		listener.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, 2)
	udpServer := &dns.Server{PacketConn: packet, Handler: handler, NotifyStartedFunc: func() { ready <- struct{}{} }}
	tcpServer := &dns.Server{Listener: listener, Handler: handler, NotifyStartedFunc: func() { ready <- struct{}{} }}
	t.Cleanup(func() { udpServer.Shutdown(); tcpServer.Shutdown() })
	go udpServer.ActivateAndServe()
	go tcpServer.ActivateAndServe()
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("fixture upstream not ready")
		}
	}
	// Package tests are sequential. Each resolver is closed before this cleanup;
	// the production upstream choice is never changed by application requests.
	previous := upstreamServer()
	upstream = listener.Addr().String()
	t.Cleanup(func() { upstream = previous })
}

func TestForwardedTruncatedUDPAnswerRetriesTCPForBothLocalTransports(t *testing.T) {
	var udpCalls, tcpCalls atomic.Int32
	value := strings.Repeat("large-answer-", 120)
	useTestUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(r)
		reply.RecursionAvailable = true
		if w.LocalAddr().Network() == "udp" {
			udpCalls.Add(1)
			reply.Truncated = true
		} else {
			tcpCalls.Add(1)
			var chunks []string
			for remaining := value; len(remaining) > 0; {
				n := len(remaining)
				if n > 200 {
					n = 200
				}
				chunks = append(chunks, remaining[:n])
				remaining = remaining[n:]
			}
			reply.Answer = []dns.RR{&dns.TXT{Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 30}, Txt: chunks}}
		}
		_ = w.WriteMsg(reply)
	}))
	resolver, _ := New("127.0.0.1")
	startTestDNS(t, resolver)
	defer resolver.Close()
	for _, transport := range []string{"udp", "tcp", "udp-small"} {
		t.Run(transport, func(t *testing.T) {
			query := new(dns.Msg)
			query.SetQuestion("large.external.test.", dns.TypeTXT)
			network := transport
			if transport == "udp" {
				query.SetEdns0(4096, false)
			}
			if transport == "udp-small" {
				network = "udp"
			}
			answer, _, err := (&dns.Client{Net: network, Timeout: 3 * time.Second}).Exchange(query, resolver.listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			if answer.Id != query.Id || len(answer.Question) != 1 || answer.Question[0] != query.Question[0] {
				t.Fatal("fallback changed the original question or transaction")
			}
			if transport == "udp-small" {
				packed, err := answer.Pack()
				if err != nil {
					t.Fatal(err)
				}
				if !answer.Truncated || len(packed) > dns.MinMsgSize {
					t.Fatal("UDP caller without EDNS received oversized or falsely complete answer")
				}
				return
			}
			if answer.Truncated || len(answer.Answer) != 1 {
				t.Fatalf("forwarded answer stayed truncated or empty: tc=%t records=%d", answer.Truncated, len(answer.Answer))
			}
			text, ok := answer.Answer[0].(*dns.TXT)
			if !ok || strings.Join(text.Txt, "") != value {
				t.Fatal("complete upstream TXT answer lost")
			}
		})
	}
	if udpCalls.Load() != 3 || tcpCalls.Load() != 3 {
		t.Fatal("upstream fallback count wrong", udpCalls.Load(), tcpCalls.Load())
	}
}

func TestForwardedCompleteUDPAnswerDoesNotOpenTCP(t *testing.T) {
	var tcpCalls atomic.Int32
	useTestUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		if w.LocalAddr().Network() == "tcp" {
			tcpCalls.Add(1)
		}
		reply := new(dns.Msg)
		reply.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(reply)
	}))
	resolver, _ := New("127.0.0.1")
	startTestDNS(t, resolver)
	defer resolver.Close()
	for _, transport := range []string{"udp", "tcp"} {
		query := new(dns.Msg)
		query.SetQuestion("missing.external.test.", dns.TypeA)
		answer, _, err := (&dns.Client{Net: transport, Timeout: time.Second}).Exchange(query, resolver.listener.Addr().String())
		if err != nil || answer == nil || answer.Rcode != dns.RcodeNameError || answer.Truncated {
			t.Fatal("complete upstream denial was changed", err)
		}
	}
	if tcpCalls.Load() != 0 {
		t.Fatal("untruncated answer opened upstream TCP")
	}
}

func TestForwardedTCPFailureDoesNotPublishTruncatedUpstreamAnswer(t *testing.T) {
	for _, mode := range []string{"closed", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			useTestUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
				if w.LocalAddr().Network() == "tcp" && mode == "closed" {
					_ = w.Close()
					return
				}
				reply := new(dns.Msg)
				reply.SetReply(r)
				reply.Truncated = true
				_ = w.WriteMsg(reply)
			}))
			resolver, _ := New("127.0.0.1")
			startTestDNS(t, resolver)
			defer resolver.Close()
			for _, transport := range []string{"udp", "tcp"} {
				query := new(dns.Msg)
				query.SetQuestion("incomplete.external.test.", dns.TypeTXT)
				answer, _, err := (&dns.Client{Net: transport, Timeout: time.Second}).Exchange(query, resolver.listener.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				if answer.Rcode != dns.RcodeServerFailure || answer.Truncated || len(answer.Answer) != 0 {
					t.Fatal("failed fallback published incomplete data")
				}
			}
		})
	}
}

func TestForwardedStalledTCPFallbackHasOneBoundedBudget(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	var tcpCalls atomic.Int32
	useTestUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		if w.LocalAddr().Network() == "tcp" {
			tcpCalls.Add(1)
			<-stop
			return
		}
		// Consume part of the same overall budget before requesting TCP.
		time.Sleep(1250 * time.Millisecond)
		reply := new(dns.Msg)
		reply.SetReply(r)
		reply.Truncated = true
		_ = w.WriteMsg(reply)
	}))
	resolver, _ := New("127.0.0.1")
	startTestDNS(t, resolver)
	defer resolver.Close()
	query := new(dns.Msg)
	query.SetQuestion("stalled.external.test.", dns.TypeTXT)
	started := time.Now()
	answer, _, err := (&dns.Client{Net: "tcp", Timeout: 4 * time.Second}).Exchange(query, resolver.listener.Addr().String())
	if err != nil || answer == nil || answer.Rcode != dns.RcodeServerFailure {
		t.Fatal("stalled fallback did not return bounded SERVFAIL", err)
	}
	if time.Since(started) > 3*time.Second || tcpCalls.Load() != 1 {
		t.Fatal("fallback budget or retry count wrong")
	}
}
