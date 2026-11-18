package mobile

import (
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifiedSessionPeerMatchesSourceAndUniqueCurrentName(t *testing.T) {
	peer := daemon.SessionPeer{Name: "Viewer", IPv4: "100.64.0.2", PublicKey: fixturePeerKey}
	source := &net.TCPAddr{IP: net.ParseIP(peer.IPv4), Port: 1234}
	cases := []struct {
		name, claim string
		peers       []daemon.SessionPeer
		from        net.Addr
		ok          bool
	}{
		{"canonical", "viewer", []daemon.SessionPeer{peer}, source, true},
		{"missing", "Viewer", nil, source, false},
		{"spoofed-name", "Owner", []daemon.SessionPeer{peer}, source, false},
		{"wrong-source", "Viewer", []daemon.SessionPeer{peer}, &net.TCPAddr{IP: net.ParseIP("100.64.0.3")}, false},
		{"no-source", "Viewer", []daemon.SessionPeer{peer}, nil, false},
		{"non-tcp", "Viewer", []daemon.SessionPeer{peer}, &net.UDPAddr{IP: source.IP}, false},
		{"whitespace", " Viewer", []daemon.SessionPeer{peer}, source, false},
		{"duplicate-name", "Viewer", []daemon.SessionPeer{peer, {Name: "viewer", IPv4: "100.64.0.3"}}, source, false},
		{"duplicate-source", "Viewer", []daemon.SessionPeer{peer, {Name: "Other", IPv4: peer.IPv4}}, source, false},
		{"nil-tcp-source", "Viewer", []daemon.SessionPeer{peer}, (*net.TCPAddr)(nil), false},
		{"empty-key", "Viewer", []daemon.SessionPeer{{Name: peer.Name, IPv4: peer.IPv4}}, source, false},
		{"malformed-key", "Viewer", []daemon.SessionPeer{{Name: peer.Name, IPv4: peer.IPv4, PublicKey: "not-base64"}}, source, false},
		{"short-key", "Viewer", []daemon.SessionPeer{{Name: peer.Name, IPv4: peer.IPv4, PublicKey: "AA=="}}, source, false},
		{"noncanonical-key", "Viewer", []daemon.SessionPeer{{Name: peer.Name, IPv4: peer.IPv4, PublicKey: fixturePeerKey + "\n"}}, source, false},
		{"invalid-map-ip", "Viewer", []daemon.SessionPeer{{Name: peer.Name, IPv4: "broken"}}, source, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, _, ok := verifiedSessionPeer(tc.peers, tc.claim, tc.from)
			if ok != tc.ok {
				t.Fatalf("accepted=%t", ok)
			}
			if ok && name != "Viewer" {
				t.Fatal("did not return verified map label")
			}
		})
	}
}

func TestMobileSessionSpoofDoesNotReachConsentAndValidPeerStillWorks(t *testing.T) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cb := &ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}
	close(cb.release)
	if err := startFixtureSessionHost(owner, ln, "host", cb, time.Second); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(time.Second))
	if mux, err := session.Dial(conn, "trusted-owner"); err == nil {
		mux.Close()
		t.Fatal("spoofed label admitted")
	}
	conn.Close()
	select {
	case <-cb.entered:
		t.Fatal("spoofed label displayed for consent")
	default:
	}
	conn, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	mux, err := session.Dial(conn, "viewer")
	if err != nil {
		t.Fatal("verified peer lost recovery", err)
	}
	defer mux.Close()
	select {
	case <-cb.entered:
	default:
		t.Fatal("verified peer did not request consent")
	}
}

func TestMobileSessionRevokedMapCannotUsePendingConsent(t *testing.T) {
	for _, change := range []string{"removed", "key-replaced", "renamed", "duplicate-source", "duplicate-name"} {
		t.Run(change, func(t *testing.T) { testPendingIdentityChange(t, change) })
	}
}
func testPendingIdentityChange(t *testing.T, change string) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var peers atomic.Value
	peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}})
	cb := &ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	defer once.Do(func() { close(cb.release) })
	if err := owner.startSessionHostWithPeerLookup(ln, "host", cb, time.Second, func() []daemon.SessionPeer { return peers.Load().([]daemon.SessionPeer) }); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	done := make(chan error, 1)
	go func() {
		mux, err := session.Dial(conn, "viewer")
		if mux != nil {
			mux.Close()
		}
		done <- err
	}()
	select {
	case <-cb.entered:
	case <-time.After(time.Second):
		t.Fatal("consent not requested")
	}
	switch change {
	case "key-replaced":
		peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: "AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}})
	case "renamed":
		peers.Store([]daemon.SessionPeer{{Name: "new-viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}})
	case "duplicate-source":
		peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}, {Name: "other", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}})
	case "duplicate-name":
		peers.Store([]daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}, {Name: "VIEWER", IPv4: "127.0.0.2", PublicKey: fixturePeerKey}})
	case "removed":
		peers.Store([]daemon.SessionPeer{})
	default:
		t.Fatal("unknown fixture change")
	}
	once.Do(func() { close(cb.release) })
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("revoked map used old approval")
		}
	case <-time.After(time.Second):
		t.Fatal("pending consent stuck")
	}
	if cb.captures.Load() != 0 {
		t.Fatal("revoked peer started capture")
	}
}

func TestMobileSessionMissingDaemonNeverFallsBackToClaimedName(t *testing.T) {
	owner := &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cb := &ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}
	close(cb.release)
	if err := owner.startSessionHost(ln, "host", cb); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	if mux, err := session.Dial(conn, "viewer"); err == nil {
		mux.Close()
		t.Fatal("missing map bypassed identity check")
	}
	select {
	case <-cb.entered:
		t.Fatal("unverified name reached consent")
	default:
	}
}

const fixturePeerKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
