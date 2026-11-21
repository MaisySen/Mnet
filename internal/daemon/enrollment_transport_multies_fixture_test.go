//go:build multies_native_fixture

package daemon

// Opt-in local process fixture only. No product startup caller is added. A real
// WireGuard device uses this daemon's SAME already-durable native private key,
// in-memory TUN and loopback UDP. No OS interface or user device is activated.
import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fintex.au/mnet/internal/controlclient"
	"fmt"
	"golang.zx2c4.com/wireguard/tun/tuntest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testActualEnrolledNativeTransport(t *testing.T, d *Daemon, dir string) {
	t.Helper()
	if d.stateOwner.saved.native == nil || d.cc != nil || d.dev != nil || d.state.NodeToken != "" || d.state.AuthKey != "" {
		t.Fatal("transport requires exact sole native credential, no legacy path")
	}
	before, _ := os.ReadFile(d.stateOwner.path)
	keyBefore := d.stateOwner.saved.PublicKey
	limits := controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 16}
	tun := tuntest.NewChannelTUN()
	// OS TUN writes normally drain independently of applyMap. Continuously
	// drain this deliberately unbuffered test TUN while a peer is removed;
	// otherwise WireGuard RemovePeer can wait on a test-only blocked writer.
	packets := make(chan []byte, 1024)
	stopDrain := make(chan struct{})
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			select {
			case packet, ok := <-tun.Inbound:
				if !ok {
					return
				}
				select {
				case packets <- packet:
				case <-stopDrain:
					return
				}
			case <-stopDrain:
				return
			}
		}
	}()
	defer func() { close(stopDrain); <-drainDone }()
	d.cfg.TUN = tun.TUN()
	if err := d.startDevice(); err != nil {
		t.Fatal("actual retained-key WireGuard device", err)
	}
	defer d.stopDevice()
	if d.bind.Port() == 0 {
		t.Fatal("real WireGuard UDP bind absent")
	}
	if err := d.nativeCurrentEndpoints(context.Background(), []string{fmt.Sprintf("127.0.0.1:%d", d.bind.Port())}, limits); err != nil {
		t.Fatal(err)
	}
	m, err := d.nativeCurrentMap(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	var peerID, peerIP, peerKey string
	for _, peer := range m.Peers {
		if len(peer.Endpoints) == 1 && strings.HasPrefix(peer.Endpoints[0], "127.0.0.1:") {
			peerID = peer.ID
			peerIP = peer.IPv4
			peerKey = peer.PubKey
		}
	}
	if peerID == "" || m.Self.PubKey != keyBefore || !m.Self.Approved {
		t.Fatal("actual enrolled current map missing transport peer/own durable key")
	}
	// This calls the real map-to-device authorization/configuration path. No
	// manually constructed peer or legacy response is inserted into the daemon.
	d.applyMap(m)
	expected := tuntest.Ping(netip.MustParseAddr(m.Self.IPv4), netip.MustParseAddr(peerIP))
	deadline := time.Now().Add(5 * time.Second)
	received := false
	for !received && time.Now().Before(deadline) {
		select {
		case tun.Outbound <- tuntest.Ping(netip.MustParseAddr(peerIP), netip.MustParseAddr(m.Self.IPv4)):
		default:
		}
		select {
		case packet := <-packets:
			if !bytes.Equal(packet, expected) {
				t.Fatal("reverse decrypted packet/source/destination changed")
			}
			received = true
		case <-time.After(30 * time.Millisecond):
		}
	}
	if !received {
		t.Fatal("no reverse encrypted UDP packet from actually approved peer")
	}
	if err := writeFixtureAtomic(dir, "native-transport-ready", []byte("actual encrypted reverse payload verified")); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "transport-peer-revoked")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual Store revocation timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	revoked, err := d.nativeCurrentMap(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, peer := range revoked.Peers {
		if peer.ID == peerID {
			t.Fatal("revoked transport peer still returned by current source")
		}
	}
	d.applyMap(revoked)
	rawKey, _ := base64.StdEncoding.DecodeString(peerKey)
	ipc, err := d.dev.IpcGet()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ipc, "public_key="+hex.EncodeToString(rawKey)) {
		t.Fatal("actual WireGuard device retained revoked peer")
	}
	// Drain packets already decrypted before revocation. New outbound payloads
	// cannot select a removed peer. The parent also inspects its inbound channel.
	for {
		select {
		case <-packets:
		default:
			goto drained
		}
	}
drained:
	if err := writeFixtureAtomic(dir, "native-transport-removal-ready", []byte("WireGuard revoked peer absent")); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "transport-post-removal-barrier")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("post-removal drain barrier timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		select {
		case tun.Outbound <- tuntest.Ping(netip.MustParseAddr(peerIP), netip.MustParseAddr(m.Self.IPv4)):
		default:
		}
		select {
		case <-packets:
			t.Fatal("revoked peer encrypted payload reached native after removal")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := writeFixtureAtomic(dir, "native-transport-revocation-checked", []byte("actual map removed peer from WireGuard")); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		select {
		case <-packets:
			t.Fatal("revoked peer encrypted payload reached native during post-removal observation")
		default:
		}
		if _, err := os.Stat(filepath.Join(dir, "transport-post-removal-verified")); err == nil {
			select {
			case <-packets:
				t.Fatal("revoked peer late encrypted payload reached native")
			default:
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("post-removal packet denial not verified")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if after, _ := os.ReadFile(d.stateOwner.path); !bytes.Equal(before, after) || d.stateOwner.saved.PublicKey != keyBefore || d.cc != nil {
		t.Fatal("transport replaced durable native identity/State or created legacy client")
	}
}
