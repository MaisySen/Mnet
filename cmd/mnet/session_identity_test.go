package main

import (
	"errors"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type cliOnceListener struct{ conn net.Conn }

func (l cliOnceListener) Accept() (net.Conn, error) { return l.conn, nil }
func (l cliOnceListener) Close() error              { return nil }
func (l cliOnceListener) Addr() net.Addr            { return l.conn.LocalAddr() }

func TestCLIRejectsHostLabelNotBoundToSelectedPeer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hostDone := make(chan *session.Mux, 1)
	go func() {
		m, _ := session.Listen(ln, "impostor", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		hostDone <- m
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	peer := daemon.SessionPeer{Name: "selected-host", IPv4: "127.0.0.1", PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
	mux, err := dialAuthorizedCLI(conn, "viewer", time.Second, peer, func() ([]daemon.SessionPeer, error) { return []daemon.SessionPeer{peer}, nil })
	if err == nil {
		mux.Close()
		t.Error("CLI admitted unbound host label")
	}
	host := <-hostDone
	if host != nil {
		host.Close()
	}
}

func TestCLISelectionRejectsAmbiguousOrReassignedDisplayPeer(t *testing.T) {
	const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	peer := daemon.SessionPeer{Name: "host", IPv4: "127.0.0.1", PublicKey: key}
	for _, tc := range []struct {
		name, selector string
		status         []daemon.PeerStatus
		peers          []daemon.SessionPeer
		ok             bool
	}{
		{"name", "host", []daemon.PeerStatus{{Name: "host", IPv4: peer.IPv4}}, []daemon.SessionPeer{peer}, true},
		{"fqdn", "host.mesh", []daemon.PeerStatus{{Name: "host", IPv4: peer.IPv4, FQDN: "host.mesh"}}, []daemon.SessionPeer{peer}, true},
		{"ip", "127.0.0.1", []daemon.PeerStatus{{Name: "host", IPv4: peer.IPv4}}, []daemon.SessionPeer{peer}, true},
		{"duplicate-selector", "host", []daemon.PeerStatus{{Name: "host", IPv4: peer.IPv4}, {Name: "host", IPv4: "127.0.0.2"}}, []daemon.SessionPeer{peer}, false},
		{"reassigned-ip", "host", []daemon.PeerStatus{{Name: "old-host", IPv4: peer.IPv4, FQDN: "host"}}, []daemon.SessionPeer{peer}, false},
		{"no-current-map", "host", []daemon.PeerStatus{{Name: "host", IPv4: peer.IPv4}}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := selectCLISessionPeer(tc.selector, &daemon.Status{Peers: tc.status}, tc.peers)
			if (err == nil) != tc.ok {
				t.Fatal("selection", err)
			}
		})
	}
}

func TestCLIAdmissionAndActivePeerChanges(t *testing.T) {
	for _, change := range []string{"valid", "removed-consent", "key-consent", "removed-active", "key-active", "unavailable-active"} {
		t.Run(change, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			peer := daemon.SessionPeer{Name: "host", IPv4: "127.0.0.1", PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
			var peers atomic.Value
			peers.Store([]daemon.SessionPeer{peer})
			var unavailable atomic.Bool
			lookup := func() ([]daemon.SessionPeer, error) {
				if unavailable.Load() {
					return nil, errors.New("local API down")
				}
				return peers.Load().([]daemon.SessionPeer), nil
			}
			access := &cliSessionAuthorization{peer: peer, lookup: lookup}
			entered, release := make(chan struct{}), make(chan struct{})
			hostDone := make(chan *session.Mux, 1)
			go func() {
				m, _ := session.Listen(ln, "host", func(session.Hello, net.Addr) (bool, string) { close(entered); <-release; return true, "" })
				hostDone <- m
			}()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			type result struct {
				m   *session.Mux
				err error
			}
			got := make(chan result, 1)
			go func() { m, err := dialAuthorizedCLI(conn, "viewer", time.Second, peer, lookup); got <- result{m, err} }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("hello not sent")
			}
			revoke := func() {
				switch {
				case strings.HasPrefix(change, "key"):
					p := peer
					p.PublicKey = "AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
					peers.Store([]daemon.SessionPeer{p})
				case change == "unavailable-active":
					unavailable.Store(true)
				default:
					peers.Store([]daemon.SessionPeer{})
				}
			}
			if strings.HasSuffix(change, "consent") {
				revoke()
			}
			close(release)
			r := <-got
			host := <-hostDone
			if host != nil {
				defer host.Close()
			}
			if strings.HasSuffix(change, "consent") {
				if r.err == nil {
					r.m.Close()
					t.Fatal("changed host admitted")
				}
				return
			}
			if r.err != nil {
				t.Fatal(r.err)
			}
			defer r.m.Close()
			if !access.check() {
				t.Fatal("valid peer denied")
			}
			if change == "valid" {
				return
			}
			go access.watch(r.m)
			revoke()
			select {
			case <-r.m.Closed():
			case <-time.After(2 * time.Second):
				t.Fatal("known-invalid host retained CLI")
			}
			peers.Store([]daemon.SessionPeer{peer})
			unavailable.Store(false)
			if access.check() {
				t.Fatal("reapproval revived pinned CLI run")
			}
		})
	}
}

type cliEvidenceTransport func(*http.Request) (*http.Response, error)

func (f cliEvidenceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCLISessionEvidenceRejectsUnverifiableReplies(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"valid", `[{"Name":"host","IPv4":"127.0.0.1","PublicKey":"full-key"}]`, 200, true},
		{"empty", "null", 200, true},
		{"missing-route", "not supported", 404, false},
		{"denied", "no", 401, false},
		{"redirect", "", 302, false},
		{"malformed", "[", 200, false},
		{"trailing-json", "[]{}", 200, false},
		{"oversized", strings.Repeat(" ", (4<<20)+1), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hc := &http.Client{Transport: cliEvidenceTransport(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet || req.URL.Path != "/localapi/session-peers" {
					t.Error("wrong evidence request")
				}
				if _, ok := req.Context().Deadline(); !ok {
					t.Error("evidence request has no deadline")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			_, err := readCLISessionPeers(hc)
			if (err == nil) != tc.ok {
				t.Fatal("evidence result", err)
			}
		})
	}
}
