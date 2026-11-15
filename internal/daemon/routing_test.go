package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/tuntest"

	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/stun"
)

func testDaemon(t *testing.T) *Daemon {
	t.Helper()
	var priv [32]byte
	rand.Read(priv[:])
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	tun := tuntest.NewChannelTUN()
	dev := device.NewDevice(tun.TUN(), bind.New(), device.NewLogger(device.LogLevelSilent, "test: "))
	if err := dev.IpcSetOperation(strings.NewReader("private_key=" + hex.EncodeToString(priv[:]) + "\nlisten_port=0\n")); err != nil {
		t.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dev.Close)
	return &Daemon{
		dev:    dev,
		state:  &State{},
		peers:  map[string]*peerTrack{},
		stunTx: map[stun.TxID]time.Time{},
	}
}

func pubKeyOf(t *testing.T, seed byte) string {
	t.Helper()
	var priv [32]byte
	for i := range priv {
		priv[i] = seed
	}
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub)
}

func allowedIPsOf(t *testing.T, d *Daemon, pub string) []string {
	t.Helper()
	var buf bytes.Buffer
	if err := d.dev.IpcGetOperation(&buf); err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(pub)
	want := hex.EncodeToString(raw)
	var out []string
	inPeer := false
	for _, line := range strings.Split(buf.String(), "\n") {
		k, v, _ := strings.Cut(line, "=")
		switch k {
		case "public_key":
			inPeer = v == want
		case "allowed_ip":
			if inPeer {
				out = append(out, v)
			}
		}
	}
	return out
}

// The map's routes and exit-node selection must become WireGuard allowed
// IPs: without that the device drops the traffic at the crypto layer.
func TestApplyMapRoutesAndExitNode(t *testing.T) {
	d := testDaemon(t)
	gwPub := pubKeyOf(t, 0x0A)
	plainPub := pubKeyOf(t, 0x0B)
	d.state.UseExitNode = "gateway"

	m := &netmap.Map{
		Version: 1,
		Peers: []netmap.Node{
			{Name: "gateway", PubKey: gwPub, IPv4: "100.64.0.2", ExitNode: true, Routes: []string{"10.9.0.0/24"}},
			{Name: "laptop", PubKey: plainPub, IPv4: "100.64.0.3"},
		},
	}
	d.applyMap(m)

	got := allowedIPsOf(t, d, gwPub)
	want := map[string]bool{"100.64.0.2/32": true, "10.9.0.0/24": true, "0.0.0.0/0": true, "::/0": true}
	for _, ip := range got {
		delete(want, ip)
	}
	if len(want) != 0 {
		t.Fatalf("gateway allowed ips missing %v, got %v", want, got)
	}

	got = allowedIPsOf(t, d, plainPub)
	if len(got) != 1 || got[0] != "100.64.0.3/32" {
		t.Fatalf("plain peer should only get its /32, got %v", got)
	}

	// Deselecting the exit node must remove the default routes.
	d.state.UseExitNode = ""
	d.applyMap(m)
	for _, ip := range allowedIPsOf(t, d, gwPub) {
		if ip == "0.0.0.0/0" || ip == "::/0" {
			t.Fatalf("default route leaked after deselect: %v", ip)
		}
	}
}

func TestApplyMapRemovesRevokedWireGuardPeer(t *testing.T) {
	d := testDaemon(t)
	key := pubKeyOf(t, 0x21)
	d.applyMap(&netmap.Map{Version: 1, Peers: []netmap.Node{{PubKey: key, IPv4: "100.64.0.2"}}})
	if len(allowedIPsOf(t, d, key)) != 1 {
		t.Fatal("fixture peer not installed")
	}
	d.applyMap(&netmap.Map{Version: 2})
	if len(allowedIPsOf(t, d, key)) != 0 {
		t.Fatal("revoked peer remains in WireGuard device")
	}
	d.peersMu.Lock()
	_, exists := d.peers[key]
	d.peersMu.Unlock()
	if exists {
		t.Fatal("revoked peer remains in path-selection table")
	}
}
