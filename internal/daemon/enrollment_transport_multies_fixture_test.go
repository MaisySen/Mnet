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
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/tuntest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var developmentTCPTransport func(*testing.T, *Daemon, string)
var developmentTransportStart func(*testing.T, *Daemon, tun.Device) (func(), func() error)

func testActualEnrolledNativeTransport(t *testing.T, d *Daemon, dir string, relayMode bool) {
	t.Helper()
	if os.Getenv("MNET_DEVELOPMENT_NETWORK_TCP") == "1" {
		if developmentTCPTransport == nil {
			t.Fatal("explicit tagged TCP controller missing")
		}
		developmentTCPTransport(t, d, dir)
		return
	}
	if d.stateOwner.saved.native == nil || d.cc != nil || d.dev != nil || d.state.NodeToken != "" || d.state.AuthKey != "" {
		t.Fatal("transport requires exact sole native credential, no legacy path")
	}
	before, _ := os.ReadFile(d.stateOwner.path)
	keyBefore := d.stateOwner.saved.PublicKey
	limits := controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 2, MaxRequests: 16}
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
	var refreshDevelopment func() error
	if developmentTransportStart != nil && !relayMode && os.Getenv("MNET_DEVELOPMENT_NETWORK_SESSION") == "1" {
		stop, refresh := developmentTransportStart(t, d, tun.TUN())
		refreshDevelopment = refresh
		defer stop()
	} else {
		d.cfg.TUN = tun.TUN()
		if err := d.startDevice(); err != nil {
			t.Fatal("actual retained-key WireGuard device", err)
		}
		defer d.stopDevice()
	}
	if d.bind.Port() == 0 {
		t.Fatal("real WireGuard UDP bind absent")
	}
	endpoints := []string{fmt.Sprintf("127.0.0.1:%d", d.bind.Port())}
	if relayMode {
		endpoints = nil
	}
	if err := d.nativeCurrentEndpoints(context.Background(), endpoints, limits); err != nil {
		t.Fatal(err)
	}
	m, err := d.nativeCurrentMap(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	var relaySocket *nativeRegistrarRelay
	if relayMode {
		relaySocket, err = d.nativeCurrentRelay(context.Background(), limits, 256)
		if err != nil {
			t.Fatal(err)
		}
		defer relaySocket.Close()
		d.bind.SetRelaySender(relaySocket.Send)
		defer d.bind.SetRelaySender(nil)
		if err := writeFixtureAtomic(dir, "native-relay-socket-ready", []byte("dedicated native relay socket and zero-endpoint map ready")); err != nil {
			t.Fatal(err)
		}
	}
	targetID, _ := os.ReadFile(filepath.Join(dir, "transport-peer-id"))
	var peerID, peerIP, peerKey string
	for _, peer := range m.Peers {
		if relayMode && peer.ID == string(targetID) || !relayMode && len(peer.Endpoints) == 1 && strings.HasPrefix(peer.Endpoints[0], "127.0.0.1:") {
			peerID = peer.ID
			peerIP = peer.IPv4
			peerKey = peer.PubKey
		}
	}
	if relayMode && len(m.Self.Endpoints) != 0 {
		t.Fatal("relay-only native advertises direct endpoint")
	}
	if peerID == "" || m.Self.PubKey != keyBefore || !m.Self.Approved {
		t.Fatal("actual enrolled current map missing transport peer/own durable key")
	}
	if relayMode {
		readyDeadline := time.Now().Add(time.Second)
		for {
			if _, err := os.Stat(filepath.Join(dir, "transport-relay-peer-configured")); err == nil {
				break
			}
			if time.Now().After(readyDeadline) {
				t.Fatal("relay peer configuration readiness timeout")
			}
			time.Sleep(5 * time.Millisecond)
		}
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
	if relayMode {
		// Explicit disposable caller reopens ONCE using the same retained
		// client/State and original receipt. No automatic product retry or
		// alternate credential/lease/timeout is selected.
		old := relaySocket
		clientBefore := d.nativeClient
		old.Close()
		select {
		case <-old.Done():
		case <-time.After(time.Second):
			t.Fatal("retired socket owner did not finish")
		}
		if err := old.Send(peerKey, []byte{1}); err == nil {
			t.Fatal("retired native socket still sends")
		}
		relaySocket, err = d.nativeCurrentRelay(context.Background(), limits, 256)
		if err != nil {
			t.Fatal("same original credential explicit socket reopen", err)
		}
		defer relaySocket.Close()
		if relaySocket == old || d.nativeClient != clientBefore {
			t.Fatal("socket replacement reset shared client owner")
		}
		afterReopen, err := os.ReadFile(d.stateOwner.path)
		if err != nil || !bytes.Equal(before, afterReopen) || d.stateOwner.saved.PublicKey != keyBefore {
			t.Fatal("socket replacement rewrote authority or identity")
		}
		d.bind.SetRelaySender(relaySocket.Send)
		if err := writeFixtureAtomic(dir, "native-relay-reopened", []byte("same original receipt and retained client, fresh socket")); err != nil {
			t.Fatal(err)
		}
		nextExpected := append([]byte{}, expected...)
		nextExpected[len(nextExpected)-1] ^= 0x31
		nextOutbound := tuntest.Ping(netip.MustParseAddr(peerIP), netip.MustParseAddr(m.Self.IPv4))
		nextOutbound[len(nextOutbound)-1] ^= 0x31
		freshReceived := false
		freshPeerReceived := false
		until := time.Now().Add(2 * time.Second)
		for time.Now().Before(until) && (!freshReceived || !freshPeerReceived) {
			select {
			case tun.Outbound <- nextOutbound:
			default:
			}
			select {
			case packet := <-packets:
				if bytes.Equal(packet, nextExpected) {
					freshReceived = true
				} else if !bytes.Equal(packet, expected) {
					t.Fatal("reopened socket decrypted unexpected packet")
				}
			case <-time.After(10 * time.Millisecond):
			}
			_, err := os.Stat(filepath.Join(dir, "peer-relay-reopened-received"))
			freshPeerReceived = err == nil
		}
		if !freshReceived || !freshPeerReceived {
			t.Fatal("fresh marked encrypted packet missing after explicit socket replacement", freshReceived, freshPeerReceived)
		}
		t.Log("actual retained native owner socket closed/reopened once; same State/client/original receipt; fresh marked encrypted packets both directions")
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
		if relayMode {
			select {
			case tun.Outbound <- tuntest.Ping(netip.MustParseAddr(peerIP), netip.MustParseAddr(m.Self.IPv4)):
			default:
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if refreshDevelopment != nil {
		if err := refreshDevelopment(); err != nil {
			t.Fatal("explicit development map refresh", err)
		}
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
	postPacket := tuntest.Ping(netip.MustParseAddr(peerIP), netip.MustParseAddr(m.Self.IPv4))
	if relayMode {
		postPacket[len(postPacket)-1] ^= 0x5a
	}
	preReopenExpected := append([]byte{}, expected...)
	preReopenExpected[len(preReopenExpected)-1] ^= 0x31
	deniedInbound := func(packet []byte) bool {
		return !relayMode || !bytes.Equal(packet, expected) && !bytes.Equal(packet, preReopenExpected)
	}
	for i := 0; i < 5; i++ {
		select {
		case tun.Outbound <- postPacket:
		default:
		}
		select {
		case packet := <-packets:
			if deniedInbound(packet) {
				t.Fatal("revoked peer encrypted payload reached native after removal")
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := writeFixtureAtomic(dir, "native-transport-revocation-checked", []byte("actual map removed peer from WireGuard")); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		select {
		case packet := <-packets:
			if deniedInbound(packet) {
				t.Fatal("revoked peer encrypted payload reached native during post-removal observation")
			}
		default:
		}
		if _, err := os.Stat(filepath.Join(dir, "transport-post-removal-verified")); err == nil {
			select {
			case packet := <-packets:
				if deniedInbound(packet) {
					t.Fatal("revoked peer late encrypted payload reached native")
				}
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
