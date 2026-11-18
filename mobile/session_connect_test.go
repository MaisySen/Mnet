package mobile

import (
	"context"
	"net"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
)

func TestSessionConnectCancelBeforeStartAndOwnerStop(t *testing.T) {
	n := &Node{}
	a, err := n.NewSessionConnect()
	if err != nil {
		t.Fatal(err)
	}
	a.Cancel()
	a.Cancel()
	if err := a.begin(); err == nil {
		t.Fatal("canceled handle reused")
	}
	a, err = n.NewSessionConnect()
	if err != nil {
		t.Fatal(err)
	}
	n.stopViewers(false)
	select {
	case <-a.ctx.Done():
	default:
		t.Fatal("node stop left dial alive")
	}
	if err := a.begin(); err == nil {
		t.Fatal("node-stopped attempt started")
	}
	b, err := n.NewSessionConnect()
	if err != nil {
		t.Fatal("temporary stop closed node permanently")
	}
	defer b.Cancel()
	n.stopViewers(true)
	if _, err := n.NewSessionConnect(); err == nil {
		t.Fatal("closed node admitted attempt")
	}
}
func TestSessionConnectCancelInterruptsPendingDialContext(t *testing.T) {
	n := &Node{}
	a, _ := n.NewSessionConnect()
	if err := a.begin(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	done := make(chan error, 1)
	// Controlled dial dependency models an in-flight syscall waiting on the same
	// context given to the actual net.Dialer. No unroutable-address timing guess.
	go func() { close(entered); <-a.ctx.Done(); _, err := a.complete(nil, a.ctx.Err()); done <- err }()
	<-entered
	a.Cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled dial succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("dial context not canceled")
	}
	sessMu.Lock()
	count := len(n.sessionConnects)
	sessMu.Unlock()
	if count != 0 {
		t.Fatal("canceled attempt leaked owner entry")
	}
	// The actual source-bound dial must honor an already-canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if c, err := dialMeshTCPContext(ctx, "127.0.0.1", "127.0.0.1:1", 10*time.Second); err == nil {
		c.Close()
		t.Fatal("dial ignored cancellation")
	}
}
func TestSessionConnectCancelClosesOnlyOwnPendingHello(t *testing.T) {
	n := &Node{}
	a, _ := n.NewSessionConnect()
	other, _ := n.NewSessionConnect()
	defer other.Cancel()
	if err := a.begin(); err != nil {
		t.Fatal(err)
	}
	if err := other.begin(); err != nil {
		t.Fatal(err)
	}
	x, y := net.Pipe()
	defer y.Close()
	unrelated, remote := net.Pipe()
	defer remote.Close()
	if err := a.attach(x); err != nil {
		t.Fatal(err)
	}
	if err := other.attach(unrelated); err != nil {
		t.Fatal(err)
	}
	helloRead := make(chan struct{})
	go func() { b := make([]byte, 4096); y.Read(b); close(helloRead) }()
	done := make(chan error, 1)
	go func() {
		v, err := n.connectSessionWithControlVerifier(x, "viewer", &viewerChecks{}, time.Minute, func(session.HelloReply, net.Addr) error { return nil }, session.NewViewerControl(nil))
		_, err = a.complete(v, err)
		done <- err
	}()
	<-helloRead
	a.Cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("pending hello granted after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("pending hello did not close")
	}
	sent := make(chan error, 1)
	go func() { _, err := unrelated.Write([]byte{1}); sent <- err }()
	remote.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := remote.Read(make([]byte, 1)); err != nil {
		t.Fatal("unrelated attempt was closed", err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
}
func TestSessionConnectCancelRacesLateSocketAndSuccessfulReturn(t *testing.T) {
	for i := 0; i < 50; i++ {
		n := &Node{}
		a, _ := n.NewSessionConnect()
		a.begin()
		x, y := net.Pipe()
		attached := make(chan struct{})
		go func() { a.attach(x); close(attached) }()
		a.Cancel()
		<-attached
		y.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := y.Read(make([]byte, 1)); err == nil {
			t.Fatal("late socket remained open")
		}
		y.Close()
		if _, err := a.complete(nil, nil); err == nil {
			t.Fatal("canceled operation returned success")
		}
	}
}

// Both use real protocol hello/muxes over loopback pipe transports. This is
// ownership proof, not Android or mesh-routing acceptance.
func TestSessionConnectCancellationPreservesActiveSiblingAndTransferredViewer(t *testing.T) {
	n := &Node{}
	active, err := n.NewSessionConnect()
	if err != nil {
		t.Fatal(err)
	}
	active.begin()
	x, y := net.Pipe()
	active.attach(x)
	hostResult := make(chan *session.Mux, 1)
	go func() {
		m, _ := session.Listen(&viewerOnce{conn: y}, "host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		hostResult <- m
	}()
	v, err := n.connectSession(x, "viewer", &viewerChecks{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	host := <-hostResult
	if host == nil {
		t.Fatal("host failed")
	}
	defer host.Close()
	defer v.Close()
	returned, err := active.complete(v, nil)
	if err != nil || returned != v {
		t.Fatal("ownership not transferred", err)
	}
	pending, _ := n.NewSessionConnect()
	pending.begin()
	a, b := net.Pipe()
	defer b.Close()
	pending.attach(a)
	pending.Cancel()
	active.Cancel()
	select {
	case <-v.mux.Closed():
		t.Fatal("cancel closed transferred/sibling viewer")
	default:
	}
	if err := v.viewer.SendChat("viewer", "still connected"); err != nil {
		t.Fatal("active sibling no longer usable", err)
	}
	if err := active.begin(); err == nil {
		t.Fatal("completed handle reused")
	}
	n.stopViewers(false)
	select {
	case <-v.mux.Closed():
	case <-time.After(time.Second):
		t.Fatal("node stop missed transferred viewer")
	}
}
func TestSessionConnectCancelBeforePublicationClosesReturnedViewer(t *testing.T) {
	n := &Node{}
	attempt, _ := n.NewSessionConnect()
	attempt.begin()
	a, b := net.Pipe()
	attempt.attach(a)
	hostResult := make(chan *session.Mux, 1)
	go func() {
		m, _ := session.Listen(&viewerOnce{conn: b}, "host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		hostResult <- m
	}()
	v, err := n.connectSession(a, "viewer", &viewerChecks{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	host := <-hostResult
	if host == nil {
		t.Fatal("host failed")
	}
	defer host.Close()
	defer v.Close()
	attempt.Cancel()
	if published, err := attempt.complete(v, nil); err == nil || published != nil {
		t.Fatal("late viewer published")
	}
	select {
	case <-v.mux.Closed():
	case <-time.After(time.Second):
		t.Fatal("late viewer remained open")
	}
}
