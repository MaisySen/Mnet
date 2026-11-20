package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
)

func TestDeviceSnapshotUsesOnlyReadOnlyStatusAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  int
		body  string
		valid bool
	}{{"valid", 200, `{"status":"up","approval_url":"private-url"}`, true}, {"unknown state", 200, `{"status":"new-state"}`, true}, {"refused", 403, `private-error`, false}, {"missing state", 200, `{}`, false}, {"malformed", 200, `not-json`, false}, {"trailing", 200, `{"status":"up"}{}`, false}, {"too many peers", 200, `{"status":"up","peers":[` + strings.Repeat(`{},`, 4096) + `{}]}`, false}, {"too large", 200, strings.Repeat(" ", 1<<20) + `{}`, false}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/localapi/status" || r.URL.RawQuery != "" {
					t.Error("read exceeded status scope")
				}
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			st, err := readDeviceSnapshot(context.Background(), s.Client(), s.URL)
			if (err == nil) != tc.valid || calls != 1 {
				t.Fatal(err, calls)
			}
			if tc.valid && st == nil {
				t.Fatal("no snapshot")
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Fatal("raw error leaked")
			}
		})
	}
}
func TestDeviceSnapshotCancellationJoinsWithoutMutation(t *testing.T) {
	entered := make(chan struct{})
	closed := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(closed) }))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := readDeviceSnapshot(ctx, s.Client(), s.URL); done <- err }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled status accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("reader did not end")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("request not canceled")
	}
}
func TestDeviceTextAndSnapshotNeverRenderPrivateFieldsOrNonUpPeers(t *testing.T) {
	st := &daemon.Status{Status: "up", NodeName: "local\n\x00é", Network: "fixture", ApprovalURL: "PRIVATE_APPROVAL", Server: "PRIVATE_SERVER", LastError: "PRIVATE_ERROR", PublicKey: "PRIVATE_KEY", Peers: []daemon.PeerStatus{{Name: "peer\n\x1b", IPv4: "100.64.0.2", Endpoint: "PRIVATE_ENDPOINT", HandshakeState: "clock-skew", Online: true, LastHandshake: 0}}}
	lines, page := deviceSnapshotLines(st, 9, time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC))
	all := strings.Join(lines, "\n")
	if page != 0 || strings.Contains(all, "PRIVATE_") || strings.Contains(all, "é") || strings.Contains(all, "\x1b") || !strings.Contains(all, "unknown") || !strings.Contains(all, "clock skew") {
		t.Fatal(all)
	}
	for _, state := range []string{"down", "preparing", "joining", "waiting-approval", "key-expired", "unrecognized\nstate"} {
		st.Status = state
		lines, p := deviceSnapshotLines(st, 2, time.Now())
		all = strings.Join(lines, "\n")
		if p != 0 || strings.Contains(all, "100.64.0.2") || strings.Contains(all, "PRIVATE_") || strings.Contains(all, "unrecognized") {
			t.Fatal(state, all)
		}
	}
	if got := deviceText(strings.Repeat("x", 300), 32); len(got) != 32 {
		t.Fatal(len(got))
	}
}
func TestDeviceSnapshotPaginationNeverDropsOrInventsRows(t *testing.T) {
	st := &daemon.Status{Status: "up"}
	for i := 0; i < 19; i++ {
		st.Peers = append(st.Peers, daemon.PeerStatus{Name: fmt.Sprintf("peer-%02d", i), HandshakeState: "unavailable", LastHandshake: -1})
	}
	for _, tc := range []struct {
		page, want  int
		first, last string
	}{{-1, 0, "peer-00", "peer-07"}, {0, 0, "peer-00", "peer-07"}, {1, 1, "peer-08", "peer-15"}, {2, 2, "peer-16", "peer-18"}, {99, 2, "peer-16", "peer-18"}} {
		lines, p := deviceSnapshotLines(st, tc.page, time.Now())
		all := strings.Join(lines, "\n")
		if p != tc.want || !strings.Contains(all, tc.first) || !strings.Contains(all, tc.last) {
			t.Fatal(tc, all)
		}
	}
}
func TestDevicePeerStateDoesNotPromoteUnknownOrFutureTelemetry(t *testing.T) {
	if got := devicePeerState(daemon.PeerStatus{Online: true, HandshakeState: "recent", LastHandshake: -1}); got != "unknown" {
		t.Fatal("missing age became online", got)
	}
	for _, state := range []string{"unavailable", "clock-skew", "new-state"} {
		if got := devicePeerState(daemon.PeerStatus{Online: true, HandshakeState: state}); got != "unknown" {
			t.Fatal(state, got)
		}
	}
	for _, state := range []string{"never", "stale"} {
		if got := devicePeerState(daemon.PeerStatus{Online: true, HandshakeState: state}); got != "offline" {
			t.Fatal(state, got)
		}
	}
}
