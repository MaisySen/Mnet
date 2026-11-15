package daemon_test

// End to end: two real WireGuard devices (in-memory TUNs) join a real
// control server, learn each other from the map, and move encrypted
// traffic first over direct UDP, then over the relay WebSocket.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/tuntest"

	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

type testNode struct {
	name string
	priv [32]byte
	pub  string
	join netmap.JoinResponse
	tun  *tuntest.ChannelTUN
	bind *bind.Bind
	dev  *device.Device
	ip   string
}

func newTestNode(t *testing.T, name string) *testNode {
	t.Helper()
	n := &testNode{name: name, tun: tuntest.NewChannelTUN()}
	rand.Read(n.priv[:])
	n.priv[0] &= 248
	n.priv[31] = (n.priv[31] & 127) | 64
	pub, err := curve25519.X25519(n.priv[:], curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	n.pub = base64.StdEncoding.EncodeToString(pub)
	return n
}

func (n *testNode) startDevice(t *testing.T) {
	t.Helper()
	n.bind = bind.New()
	logger := device.NewLogger(device.LogLevelSilent, n.name+": ")
	n.dev = device.NewDevice(n.tun.TUN(), n.bind, logger)
	conf := "private_key=" + hex.EncodeToString(n.priv[:]) + "\nlisten_port=0\n"
	if err := n.dev.IpcSetOperation(strings.NewReader(conf)); err != nil {
		t.Fatal(err)
	}
	if err := n.dev.Up(); err != nil {
		t.Fatal(err)
	}
}

func (n *testNode) addPeer(t *testing.T, p netmap.Node, endpoint string) {
	t.Helper()
	raw, _ := base64.StdEncoding.DecodeString(p.PubKey)
	conf := fmt.Sprintf("public_key=%s\nallowed_ip=%s/32\npersistent_keepalive_interval=25\nendpoint=%s\n",
		hex.EncodeToString(raw), p.IPv4, endpoint)
	if err := n.dev.IpcSetOperation(strings.NewReader(conf)); err != nil {
		t.Fatal(err)
	}
}

func (n *testNode) ping(t *testing.T, dst, src string) {
	t.Helper()
	d, _ := netip.ParseAddr(dst)
	s, _ := netip.ParseAddr(src)
	n.tun.Outbound <- tuntest.Ping(d, s)
}

func (n *testNode) wantPacket(t *testing.T, via string) []byte {
	t.Helper()
	select {
	case pkt := <-n.tun.Inbound:
		return pkt
	case <-time.After(15 * time.Second):
		t.Fatalf("%s got nothing via %s", n.name, via)
		return nil
	}
}

// pingUntil keeps pinging until one lands: relay transport is UDP, a
// single lost packet must not sink the mesh.
func pingUntil(t *testing.T, from, to *testNode, via string) []byte {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		from.ping(t, to.ip, from.ip)
		select {
		case pkt := <-to.tun.Inbound:
			if len(pkt) > 0 && pkt[0]>>4 == 4 {
				return pkt
			}
		case <-time.After(1500 * time.Millisecond):
		}
	}
	t.Fatalf("%s got nothing via %s", to.name, via)
	return nil
}

func adminPost(t *testing.T, base, cookie, path string, body any) []byte {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	out.ReadFrom(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%s: %d %s", path, resp.StatusCode, out.String())
	}
	return out.Bytes()
}

func TestMeshEndToEnd(t *testing.T) {
	// Real control plane.
	store, err := control.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	srv, _, err := control.NewServer(store, relay.NewHub(), "http://placeholder", "pw")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := ts.URL

	// Admin session + network + reusable auth key.
	lb, _ := json.Marshal(map[string]string{"password": "pw"})
	lr, err := http.Post(base+"/admin/api/login", "application/json", bytes.NewReader(lb))
	if err != nil {
		t.Fatal(err)
	}
	cookie := lr.Cookies()[0].String()
	lr.Body.Close()
	netBody := adminPost(t, base, cookie, "/admin/api/networks", map[string]string{"name": "home"})
	var network netmap.Network
	json.Unmarshal(netBody, &network)
	keyBody := adminPost(t, base, cookie, "/admin/api/authkeys", map[string]any{"network_id": network.ID, "reusable": true})
	var key netmap.AuthKey
	json.Unmarshal(keyBody, &key)

	// Two nodes.
	a := newTestNode(t, "laptop")
	b := newTestNode(t, "server")
	for _, n := range []*testNode{a, b} {
		cc := &controlclient.Client{Base: base}
		jr, err := cc.Join(context.Background(), netmap.JoinRequest{
			Network: "home", Name: n.name, PubKey: n.pub, AuthKey: key.Key,
		})
		if err != nil {
			t.Fatalf("%s join: %v", n.name, err)
		}
		if !jr.Approved {
			t.Fatalf("%s not approved with auth key", n.name)
		}
		n.join = *jr
		n.startDevice(t)
		defer n.dev.Close()
	}
	ccA := &controlclient.Client{Base: base, NodeID: a.join.NodeID, Token: a.join.NodeToken}
	ccB := &controlclient.Client{Base: base, NodeID: b.join.NodeID, Token: b.join.NodeToken}

	// B reports its real loopback endpoint; A learns the map.
	bPort := 0
	{
		// Open happens inside device.Up; read the bound port.
		deadline := time.Now().Add(2 * time.Second)
		for b.bind.Port() == 0 && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		bPort = int(b.bind.Port())
		if bPort == 0 {
			t.Fatal("b bind has no port")
		}
	}
	if err := ccB.ReportEndpoints(context.Background(), []string{fmt.Sprintf("127.0.0.1:%d", bPort)}); err != nil {
		t.Fatal(err)
	}
	m, err := ccA.WaitMap(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Peers) != 1 || m.Peers[0].Name != "server" {
		t.Fatalf("a's map: %+v", m.Peers)
	}
	peerB := m.Peers[0]
	a.ip = m.Self.IPv4
	b.ip = peerB.IPv4
	mb, err := ccB.WaitMap(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	peerA := mb.Peers[0]

	// Direct path: A encrypts to B over loopback UDP.
	a.addPeer(t, peerB, fmt.Sprintf("127.0.0.1:%d", bPort))
	b.addPeer(t, peerA, "") // B learns A's endpoint from roaming
	pkt := pingUntil(t, a, b, "direct udp")
	t.Logf("direct path: %d byte packet decrypted on B", len(pkt))

	// Relay path: no direct endpoints, packets ride the WebSocket relay.
	relays := make([]*controlclient.Relay, 0, 2)
	for _, n := range []*testNode{a, b} {
		cc := &controlclient.Client{Base: base, NodeID: n.join.NodeID, Token: n.join.NodeToken}
		r, err := cc.DialRelay(context.Background(), n.pub, n.bind.Inject)
		if err != nil {
			t.Fatal(err)
		}
		relays = append(relays, r)
		defer r.Close()
		n.bind.SetRelaySender(r.Send)
	}
	a.addPeer(t, peerB, bind.RelayPrefix+peerB.PubKey)
	b.addPeer(t, peerA, bind.RelayPrefix+peerA.PubKey)
	pkt = pingUntil(t, a, b, "relay")
	t.Logf("relay path: %d byte packet decrypted on B through the control server", len(pkt))

	// And back from B to A over the relay, proving roaming stayed sane.
	// Allow a couple of tries: a packet can be dropped mid path switch.
	pkt = pingUntil(t, b, a, "relay reverse")
	t.Logf("relay reverse path works too")
}
