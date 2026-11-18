package mobile

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

func TestMobileViewerRejectsHostMissingFromKnownMap(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		m, err := session.Listen(ln, "host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		if err == nil {
			defer m.Close()
			<-m.Closed()
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	n := &Node{}
	v, err := n.connectAuthorizedSession(conn, "viewer", &viewerChecks{}, time.Second, func() []daemon.SessionPeer { return nil })
	if err == nil {
		v.Close()
		t.Error("host absent from current authorized map was admitted")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rejected connection leaked")
	}
}

func TestMobileViewerPinsHostAcrossConsentAndActiveSession(t *testing.T) {
	for _, change := range []string{"valid", "wrong-label", "removed-during-consent", "key-during-consent", "removed-active", "key-active"} {
		t.Run(change, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			var peers atomic.Value
			initial := []daemon.SessionPeer{{Name: "host", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}}
			peers.Store(initial)
			entered, release := make(chan struct{}), make(chan struct{})
			hostResult := make(chan *session.Mux, 1)
			label := "host"
			if change == "wrong-label" {
				label = "another-host"
			}
			go func() {
				m, _ := session.Listen(ln, label, func(session.Hello, net.Addr) (bool, string) { close(entered); <-release; return true, "" })
				hostResult <- m
			}()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			n := &Node{}
			defer n.stopViewers(false)
			cb := &identityViewerCallbacks{}
			type result struct {
				v   *SessionViewer
				err error
			}
			connected := make(chan result, 1)
			go func() {
				v, err := n.connectAuthorizedSession(conn, "viewer", cb, time.Second, func() []daemon.SessionPeer { return peers.Load().([]daemon.SessionPeer) })
				connected <- result{v, err}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("host did not receive hello")
			}
			revoke := func() {
				if strings.HasPrefix(change, "key-") {
					peers.Store([]daemon.SessionPeer{{Name: "host", IPv4: "127.0.0.1", PublicKey: "AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}})
				} else {
					peers.Store([]daemon.SessionPeer{})
				}
			}
			if strings.HasSuffix(change, "during-consent") {
				revoke()
			}
			close(release)
			var got result
			select {
			case got = <-connected:
			case <-time.After(2 * time.Second):
				t.Fatal("viewer handshake stuck")
			}
			host := <-hostResult
			if host != nil {
				defer host.Close()
			}
			rejected := change == "wrong-label" || strings.HasSuffix(change, "during-consent")
			if rejected {
				if got.err == nil {
					got.v.Close()
					t.Fatal("changed/unbound host admitted")
				}
				return
			}
			if got.err != nil {
				t.Fatal(got.err)
			}
			defer got.v.Close()
			if change == "valid" {
				peers.Store(initial)
				time.Sleep(160 * time.Millisecond)
				select {
				case <-got.v.mux.Closed():
					t.Fatal("valid current peer closed")
				default:
				}
				got.v.Close()
			} else {
				revoke()
				select {
				case <-got.v.mux.Closed():
				case <-time.After(time.Second):
					t.Fatal("revoked host connection retained")
				}
				if got.v.SendInputJSON(`{"type":"tap"}`) == nil {
					t.Fatal("revoked input accepted")
				}
				if got.v.SendChat("private") == nil {
					t.Fatal("revoked chat accepted")
				}
				if got.v.SendFile("must-not-open") == nil {
					t.Fatal("revoked file accepted")
				}
			}
			until := time.Now().Add(time.Second)
			for cb.closed.Load() == 0 && time.Now().Before(until) {
				time.Sleep(time.Millisecond)
			}
			if cb.closed.Load() != 1 {
				t.Fatal("viewer closed callback count", cb.closed.Load())
			}
		})
	}
}

type identityViewerCallbacks struct{ frames, chats, closed atomic.Int32 }

func (c *identityViewerCallbacks) OnFrame([]byte) { c.frames.Add(1) }
func (c *identityViewerCallbacks) OnChat(string)  { c.chats.Add(1) }
func (c *identityViewerCallbacks) OnClosed()      { c.closed.Add(1) }

func TestMobileViewerSuppressesCallbacksAfterKnownDenial(t *testing.T) {
	var current atomic.Bool
	current.Store(true)
	access := &activeSessionAccess{current: current.Load}
	cb := &identityViewerCallbacks{}
	guarded := &authorizedViewerCallbacks{ViewerCallbacks: cb, access: access}
	guarded.OnFrame([]byte("frame"))
	guarded.OnChat("chat")
	if cb.frames.Load() != 1 || cb.chats.Load() != 1 {
		t.Fatal("authorized callbacks suppressed")
	}
	current.Store(false)
	guarded.OnFrame([]byte("revoked"))
	guarded.OnChat("revoked")
	current.Store(true)
	guarded.OnFrame([]byte("restored"))
	guarded.OnChat("restored")
	if cb.frames.Load() != 1 || cb.chats.Load() != 1 {
		t.Fatal("ended session dispatched callbacks")
	}
}
