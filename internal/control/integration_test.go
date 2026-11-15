package control_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

// testEnv spins up a real control server on a random port.
func testEnv(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	store, err := control.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	srv, pw, err := control.NewServer(store, relay.NewHub(), "http://placeholder", "test-admin-pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = pw
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, "test-admin-pw"
}

func post(t *testing.T, url string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := new(bytes.Buffer)
	out.ReadFrom(resp.Body)
	return resp.StatusCode, out.Bytes()
}

func adminLogin(t *testing.T, base, pw string) *http.Cookie {
	t.Helper()
	code, _ := post(t, base+"/admin/api/login", map[string]string{"password": pw}, nil)
	if code != 200 {
		t.Fatalf("admin login: %d", code)
	}
	// Re-do with cookie capture.
	b, _ := json.Marshal(map[string]string{"password": pw})
	req, _ := http.NewRequest(http.MethodPost, base+"/admin/api/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "mnet_admin" {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func fakePubKey(i byte) string {
	var k [32]byte
	for j := range k {
		k[j] = i
	}
	return base64.StdEncoding.EncodeToString(k[:])
}

// TestJoinApproveMapRelay walks the whole node lifecycle.
func TestJoinApproveMapRelay(t *testing.T) {
	ts, pw := testEnv(t)
	base := ts.URL
	cookie := adminLogin(t, base, pw)

	// Wrong password is refused.
	if code, _ := post(t, base+"/admin/api/login", map[string]string{"password": "nope"}, nil); code != 401 {
		t.Fatalf("bad password accepted: %d", code)
	}
	// Admin API requires the session.
	if code, _ := post(t, base+"/admin/api/networks", map[string]string{"name": "home"}, nil); code != 401 {
		t.Fatalf("admin api open: %d", code)
	}

	// Create the network.
	code, body := post(t, base+"/admin/api/networks", map[string]string{"name": "home"}, map[string]string{"Cookie": cookie.String()})
	if code != 200 {
		t.Fatalf("create network: %d %s", code, body)
	}
	var net netmap.Network
	json.Unmarshal(body, &net)
	if net.Name != "home" {
		t.Fatalf("network name: %+v", net)
	}

	// Two nodes join without keys: both pending.
	join := func(name string, i byte) netmap.JoinResponse {
		code, body := post(t, base+"/api/join", netmap.JoinRequest{Network: "home", Name: name, PubKey: fakePubKey(i)}, nil)
		if code != 200 {
			t.Fatalf("join %s: %d %s", name, code, body)
		}
		var jr netmap.JoinResponse
		json.Unmarshal(body, &jr)
		return jr
	}
	a := join("laptop", 0xAA)
	b := join("server", 0xBB)
	if a.Approved || b.Approved {
		t.Fatal("joins should be pending")
	}
	if a.ApprovalURL == "" || !strings.Contains(a.ApprovalURL, a.NodeID) {
		t.Fatalf("approval url missing: %+v", a)
	}

	// Map before approval is refused.
	headers := func(j netmap.JoinResponse) map[string]string {
		return map[string]string{"X-Node-ID": j.NodeID, "X-Node-Token": j.NodeToken}
	}
	if code, _ := getMap(t, base, a, headers(a)); code != 403 {
		t.Fatalf("map before approval: %d", code)
	}

	// Approve laptop only.
	code, _ = post(t, base+"/admin/api/nodes/approve", map[string]string{"node_id": a.NodeID}, map[string]string{"Cookie": cookie.String()})
	if code != 200 {
		t.Fatalf("approve: %d", code)
	}
	code, m := getMap(t, base, a, headers(a))
	if code != 200 {
		t.Fatalf("map after approval: %d", code)
	}
	if len(m.Peers) != 0 {
		t.Fatalf("unapproved peer leaked into map: %+v", m.Peers)
	}
	if m.Self.IPv4 == "" || !strings.HasPrefix(m.Self.IPv4, "100.64.") {
		t.Fatalf("bad self ip: %+v", m.Self)
	}

	// Approve server, report endpoints, laptop sees it.
	post(t, base+"/admin/api/nodes/approve", map[string]string{"node_id": b.NodeID}, map[string]string{"Cookie": cookie.String()})
	code, _ = post(t, base+"/api/endpoints", netmap.EndpointsUpdate{Endpoints: []string{"203.0.113.7:51820"}}, headers(b))
	if code != 200 {
		t.Fatalf("endpoints: %d", code)
	}
	code, m = getMap(t, base, a, headers(a))
	if code != 200 || len(m.Peers) != 1 {
		t.Fatalf("map with one peer: %d %+v", code, m.Peers)
	}
	if m.Peers[0].Endpoints[0] != "203.0.113.7:51820" {
		t.Fatalf("endpoints not propagated: %+v", m.Peers[0])
	}

	// Long poll wakes the moment anything in the network changes.
	type waitRes struct {
		code int
		m    *netmap.Map
		took time.Duration
	}
	resc := make(chan waitRes, 1)
	go func() {
		start := time.Now()
		code, m2 := getMapWait(t, base, a, headers(a), m.Version)
		resc <- waitRes{code, m2, time.Since(start)}
	}()
	time.Sleep(400 * time.Millisecond)
	post(t, base+"/admin/api/authkeys", map[string]any{"network_id": net.ID, "note": "wake", "reusable": true}, map[string]string{"Cookie": cookie.String()})
	// Auth keys do not bump the map; approving a node does. Join and approve one.
	code2, body2 := post(t, base+"/api/join", netmap.JoinRequest{Network: "home", Name: "waker", PubKey: fakePubKey(0x77)}, nil)
	if code2 != 200 {
		t.Fatalf("waker join: %d %s", code2, body2)
	}
	var waker netmap.JoinResponse
	json.Unmarshal(body2, &waker)
	post(t, base+"/admin/api/nodes/approve", map[string]string{"node_id": waker.NodeID}, map[string]string{"Cookie": cookie.String()})
	select {
	case r := <-resc:
		if r.code != 200 || r.m.Version <= m.Version {
			t.Fatalf("long poll stale: %d v%d", r.code, r.m.Version)
		}
		if r.took > 10*time.Second {
			t.Fatalf("long poll did not wake on change: %s", r.took)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("long poll never returned")
	}

	// Auth key join skips approval.
	code, body = post(t, base+"/admin/api/authkeys", map[string]any{"network_id": net.ID, "note": "ci", "reusable": false, "ttl_hours": 1}, map[string]string{"Cookie": cookie.String()})
	if code != 200 {
		t.Fatalf("make key: %d %s", code, body)
	}
	var key netmap.AuthKey
	json.Unmarshal(body, &key)
	code, body = post(t, base+"/api/join", netmap.JoinRequest{Network: "home", Name: "ci-runner", PubKey: fakePubKey(0xCC), AuthKey: key.Key}, nil)
	if code != 200 {
		t.Fatalf("key join: %d %s", code, body)
	}
	var jr netmap.JoinResponse
	json.Unmarshal(body, &jr)
	if !jr.Approved {
		t.Fatal("auth key join should be approved at once")
	}
	// Single use key cannot join twice.
	code, _ = post(t, base+"/api/join", netmap.JoinRequest{Network: "home", Name: "ci-runner-2", PubKey: fakePubKey(0xCD), AuthKey: key.Key}, nil)
	if code != 403 {
		t.Fatalf("single use key reused: %d", code)
	}

	// Relay: both nodes attach, one forwards a packet to the other.
	wsURL := func(j netmap.JoinResponse) string {
		return strings.Replace(base, "http", "ws", 1) + "/api/relay/ws?node_id=" + j.NodeID + "&token=" + j.NodeToken
	}
	dial := func(u string) *websocket.Conn {
		c, _, err := websocket.DefaultDialer.Dial(u, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	wsA, wsB := dial(wsURL(a)), dial(wsURL(b))
	defer wsA.Close()
	defer wsB.Close()
	time.Sleep(100 * time.Millisecond)

	payload := []byte("wireguard-encrypted-blob")
	dst, _ := base64.StdEncoding.DecodeString(fakePubKey(0xAA))
	frame := append(dst, payload...)
	if err := wsB.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}
	wsA.SetReadDeadline(time.Now().Add(3 * time.Second))
	mt, msg, err := wsA.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.BinaryMessage || len(msg) < 33 {
		t.Fatalf("relay frame: %d %d", mt, len(msg))
	}
	if got := base64.StdEncoding.EncodeToString(msg[:32]); got != fakePubKey(0xBB) {
		t.Fatalf("wrong source: %s", got)
	}
	if !bytes.Equal(msg[32:], payload) {
		t.Fatal("payload corrupted in relay")
	}

	// Node deletion pulls it from maps.
	post(t, base+"/admin/api/nodes/delete", map[string]string{"node_id": b.NodeID}, map[string]string{"Cookie": cookie.String()})
	_, m3 := getMap(t, base, a, headers(a))
	names := map[string]bool{}
	for _, p := range m3.Peers {
		names[p.Name] = true
	}
	if names["server"] || !names["ci-runner"] || !names["waker"] {
		t.Fatalf("delete did not propagate: %+v", m3.Peers)
	}
}

func getMap(t *testing.T, base string, j netmap.JoinResponse, headers map[string]string) (int, *netmap.Map) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/map", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m netmap.Map
	json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, &m
}

func getMapWait(t *testing.T, base string, j netmap.JoinResponse, headers map[string]string, since int64) (int, *netmap.Map) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/map?since=%d&wait=1", base, since), nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m netmap.Map
	json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, &m
}

// TestIPAllocation hands out distinct CGNAT addresses.
func TestIPAllocation(t *testing.T) {
	store, err := control.Open(t.TempDir() + "/s.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateNetwork("home"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		resp, err := store.Join(netmap.JoinRequest{Network: "home", Name: fmt.Sprintf("n%d", i), PubKey: fakePubKey(byte(i + 1))})
		if err != nil {
			t.Fatal(err)
		}
		st, err := store.JoinStatus(resp.NodeID, resp.NodeToken)
		if err != nil {
			t.Fatal(err)
		}
		if seen[st.IPv4] {
			t.Fatalf("duplicate ip %s", st.IPv4)
		}
		seen[st.IPv4] = true
	}
}
