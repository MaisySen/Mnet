package daemon

import (
	"net"
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

func TestUnderlayCandidatesExcludeOverlayAddress(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", false}, {"198.18.10.2", true}, {"192.168.1.2", true}, {"127.0.0.1", false}, {"169.254.1.2", false},
	} {
		if got := usableUnderlayIP(net.ParseIP(tc.ip).To4(), "100.64.0.1"); got != tc.want {
			t.Errorf("%s: %v", tc.ip, got)
		}
	}
}

func TestRelayRoamingDoesNotCountAsDirectProbeSuccess(t *testing.T) {
	now := time.Now()
	key := pubKeyOf(t, 0x53)
	st := &peerTrack{node: netmap.Node{PubKey: key, Endpoints: []string{"198.18.10.3:12345", "198.18.10.4:12345"}}}
	d := &Daemon{peers: map[string]*peerTrack{key: st}}
	stats := map[string]*peerStats{key: {endpoint: bind.RelayPrefix + key, handshake: now.Unix()}}
	if !d.selectPaths(stats, now) || !st.probe || st.onRelay {
		t.Fatal("did not start authenticated direct probe")
	}
	st.probe = false
	if d.selectPaths(stats, now.Add(time.Second)) {
		t.Fatal("unbounded probe rate")
	}
	if !d.selectPaths(stats, now.Add(5*time.Second)) || !st.probe || st.cycle != 1 {
		t.Fatal("relay roaming suppressed next direct candidate")
	}
	if !d.selectPaths(stats, now.Add(31*time.Second)) || !st.onRelay {
		t.Fatal("failed direct probe window did not fall back")
	}
	if d.selectPaths(stats, now.Add(60*time.Second)) {
		t.Fatal("no cooldown after failed window")
	}
	if !d.selectPaths(stats, now.Add(91*time.Second)) || st.onRelay {
		t.Fatal("next bounded probe window did not open")
	}
}
