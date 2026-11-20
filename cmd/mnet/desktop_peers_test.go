package main

import (
	"context"
	"encoding/json"
	"fintex.au/mnet/internal/daemon"
	"io"
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

type desktopSnapshotTransport func(*http.Request) (*http.Response, error)

func (f desktopSnapshotTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDesktopPeerSnapshotAgeStartsBeforeOldestRead(t *testing.T) {
	var first time.Time
	status := `{"status":"up","peers":[{"name":"fixture","ipv4":"100.64.0.2"}]}`
	peers := `[{"name":"fixture","ipv4":"100.64.0.2","PublicKey":"` + strings.Repeat("A", 43) + `="}]`
	hc := &http.Client{Transport: desktopSnapshotTransport(func(r *http.Request) (*http.Response, error) {
		body := peers
		if r.URL.Path == "/localapi/status" {
			first = time.Now()
			body = status
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	got, err := readDesktopPeerSnapshot(context.Background(), hc, "http://snapshot.invalid")
	if err != nil || len(got.Peers) != 1 || got.Sampled.After(first) {
		t.Fatal(got, err, first)
	}
	s := newDesktopPeerSelection()
	s.replace(got)
	if !s.choose(0, first) {
		t.Fatal("fresh snapshot refused")
	}
	if _, err := s.choice(false, got.Sampled.Add(desktopPeerMaxAge)); err == nil {
		t.Fatal("oldest read age extended")
	}
}

func TestDesktopPeerSnapshotCanceledResponsesCannotPublish(t *testing.T) {
	for _, stage := range []string{"before", "status", "peers"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if stage == "before" {
				cancel()
			}
			calls := 0
			hc := &http.Client{Transport: desktopSnapshotTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body := `[]`
				if r.URL.Path == "/localapi/status" {
					body = `{"status":"up"}`
				}
				if stage == "status" || (stage == "peers" && calls == 2) {
					cancel()
				}
				// Deliberately return a body despite cancellation, as a late
				// custom transport or buffered response can do.
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			got, err := readDesktopPeerSnapshot(ctx, hc, "http://snapshot.invalid")
			want := map[string]int{"before": 0, "status": 1, "peers": 2}[stage]
			if err == nil || !got.Sampled.IsZero() || len(got.Peers) != 0 || calls != want {
				t.Fatal(got, err, calls, want)
			}
		})
	}
	if _, err := readDesktopPeerSnapshot(nil, nil, ""); err == nil {
		t.Fatal("nil owner accepted")
	}
}

func TestDesktopPeerEndpointsAreLocalCurrentSelectionOnly(t *testing.T) {
	now := time.Now()
	s := newDesktopPeerSelection()
	if s.endpoint(false, now) || s.endpoint(true, now) {
		t.Fatal("absent endpoints")
	}
	peers := make([]daemon.SessionPeer, 13)
	for _, count := range []int{0, 1, 6, 7, 13} {
		for _, last := range []bool{false, true} {
			s.replace(desktopPeerSnapshot{peers[:count], now})
			got := s.endpoint(last, now)
			if got != (count > 0) {
				t.Fatal(count, last, got)
			}
			if count > 0 {
				want := 0
				if last {
					want = count - 1
				}
				if s.selected != want {
					t.Fatal(count, last, s.selected, want)
				}
				s.clear()
				if _, err := s.choice(false, now); err == nil {
					t.Fatal("endpoint retained across clear")
				}
			}
		}
	}
	s.replace(desktopPeerSnapshot{peers, now})
	for _, stamp := range []time.Time{now.Add(-time.Nanosecond), now.Add(desktopPeerMaxAge)} {
		if s.endpoint(false, stamp) || s.endpoint(true, stamp) {
			t.Fatal("invalid age endpoint")
		}
	}
}

func TestDesktopPeerSelectedDetailsRemainExplicitAndRetire(t *testing.T) {
	now := time.Now()
	s := newDesktopPeerSelection()
	p := daemon.SessionPeer{Name: "fixture\n\x00é" + strings.Repeat("x", 200), IPv4: "100.64.0.2", PublicKey: strings.Repeat("A", 43) + "="}
	peers := make([]daemon.SessionPeer, 13)
	for i := range peers {
		peers[i] = p
	}
	s.replace(desktopPeerSnapshot{peers, now})
	if !s.choose(12, now) {
		t.Fatal("select last")
	}
	lines := s.detailLines(now)
	if len(lines) != 5 || !strings.Contains(lines[0], "Selected 13/13 label:") || !strings.Contains(lines[0], p.IPv4) || !strings.Contains(lines[1], p.PublicKey) {
		t.Fatal(lines)
	}
	for _, line := range lines {
		if len(line) > 143 || strings.ContainsAny(line, "\n\x00é") {
			t.Fatal("unbounded/unprintable detail", line)
		}
	}
	for _, stamp := range []time.Time{now.Add(-time.Nanosecond), now.Add(desktopPeerMaxAge)} {
		if strings.Contains(strings.Join(s.detailLines(stamp), " "), p.PublicKey) {
			t.Fatal("retired key visible")
		}
	}
	s.clear()
	if strings.Contains(strings.Join(s.detailLines(now), " "), p.PublicKey) {
		t.Fatal("cleared key visible")
	}
}

func TestDesktopPeerPageKeysNeverSelectOrExtendEvidence(t *testing.T) {
	now := time.Now()
	for _, count := range []int{0, 1, 6, 7, 12, 13, 4096} {
		for _, age := range []time.Duration{-time.Second, 0, desktopPeerMaxAge} {
			s := newDesktopPeerSelection()
			s.replace(desktopPeerSnapshot{make([]daemon.SessionPeer, count), now})
			s.choose(count-1, now)
			selected := s.selected
			end := 0
			if age == 0 && count > 0 {
				end = (count - 1) / 6
			}
			for _, page := range []int{-1, 0, 1, 682, 9999} {
				for _, key := range []uint32{0xff55, 0xff56} {
					got, handled := s.pageForKey(key, page, now.Add(age))
					want := page - 1
					if key == 0xff56 {
						want = page + 1
					}
					want = min(max(want, 0), end)
					if !handled || got != want || s.selected != selected || !s.snapshot.Sampled.Equal(now) {
						t.Fatal(count, age, page, key, got, want, s.selected, selected)
					}
				}
			}
			if page, handled := s.pageForKey(0xff54, 19, now); handled || page != 19 {
				t.Fatal("unrelated key handled")
			}
			s.clear()
			if page, _ := s.pageForKey(0xff56, 2, now); page != 0 {
				t.Fatal("cleared snapshot retained page")
			}
		}
	}
}
