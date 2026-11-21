//go:build multies_native_fixture

package control

// Disposable real WireGuard endpoint for the already-enrolled first fixture
// peer. Its key is the SAME one proved to the Store, no legacy join/token path.
import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fintex.au/mnet/internal/bind"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/netmap"
	"fmt"
	"github.com/gorilla/websocket"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/tuntest"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

type registrarTransportFixture struct {
	attended       *registrarAttendedFixture
	tcp            *registrarTCPFixture
	tun            *tuntest.ChannelTUN
	bind           *bind.Bind
	dev            *device.Device
	lastKey        string
	received       int
	sent           bool
	revoked        bool
	selfIP         string
	reverseSent    int
	relayMode      bool
	reopen         bool
	reopenReceived int
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
	if !native.Approved || native.KeyExpiry == nil || time.Now().After(*native.KeyExpiry) || (!f.relayMode && (len(native.Endpoints) != 1 || !strings.HasPrefix(native.Endpoints[0], "127.0.0.1:"))) {
		return
	}
	if f.relayMode && (len(native.Endpoints) != 0 || len(m.Self.Endpoints) != 0) {
		t.Fatal("relay-only participants advertised direct endpoints")
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
	f.selfIP = m.Self.IPv4
	if f.lastKey == "" {
		key, err := base64.StdEncoding.Strict().DecodeString(native.PubKey)
		if err != nil || len(key) != 32 {
			t.Fatal("native key grammar")
		}
		endpoint := bind.RelayPrefix + native.PubKey
		if !f.relayMode {
			endpoint = native.Endpoints[0]
		}
		conf := fmt.Sprintf("public_key=%s\nallowed_ip=%s/32\nendpoint=%s\n", hex.EncodeToString(key), native.IPv4, endpoint)
		if err := f.dev.IpcSetOperation(strings.NewReader(conf)); err != nil {
			t.Fatal(err)
		}
		f.lastKey = native.PubKey
	}
	if f.attended != nil {
		return
	}
	if f.tcp != nil {
		select {
		case <-f.tcp.exchanged:
			f.received++
			f.sent = true
		default:
		}
		return
	}
	expected := tuntest.Ping(netip.MustParseAddr(m.Self.IPv4), netip.MustParseAddr(native.IPv4))
	for {
		select {
		case packet := <-f.tun.Inbound:
			reopened := append([]byte{}, expected...)
			reopened[len(reopened)-1] ^= 0x31
			if f.reopen && bytes.Equal(packet, reopened) {
				f.reopenReceived++
			} else if !bytes.Equal(packet, expected) {
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
		case f.tun.Outbound <- func() []byte {
			p := tuntest.Ping(netip.MustParseAddr(native.IPv4), netip.MustParseAddr(m.Self.IPv4))
			if f.reopen {
				p[len(p)-1] ^= 0x31
			}
			return p
		}():
			f.sent = true
		default:
		}
	}
}

// The revoked test device deliberately retains its old authorized native peer
// and working session. Only native's actual fresh-map removal may deny receive.
func (f *registrarTransportFixture) sendRevokedReverse(t *testing.T, native netmap.Node) {
	t.Helper()
	if !f.revoked || f.lastKey != native.PubKey || f.selfIP == "" {
		t.Fatal("reverse revoked sender identity mismatch")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(native.PubKey)
	if err != nil {
		t.Fatal(err)
	}
	ipc, err := f.dev.IpcGet()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ipc, "public_key="+hex.EncodeToString(key)) || !strings.Contains(ipc, "allowed_ip="+native.IPv4+"/32") {
		t.Fatal("revoked sender no longer retains exact native peer/destination")
	}
	postPacket := tuntest.Ping(netip.MustParseAddr(native.IPv4), netip.MustParseAddr(f.selfIP))
	if f.relayMode {
		postPacket[len(postPacket)-1] ^= 0x5a
	}
	select {
	case f.tun.Outbound <- postPacket:
		f.reverseSent++
	case <-time.After(time.Second):
		t.Fatal("revoked peer reverse send not enqueued")
	}
}

func (f *registrarTransportFixture) connectRelay(t *testing.T, server *httptest.Server, receipt contract.HistoryReceiptWire, secret string) {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set("Authorization", registrarRelayScheme+secret)
	headers.Set(registrarRelayReferenceHeader, base64.RawURLEncoding.EncodeToString(raw))
	dialer := websocket.Dialer{TLSClientConfig: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone(), HandshakeTimeout: time.Second}
	ws, _, err := dialer.Dial("wss"+strings.TrimPrefix(server.URL, "https")+registrarRelayPath, headers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	var writeMu sync.Mutex
	f.bind.SetRelaySender(func(dst string, packet []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		// This short finite fixture deliberately has one handshake initiator.
		// Simultaneous WG initiations overwrite each other's handshake state;
		// retry is 5s, beyond the explicitly fixed 3s native socket. The test
		// peer only responds; encrypted data remains bidirectional.
		if len(packet) > 0 && packet[0] == 1 {
			return nil
		}
		key, err := base64.StdEncoding.Strict().DecodeString(dst)
		if err != nil {
			return err
		}
		frame := append(key, packet...)
		ws.SetWriteDeadline(time.Now().Add(time.Second))
		return ws.WriteMessage(websocket.BinaryMessage, frame)
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.SetReadLimit(2080)
		for {
			typ, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if typ == websocket.BinaryMessage && len(msg) > 32 {
				f.bind.Inject(base64.StdEncoding.EncodeToString(msg[:32]), msg[32:])
			}
		}
	}()
	t.Cleanup(func() { ws.Close(); <-done })
}
