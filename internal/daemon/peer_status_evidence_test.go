package daemon

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/localapi"
	"fintex.au/mnet/internal/netmap"
)

func TestPeerStatusNoStatsDoesNotInventRecentHandshake(t *testing.T) {
	d := testDaemon(t)
	d.mu.Lock()
	d.lastMap = &netmap.Map{Network: netmap.Network{Name: "fixture"}, Peers: []netmap.Node{{Name: "fixture-peer", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), IPv4: "100.64.0.2"}}}
	d.mu.Unlock()
	peers := d.PeerStatuses()
	if len(peers) != 1 || peers[0].LastHandshake != -1 || peers[0].Online {
		t.Fatalf("no device evidence presented as a recent handshake: %+v", peers)
	}
}
func TestPeerStatusMalformedKeyCannotPanicLocalStatus(t *testing.T) {
	d := testDaemon(t)
	d.mu.Lock()
	d.lastMap = &netmap.Map{Peers: []netmap.Node{{Name: "invalid", PubKey: "short"}}}
	d.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("malformed map key crashed private status: %v", r)
		}
	}()
	if peers := d.PeerStatuses(); len(peers) != 0 {
		t.Fatal("unusable key exposed as peer evidence", peers)
	}
}

func TestPeerStatusHandshakeEvidenceBoundaries(t *testing.T) {
	now := time.Unix(2000000000, 0)
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	m := &netmap.Map{Peers: []netmap.Node{{Name: "fixture", PubKey: key}}}
	for _, tc := range []struct {
		name   string
		stats  *peerStats
		state  string
		age    int64
		online bool
	}{
		{"absent", nil, "unavailable", -1, false},
		{"no-handshake", &peerStats{}, "never", -1, false},
		{"future", &peerStats{handshake: now.Unix() + 1}, "clock-skew", -1, false},
		{"far-future", &peerStats{handshake: 9223372036854775807}, "clock-skew", -1, false},
		{"now", &peerStats{handshake: now.Unix()}, "recent", 0, true},
		{"recent-limit", &peerStats{handshake: now.Unix() - 179}, "recent", 179, true},
		{"stale-limit", &peerStats{handshake: now.Unix() - 180}, "stale", 180, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := renderPeerStatuses(m, map[string]*peerStats{key: tc.stats}, now)
			if len(got) != 1 || got[0].LastHandshake != tc.age || got[0].Online != tc.online || got[0].HandshakeState != tc.state {
				t.Fatal(got)
			}
		})
	}
}
func TestPeerStatusMapAndStatsNotMutated(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	m := &netmap.Map{Peers: []netmap.Node{{Name: "fixture", PubKey: key}}}
	stat := &peerStats{endpoint: "127.0.0.1:12345", handshake: 50, rx: 4, tx: 8}
	got := renderPeerStatuses(m, map[string]*peerStats{key: stat}, time.Unix(100, 0))
	if len(got) != 1 || got[0].Rx != 4 || got[0].Tx != 8 || got[0].Endpoint != stat.endpoint || got[0].PubKeyShort != "AAAAAAAAAAAA..." {
		t.Fatal(got)
	}
	got[0].Name = "changed"
	if m.Peers[0].Name != "fixture" || stat.rx != 4 {
		t.Fatal("render changed source")
	}
}

func TestPrivateStatusAPIDoesNotInventMissingStats(t *testing.T) {
	d := testDaemon(t)
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	d.mu.Lock()
	d.status = "up"
	d.lastMap = &netmap.Map{Network: netmap.Network{Name: "fixture"}, Peers: []netmap.Node{{Name: "valid", PubKey: key}, {Name: "unusable", PubKey: "short"}}}
	d.mu.Unlock()
	handler := d.localAPIHandler("fixture-status-capability")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/localapi/status", nil))
	if denied.Code == http.StatusOK {
		t.Fatal("status capability bypass")
	}
	req := httptest.NewRequest(http.MethodGet, "/localapi/status", nil)
	req.Header.Set(localapi.TokenHeader, "fixture-status-capability")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatal(response.Code)
	}
	var st Status
	if err := json.NewDecoder(response.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if len(st.Peers) != 1 || st.Peers[0].Name != "valid" || st.Peers[0].LastHandshake != -1 || st.Peers[0].HandshakeState != "unavailable" || st.Peers[0].Online {
		t.Fatal(st.Peers)
	}
}

func TestStatusKeepsNodeAndPeerMapSnapshotWhileStatsWait(t *testing.T) {
	d := testDaemon(t)
	first := base64.StdEncoding.EncodeToString(make([]byte, 32))
	raw := make([]byte, 32)
	raw[0] = 1
	second := base64.StdEncoding.EncodeToString(raw)
	d.mu.Lock()
	d.status = "up"
	d.lastMap = &netmap.Map{Peers: []netmap.Node{{Name: "first-map", PubKey: first}}}
	d.mu.Unlock()
	d.deviceMu.Lock()
	locked := true
	defer func() {
		if locked {
			d.deviceMu.Unlock()
		}
	}()
	result := make(chan *Status, 1)
	go func() { result <- d.Status() }()
	until := time.Now().Add(time.Second)
	stack := make([]byte, 1<<20)
	for {
		n := runtime.Stack(stack, true)
		waiting := false
		for _, g := range strings.Split(string(stack[:n]), "\n\n") {
			if strings.Contains(g, "(*Daemon).Status(") && strings.Contains(g, "(*Daemon).stats(") && strings.Contains(g, "sync.(*RWMutex).RLock") {
				waiting = true
				break
			}
		}
		if waiting {
			break
		}
		if time.Now().After(until) {
			t.Fatal("Status did not reach held device counter read")
		}
		runtime.Gosched()
	}
	changed := make(chan struct{})
	go func() {
		d.applyMu.Lock()
		d.mu.Lock()
		d.status = "key-expired"
		d.lastMap = &netmap.Map{Peers: []netmap.Node{{Name: "second-map", PubKey: second}}}
		d.mu.Unlock()
		d.applyMu.Unlock()
		close(changed)
	}()
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("diagnostic IPC blocked map/authorization publication")
	}
	d.deviceMu.Unlock()
	locked = false
	select {
	case st := <-result:
		if st.Status != "up" || len(st.Peers) != 1 || st.Peers[0].Name != "first-map" {
			t.Fatal("mixed node/peer map generations", st.Status, st.Peers)
		}
	case <-time.After(time.Second):
		t.Fatal("Status did not finish")
	}
}
