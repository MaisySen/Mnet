package mobile

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

type activeIdentityCallbacks struct {
	ownerCallbacks
	started     chan struct{}
	captureOnce sync.Once
	inputs      atomic.Int32
}

func (c *activeIdentityCallbacks) Capture() ([]byte, error) {
	c.captures.Add(1)
	c.captureOnce.Do(func() { close(c.started) })
	return nil, nil
}
func (c *activeIdentityCallbacks) Inject(string) { c.inputs.Add(1) }

func TestActiveMobileSessionClosesWhenKnownPeerRemoved(t *testing.T) {
	for _, change := range []string{"removed", "key-replaced", "renamed", "duplicate-source", "duplicate-name"} {
		t.Run(change, func(t *testing.T) { testActiveIdentityChange(t, change) })
	}
}
func testActiveIdentityChange(t *testing.T, change string) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var peers atomic.Value
	peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}})
	cb := &activeIdentityCallbacks{ownerCallbacks: ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}, started: make(chan struct{})}
	close(cb.release)
	if err := owner.startSessionHostWithPeerLookup(ln, "host", cb, time.Second, func() []daemon.SessionPeer { return peers.Load().([]daemon.SessionPeer) }); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mux, err := session.Dial(conn, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()
	select {
	case <-cb.started:
	case <-time.After(time.Second):
		t.Fatal("session did not start capture")
	}
	switch change {
	case "removed":
		peers.Store([]daemon.SessionPeer{})
	case "key-replaced":
		peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: "AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}})
	case "renamed":
		peers.Store([]daemon.SessionPeer{{Name: "new-name", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}})
	case "duplicate-source":
		peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}, {Name: "other", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}})
	case "duplicate-name":
		peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}, {Name: "VIEWER", IPv4: "127.0.0.2", PublicKey: fixturePeerKey}})
	default:
		t.Fatal("unknown change")
	}
	select {
	case <-mux.Closed():
	case <-time.After(time.Second):
		t.Fatal("known-revoked active peer retained its session")
	}
}

type blockedFrameCallbacks struct {
	activeIdentityCallbacks
	captureEntered chan struct{}
	captureRelease chan struct{}
}

func (c *blockedFrameCallbacks) Capture() ([]byte, error) {
	close(c.captureEntered)
	<-c.captureRelease
	return []byte("private frame"), nil
}

func TestActiveAuthorizationDiscardsCaptureThatOutlivesMap(t *testing.T) {
	var current atomic.Bool
	current.Store(true)
	access := &activeSessionAccess{current: current.Load}
	cb := &blockedFrameCallbacks{captureEntered: make(chan struct{}), captureRelease: make(chan struct{})}
	guarded := &authorizedHostCallbacks{HostCallbacks: cb, access: access}
	done := make(chan error, 1)
	go func() {
		frame, err := guarded.Capture()
		if len(frame) != 0 {
			done <- fmt.Errorf("revoked frame escaped")
			return
		}
		done <- err
	}()
	<-cb.captureEntered
	current.Store(false)
	close(cb.captureRelease)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("late capture accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("capture worker stuck")
	}
	current.Store(true)
	if access.allowed() {
		t.Fatal("restored map revived old session")
	}
	guarded.Inject(`{"type":"tap"}`)
	if cb.inputs.Load() != 0 {
		t.Fatal("revoked session injected input")
	}
}

func TestActiveAuthorizationBlocksInputBeforePlatformCallback(t *testing.T) {
	var current atomic.Bool
	current.Store(true)
	access := &activeSessionAccess{current: current.Load}
	cb := &activeIdentityCallbacks{}
	guarded := &authorizedHostCallbacks{HostCallbacks: cb, access: access}
	guarded.Inject("first")
	if cb.inputs.Load() != 1 {
		t.Fatal("authorized input suppressed")
	}
	current.Store(false)
	guarded.Inject("revoked")
	if cb.inputs.Load() != 1 {
		t.Fatal("known-revoked input dispatched")
	}
	current.Store(true)
	guarded.Inject("restored")
	if cb.inputs.Load() != 1 {
		t.Fatal("ended session resumed without fresh consent")
	}
}

// A stuck platform capture must not keep the connection alive. The platform
// callback itself remains cooperative and is released before cleanup completes.
func TestActiveMobileRevocationClosesMuxDuringBlockedCapture(t *testing.T) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var peers atomic.Value
	peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}})
	cb := &blockedFrameCallbacks{activeIdentityCallbacks: activeIdentityCallbacks{ownerCallbacks: ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}}, captureEntered: make(chan struct{}), captureRelease: make(chan struct{})}
	close(cb.release)
	var release sync.Once
	defer func() { release.Do(func() { close(cb.captureRelease) }); owner.StopSessionHost() }()
	if err := owner.startSessionHostWithPeerLookup(ln, "host", cb, time.Second, func() []daemon.SessionPeer { return peers.Load().([]daemon.SessionPeer) }); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mux, err := session.Dial(conn, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()
	select {
	case <-cb.captureEntered:
	case <-time.After(time.Second):
		t.Fatal("capture never entered")
	}
	peers.Store([]daemon.SessionPeer{})
	select {
	case <-mux.Closed():
	case <-time.After(time.Second):
		t.Fatal("blocked capture kept revoked connection alive")
	}
	release.Do(func() { close(cb.captureRelease) })
	until := time.Now().Add(time.Second)
	for cb.ends.Load() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if cb.ends.Load() != 1 {
		t.Fatal("session end not delivered exactly once")
	}
}

type stopDuringGeometryCallbacks struct {
	activeIdentityCallbacks
	owner *Node
}

func (c *stopDuringGeometryCallbacks) ScreenWidth() int { c.owner.StopSessionHost(); return 16 }

func TestActiveAuthorizationStartupAllowsReentrantStop(t *testing.T) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cb := &stopDuringGeometryCallbacks{activeIdentityCallbacks: activeIdentityCallbacks{ownerCallbacks: ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}}, owner: owner}
	close(cb.release)
	if err := startFixtureSessionHost(owner, ln, "host", cb, time.Second); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mux, err := session.Dial(conn, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()
	select {
	case <-mux.Closed():
	case <-time.After(time.Second):
		t.Fatal("geometry stop deadlocked")
	}
	until := time.Now().Add(time.Second)
	for cb.ends.Load() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if cb.ends.Load() != 1 {
		t.Fatal("startup cleanup omitted session end")
	}
}
