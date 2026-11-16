package mobile

import (
	"fintex.au/mnet/internal/session"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type viewerChecks struct{ closed atomic.Int32 }

func (*viewerChecks) OnFrame([]byte) {}
func (*viewerChecks) OnChat(string)  {}
func (v *viewerChecks) OnClosed()    { v.closed.Add(1) }

func TestSessionViewerReplyTimeoutClosesTransport(t *testing.T) {
	n := &Node{}
	a, b := net.Pipe()
	defer b.Close()
	// Consume hello but never send a reply. Raw drain avoids a protocol reply.
	go func() { buf := make([]byte, 4096); b.Read(buf) }()
	begin := time.Now()
	if v, err := n.connectSession(a, "viewer", &viewerChecks{}, 30*time.Millisecond); err == nil {
		v.Close()
		t.Fatal("silent host accepted")
	}
	if time.Since(begin) > time.Second {
		t.Fatal("reply not bounded")
	}
	b.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := b.Read(make([]byte, 1)); err == nil {
		t.Fatal("failed viewer transport leaked")
	}
	sessMu.Lock()
	count := len(n.viewers)
	sessMu.Unlock()
	if count != 0 {
		t.Fatal("failed viewer ownership leaked")
	}
}
func TestSessionViewerCloseCancelsPending(t *testing.T) {
	n, err := NewNode(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, b := net.Pipe()
	defer b.Close()
	entered := make(chan struct{})
	go func() { buf := make([]byte, 4096); b.Read(buf); close(entered) }()
	done := make(chan error, 1)
	go func() { _, err := n.connectSession(a, "viewer", &viewerChecks{}, time.Minute); done <- err }()
	<-entered
	n.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("pending closed viewer accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel reply")
	}
	c, d := net.Pipe()
	defer d.Close()
	if v, err := n.connectSession(c, "viewer", &viewerChecks{}, time.Minute); err == nil {
		v.Close()
		t.Fatal("disposed node viewed")
	}
}
func TestSessionViewerActiveDeadlineClearedAndOwnerStop(t *testing.T) {
	n := &Node{}
	a, b := net.Pipe()
	cb := &viewerChecks{}
	ln := &viewerOnce{conn: b}
	hostResult := make(chan *session.Mux, 1)
	go func() {
		m, _ := session.Listen(ln, "host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		hostResult <- m
	}()
	v, err := n.connectSession(a, "viewer", cb, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	host := <-hostResult
	if host == nil {
		t.Fatal("host handshake failed")
	}
	defer host.Close()
	time.Sleep(70 * time.Millisecond)
	select {
	case <-v.mux.Closed():
		t.Fatal("reply deadline leaked into active viewer")
	default:
	}
	other := &Node{}
	other.stopViewers(false)
	select {
	case <-v.mux.Closed():
		t.Fatal("unrelated node stopped viewer")
	default:
	}
	n.stopViewers(false)
	select {
	case <-v.mux.Closed():
	case <-time.After(time.Second):
		t.Fatal("owner stop leaked viewer")
	}
	deadline := time.Now().Add(time.Second)
	for cb.closed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cb.closed.Load() != 1 {
		t.Fatal("close callback count", cb.closed.Load())
	}
}

type viewerOnce struct{ conn net.Conn }

func (l *viewerOnce) Accept() (net.Conn, error) { return l.conn, nil }
func (l *viewerOnce) Close() error              { return nil }
func (l *viewerOnce) Addr() net.Addr            { return l.conn.LocalAddr() }
