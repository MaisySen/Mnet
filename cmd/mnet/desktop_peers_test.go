package main

import (
	"context"
	"encoding/json"
	"fintex.au/mnet/internal/daemon"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDesktopEligiblePeersRequireExactUnambiguousCurrentEvidence(t *testing.T) {
	key := strings.Repeat("A", 43) + "="
	p := daemon.SessionPeer{Name: "peer", IPv4: "100.64.0.2", PublicKey: key}
	st := &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{{Name: p.Name, IPv4: p.IPv4}}}
	for _, tc := range []struct {
		name   string
		state  string
		status []daemon.PeerStatus
		peers  []daemon.SessionPeer
		want   int
		fail   bool
	}{
		{"exact", "up", st.Peers, []daemon.SessionPeer{p}, 1, false},
		{"down", "down", st.Peers, []daemon.SessionPeer{p}, 0, true},
		{"missing", "up", nil, []daemon.SessionPeer{p}, 0, false},
		{"duplicate status", "up", append(st.Peers, st.Peers...), []daemon.SessionPeer{p}, 0, false},
		{"duplicate authorization", "up", st.Peers, []daemon.SessionPeer{p, p}, 0, false},
		{"wrong key", "up", st.Peers, []daemon.SessionPeer{{Name: p.Name, IPv4: p.IPv4, PublicKey: "invalid"}}, 0, false},
		{"different name", "up", []daemon.PeerStatus{{Name: "other", IPv4: p.IPv4}}, []daemon.SessionPeer{p}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := desktopEligiblePeers(&daemon.Status{Status: tc.state, Peers: tc.status}, tc.peers)
			if (err != nil) != tc.fail || len(got) != tc.want {
				t.Fatal(got, err)
			}
		})
	}
}
func TestDesktopPeerSelectionNeverCarriesAcrossSnapshotOrExpiry(t *testing.T) {
	now := time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC)
	p := daemon.SessionPeer{Name: "fixture", IPv4: "100.64.0.2", PublicKey: strings.Repeat("A", 43) + "="}
	s := newDesktopPeerSelection()
	if s.choose(0, now) {
		t.Fatal("absent selection")
	}
	s.replace(desktopPeerSnapshot{[]daemon.SessionPeer{p}, now})
	if _, err := s.choice(false, now); err == nil {
		t.Fatal("implicit selection")
	}
	if !s.choose(0, now) {
		t.Fatal("explicit selection refused")
	}
	for _, pointer := range []bool{false, true} {
		c, err := s.choice(pointer, now.Add(29*time.Second))
		if err != nil || c.Peer != p || c.Pointer != pointer {
			t.Fatal(c, err)
		}
	}
	for _, stamp := range []time.Time{now.Add(-time.Nanosecond), now.Add(30 * time.Second)} {
		if _, err := s.choice(true, stamp); err == nil {
			t.Fatal("future/expired snapshot")
		}
	}
	s.replace(desktopPeerSnapshot{[]daemon.SessionPeer{p}, now})
	if s.selected != -1 {
		t.Fatal("refresh kept selection")
	}
	s.choose(0, now)
	s.clear()
	if _, err := s.choice(false, now); err == nil {
		t.Fatal("failed refresh kept selection")
	}
}
func TestDesktopPeerSnapshotOnlyReadsBoundedLocalEvidence(t *testing.T) {
	key := strings.Repeat("A", 43) + "="
	for _, bad := range []bool{false, true} {
		calls := []string{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, r.Method+" "+r.URL.Path)
			if r.URL.Path == "/localapi/status" {
				json.NewEncoder(w).Encode(daemon.Status{Status: "up", Peers: []daemon.PeerStatus{{Name: "fixture", IPv4: "100.64.0.2"}}})
				return
			}
			if bad {
				w.Write([]byte(`[{"name":"fixture"}]trailing`))
				return
			}
			json.NewEncoder(w).Encode([]daemon.SessionPeer{{Name: "fixture", IPv4: "100.64.0.2", PublicKey: key}})
		}))
		got, err := readDesktopPeerSnapshot(context.Background(), server.Client(), server.URL)
		server.Close()
		if (err != nil) != bad || len(calls) != 2 || calls[0] != "GET /localapi/status" || calls[1] != "GET /localapi/session-peers" {
			t.Fatal(got, err, calls)
		}
		if !bad && (len(got.Peers) != 1 || got.Sampled.IsZero()) {
			t.Fatal(got)
		}
	}
}
