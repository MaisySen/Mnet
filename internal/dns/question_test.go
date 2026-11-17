package dns

import (
	"github.com/miekg/dns"
	"net"
	"testing"
)

type responseCapture struct{ reply *dns.Msg }

func (w *responseCapture) LocalAddr() net.Addr         { return &net.UDPAddr{} }
func (w *responseCapture) RemoteAddr() net.Addr        { return &net.UDPAddr{} }
func (w *responseCapture) WriteMsg(m *dns.Msg) error   { w.reply = m.Copy(); return nil }
func (w *responseCapture) Write(b []byte) (int, error) { return len(b), nil }
func (w *responseCapture) Close() error                { return nil }
func (w *responseCapture) TsigStatus() error           { return nil }
func (w *responseCapture) TsigTimersOnly(bool)         {}
func (w *responseCapture) Hijack()                     {}

func TestMalformedQuestionCountIsRefused(t *testing.T) {
	s, _ := New("127.0.0.1")
	for _, n := range []int{0, 2, 8} {
		r := new(dns.Msg)
		for i := 0; i < n; i++ {
			r.Question = append(r.Question, dns.Question{Name: "server.home.mnet.", Qtype: dns.TypeA, Qclass: dns.ClassINET})
		}
		w := new(responseCapture)
		s.handle(w, r)
		if w.reply == nil || w.reply.Rcode != dns.RcodeFormatError || len(w.reply.Answer) != 0 {
			t.Fatalf("%d questions: %+v", n, w.reply)
		}
	}
}

func TestAmbiguousDNSLookupRefusesInsteadOfSelectingAnOwner(t *testing.T) {
	s, _ := New("127.0.0.1")
	m := testMap()
	m.Peers = append(m.Peers, m.Peers[0])
	m.Peers[2].IPv4 = "100.64.0.8"
	s.Update(m)
	for _, name := range []string{"server.mnet.", "SERVER.HOME.MNET."} {
		r := new(dns.Msg)
		r.SetQuestion(name, dns.TypeA)
		w := new(responseCapture)
		s.handle(w, r)
		if w.reply == nil || w.reply.Rcode != dns.RcodeNameError || len(w.reply.Answer) != 0 {
			t.Fatalf("%s: %+v", name, w.reply)
		}
	}
}
