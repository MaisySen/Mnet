package daemon

import (
	"testing"
	"time"

	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/netmap"
)

func TestHealthyRelayStillRetriesDirectCandidate(t *testing.T) {
	now := time.Now()
	key := pubKeyOf(t, 0x51)
	st := &peerTrack{node: netmap.Node{PubKey: key, Endpoints: []string{"198.18.10.3:12345"}}, onRelay: true, lastSwitch: now.Add(-time.Minute)}
	d := &Daemon{peers: map[string]*peerTrack{key: st}}
	stats := map[string]*peerStats{key: {endpoint: bind.RelayPrefix + key, handshake: now.Unix()}}
	if !d.selectPaths(stats, now) || st.onRelay {
		t.Fatal("healthy relay pinned peer forever despite available direct candidate")
	}
	if st.lastDirectTry != now || st.lastSwitch != now {
		t.Fatal("direct retry did not record timing")
	}
	st.onRelay = true
	if d.selectPaths(stats, now.Add(5*time.Second)) || !st.onRelay {
		t.Fatal("relay retry is not rate limited")
	}
	if !d.selectPaths(stats, now.Add(91*time.Second)) || st.onRelay {
		t.Fatal("relay did not retry after cooldown")
	}
}

func TestPathSelectionKeepsHealthyDirectAndRelayWithoutCandidate(t *testing.T) {
	now := time.Now()
	key := pubKeyOf(t, 0x52)
	for _, tc := range []struct {
		name, endpoint string
		candidates     []string
		wantRelay      bool
	}{
		{"direct-roaming", "198.18.10.3:54321", []string{"198.18.10.3:12345"}, false},
		{"relay-no-candidate", bind.RelayPrefix + key, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &peerTrack{node: netmap.Node{PubKey: key, Endpoints: tc.candidates}, onRelay: true}
			d := &Daemon{peers: map[string]*peerTrack{key: st}}
			changed := d.selectPaths(map[string]*peerStats{key: {endpoint: tc.endpoint, handshake: now.Unix()}}, now)
			if st.onRelay != tc.wantRelay || changed == tc.wantRelay {
				t.Fatal("incorrect healthy-path decision")
			}
			if !st.lastDirectTry.IsZero() {
				t.Fatal("unneeded probe timer changed")
			}
		})
	}
}
