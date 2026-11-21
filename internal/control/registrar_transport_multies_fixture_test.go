//go:build multies_native_fixture

package control

// Disposable real WireGuard endpoint for the already-enrolled first fixture
// peer. Its key is the SAME one proved to the Store, no legacy join/token path.
import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/netmap"
	"fmt"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/tuntest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type registrarTransportFixture struct {
	tun      *tuntest.ChannelTUN
	bind     *bind.Bind
	dev      *device.Device
	lastKey  string
	received int
	sent     bool
	revoked  bool
}

func newRegistrarTransportFixture(t *testing.T, key [32]byte) *registrarTransportFixture {
	t.Helper()
	f := &registrarTransportFixture{tun: tuntest.NewChannelTUN(), bind: bind.New()}
	f.dev = device.NewDevice(f.tun.TUN(), f.bind, device.NewLogger(device.LogLevelSilent, "enrolled-peer: "))
	if err := f.dev.IpcSetOperation(strings.NewReader("private_key=" + hex.EncodeToString(key[:]) + "\nlisten_port=0\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.dev.Up(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.dev.Close)
	return f
}
func (f *registrarTransportFixture) endpoint() string {
	return fmt.Sprintf("127.0.0.1:%d", f.bind.Port())
}
func (f *registrarTransportFixture) tick(t *testing.T, m *netmap.Map, native netmap.Node) {
	t.Helper()
	if !native.Approved || native.KeyExpiry == nil || time.Now().After(*native.KeyExpiry) || len(native.Endpoints) != 1 || !strings.HasPrefix(native.Endpoints[0], "127.0.0.1:") {
		return
	}
	// Map must contain the exact approved native key, not public IPC configuration.
	found := false
	for _, peer := range m.Peers {
		if peer.ID == native.ID && peer.PubKey == native.PubKey {
			found = true
		}
	}
	if !found {
		t.Fatal("current first-peer map omitted native transport authorization")
	}
	if f.lastKey == "" {
		key, err := base64.StdEncoding.Strict().DecodeString(native.PubKey)
		if err != nil || len(key) != 32 {
			t.Fatal("native key grammar")
		}
		conf := fmt.Sprintf("public_key=%s\nallowed_ip=%s/32\nendpoint=%s\n", hex.EncodeToString(key), native.IPv4, native.Endpoints[0])
		if err := f.dev.IpcSetOperation(strings.NewReader(conf)); err != nil {
			t.Fatal(err)
		}
		f.lastKey = native.PubKey
	}
	expected := tuntest.Ping(netip.MustParseAddr(m.Self.IPv4), netip.MustParseAddr(native.IPv4))
	for {
		select {
		case packet := <-f.tun.Inbound:
			if !bytes.Equal(packet, expected) {
				t.Fatal("decrypted payload/source/destination differs from enrolled native packet")
			}
			f.received++
		default:
			goto drained
		}
	}
drained:
	if !f.revoked {
		select {
		case f.tun.Outbound <- tuntest.Ping(netip.MustParseAddr(native.IPv4), netip.MustParseAddr(m.Self.IPv4)):
			f.sent = true
		default:
		}
	}
}
