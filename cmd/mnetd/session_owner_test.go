package main

import (
	"net"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

func awaitOwnerResult(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case err := <-ch:
		if err == nil {
			t.Fatal("closed owner operation succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("owner left blocked transport")
	}
}
func TestSessionHostOwnerClosesAcceptAndRejectsLateSocket(t *testing.T) {
	owner := newSessionHostOwner()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if !owner.attachListener(ln) {
		t.Fatal("listener refused")
	}
	done := make(chan error, 1)
	go func() { _, err := (ownedSessionListener{ln, owner}).Accept(); done <- err }()
	owner.Close()
	owner.Close()
	awaitOwnerResult(t, done)
	a, b := net.Pipe()
	defer b.Close()
	if _, err := owner.adopt(a); err == nil {
		t.Fatal("late accepted socket adopted")
	}
	b.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := b.Read(make([]byte, 1)); err == nil {
		t.Fatal("late socket leaked")
	}
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if owner.attachListener(other) {
		t.Fatal("late listener adopted")
	}
	if _, err := other.Accept(); err == nil {
		t.Fatal("late listener left open")
	}
}
func TestSessionHostOwnerCancelsPendingHelloBeforeDeadline(t *testing.T) {
	owner := newSessionHostOwner()
	defer owner.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	owner.attachListener(ln)
	accepted := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := listenHeadlessSession(ownedSessionListener{ln, owner}, "host", time.Minute, func(session.Hello, net.Addr) (bool, string) { return true, "" })
		done <- err
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Poll only the local test ownership boundary, never rely on sleep to guess
	// that Accept has finished. No protocol bytes are sent by the silent peer.
	go func() {
		for {
			owner.mu.Lock()
			count := len(owner.connections)
			owner.mu.Unlock()
			if count == 1 {
				close(accepted)
				return
			}
			select {
			case <-owner.ctx.Done():
				return
			default:
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("socket not adopted")
	}
	owner.Close()
	awaitOwnerResult(t, done)
}
func TestSessionHostOwnerClosesActiveMuxButNotSiblingOwner(t *testing.T) {
	owner := newSessionHostOwner()
	other := newSessionHostOwner()
	defer owner.Close()
	defer other.Close()
	a, b := net.Pipe()
	owned, err := owner.adopt(a)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan *session.Mux, 1)
	go func() {
		m, _ := session.Listen(&ownerOnceListener{conn: owned}, "host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		result <- m
	}()
	viewer, err := session.Dial(b, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	host := <-result
	if host == nil {
		t.Fatal("handshake failed")
	}
	defer host.Close()
	x, y := net.Pipe()
	defer y.Close()
	sibling, _ := other.adopt(x)
	defer sibling.Close()
	owner.Close()
	select {
	case <-viewer.Closed():
	case <-time.After(time.Second):
		t.Fatal("active viewer escaped owner")
	}
	sent := make(chan error, 1)
	go func() { _, err := sibling.Write([]byte{1}); sent <- err }()
	y.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := y.Read(make([]byte, 1)); err != nil {
		t.Fatal("other owner closed", err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	owner.mu.Lock()
	count := len(owner.connections)
	owner.mu.Unlock()
	if count != 0 {
		t.Fatal("closed connection ownership leaked")
	}
}

type ownerOnceListener struct {
	net.Listener
	conn net.Conn
}

func (l *ownerOnceListener) Accept() (net.Conn, error) { return l.conn, nil }
func TestSessionHostOwnerStopsBeforeMeshReady(t *testing.T) {
	d, err := daemon.New(daemon.Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	owner := newSessionHostOwner()
	done := make(chan struct{})
	go func() { runSessionHost(owner, d, t.TempDir(), false, nil, ""); close(done) }()
	owner.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("mesh readiness wait escaped shutdown")
	}
}
func TestSessionHostOwnerCloseRacesSocketAdoption(t *testing.T) {
	for i := 0; i < 50; i++ {
		owner := newSessionHostOwner()
		a, b := net.Pipe()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); owner.adopt(a) }()
		go func() { defer wg.Done(); owner.Close() }()
		wg.Wait()
		b.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := b.Read(make([]byte, 1)); err == nil {
			t.Fatal("raced socket remained open")
		}
		b.Close()
	}
}
