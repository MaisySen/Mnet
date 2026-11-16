package mobile

import (
	"fintex.au/mnet/internal/session"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionStopDoesNotCloseAnotherNodeHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	owner := &Node{}
	sessMu.Lock()
	hosting = &sessionHostRun{owner: owner, listener: ln}
	sessMu.Unlock()
	defer owner.StopSessionHost()
	other := &Node{}
	other.StopSessionHost()
	c, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatal("unrelated node stopped another host:", err)
	}
	c.Close()
}

type ownerCallbacks struct {
	entered  chan struct{}
	release  chan struct{}
	captures atomic.Int32
	ends     atomic.Int32
}

func (c *ownerCallbacks) Capture() ([]byte, error)  { c.captures.Add(1); return nil, nil }
func (c *ownerCallbacks) ScreenWidth() int          { return 16 }
func (c *ownerCallbacks) ScreenHeight() int         { return 16 }
func (c *ownerCallbacks) Inject(string)             {}
func (c *ownerCallbacks) AcceptSession(string) bool { close(c.entered); <-c.release; return true }
func (c *ownerCallbacks) OnChat(string)             {}
func (c *ownerCallbacks) OnFile(string, int64)      {}
func (c *ownerCallbacks) OnSessionEnd()             { c.ends.Add(1) }

func TestSessionLateConsentCannotReviveStoppedHost(t *testing.T) {
	owner, other := &Node{}, &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cb := &ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}
	if err := owner.startSessionHost(ln, "fixture", cb); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	sessMu.Lock()
	old := hosting
	sessMu.Unlock()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	result := make(chan error, 1)
	go func() {
		m, e := session.Dial(conn, "viewer")
		if m != nil {
			m.Close()
		}
		result <- e
	}()
	select {
	case <-cb.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("consent not entered")
	}
	owner.StopSessionHost()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stopped pending consent accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("accepted connection leaked during consent")
	}
	replacement, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.startSessionHost(replacement, "new", cb); err != nil {
		t.Fatal(err)
	}
	defer other.StopSessionHost()
	close(cb.release)
	select {
	case <-old.done:
	case <-time.After(time.Second):
		t.Fatal("old consent worker did not exit")
	}
	// A stale run cannot clear the replacement, including explicit old cleanup.
	owner.stopSessionRun(old)
	sessMu.Lock()
	current := hosting
	sessMu.Unlock()
	if current == nil || current.owner != other {
		t.Fatal("old run removed replacement")
	}
	c, err := net.DialTimeout("tcp", replacement.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if cb.captures.Load() != 0 || cb.ends.Load() != 0 {
		t.Fatal("late consent started capture")
	}
}

func TestSessionCloseRefusesNewListener(t *testing.T) {
	n, err := NewNode(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := n.startSessionHost(ln, "closed", &ownerCallbacks{}); err == nil {
		t.Fatal("closed node hosted")
	}
	if c, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		c.Close()
		t.Fatal("rejected listener leaked")
	}
}

func TestSessionStopActiveHostClosesMux(t *testing.T) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cb := &ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}
	close(cb.release)
	if err := owner.startSessionHost(ln, "fixture", cb); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	sessMu.Lock()
	run := hosting
	sessMu.Unlock()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	m, err := session.Dial(conn, "viewer")
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	defer m.Close()
	deadline := time.Now().Add(time.Second)
	for cb.captures.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cb.captures.Load() == 0 {
		t.Fatal("actual session host never captured")
	}
	stopped := make(chan struct{})
	go func() { owner.StopSessionHost(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("active stop deadlock")
	}
	select {
	case <-m.Closed():
	case <-time.After(time.Second):
		t.Fatal("active mux leaked")
	}
	select {
	case <-run.done:
	case <-time.After(time.Second):
		t.Fatal("active worker leaked")
	}
	if cb.ends.Load() != 1 {
		t.Fatal("session end callback count", cb.ends.Load())
	}
}

type retryCallbacks struct {
	ownerCallbacks
	decisions atomic.Int32
}

func (c *retryCallbacks) AcceptSession(string) bool { return c.decisions.Add(1) > 1 }
func TestSessionHostSurvivesRefusedAndMalformedPeer(t *testing.T) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cb := &retryCallbacks{}
	if err := owner.startSessionHost(ln, "fixture", cb); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	sessMu.Lock()
	run := hosting
	sessMu.Unlock()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(time.Second))
	if m, err := session.Dial(c, "refused"); err == nil {
		m.Close()
		t.Fatal("refusal accepted")
	}
	c.Close()
	malformed, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatal("refusal killed hosting:", err)
	}
	malformed.Write([]byte("invalid peer"))
	malformed.Close()
	c, err = net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatal("malformed peer killed hosting:", err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	m, err := session.Dial(c, "approved")
	if err != nil {
		t.Fatal("later approved peer failed:", err)
	}
	defer m.Close()
	owner.StopSessionHost()
	select {
	case <-run.done:
	case <-time.After(time.Second):
		t.Fatal("worker not stopped")
	}
	if cb.decisions.Load() != 2 {
		t.Fatal("unexpected consent count", cb.decisions.Load())
	}
}
