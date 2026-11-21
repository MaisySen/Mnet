//go:build multies_native_fixture

package control

// Explicit, disposable enrolled counterpart for denial-only attended integration.
// No capture, injector, OS network interface, real UI or product hook is installed.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/daemon"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/proofnet"
	"fintex.au/mnet/internal/session"
	"golang.zx2c4.com/wireguard/device"
)

type registrarAttendedFixture struct {
	network  *proofnet.Net
	reverse  atomic.Int32
	done     chan struct{}
	listener net.Listener
}
type attendedFixtureCoordinates struct {
	Version int                `json:"version"`
	Host    daemon.SessionPeer `json:"host"`
	Viewer  daemon.SessionPeer `json:"viewer"`
}
type attendedFixtureOutcome struct {
	ChallengeVerified bool   `json:"challengeVerified"`
	Version           int    `json:"version"`
	FakeUI            bool   `json:"fake_ui"`
	ListenerClosed    bool   `json:"listenerClosed"`
	ConnClosed        bool   `json:"connClosed"`
	MuxClosed         *bool  `json:"muxClosed"`
	LateOwnDenied     bool   `json:"lateOwnDenied"`
	ScopeReason       string `json:"scopeReason"`
	Case              string `json:"case"`
	AdmissionCalls    int    `json:"admissionCalls"`
	UIAsks            int    `json:"uiAsks"`
	Approved          int    `json:"approved"`
	Muxes             int    `json:"muxes"`
	CaptureCalls      int    `json:"captureCalls"`
	InputCreates      int    `json:"inputCreates"`
	Joined            bool   `json:"joined"`
}

func newRegistrarAttendedTransportFixture(t *testing.T, key [32]byte, ip string) *registrarTransportFixture {
	t.Helper()
	injected, network, err := proofnet.CreateNetTUN([]netip.Addr{netip.MustParseAddr(ip)}, nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := network.ListenTCPAddrPort(netip.AddrPortFrom(netip.MustParseAddr(ip), 49173))
	if err != nil {
		injected.Close()
		t.Fatal(err)
	}
	a := &registrarAttendedFixture{network: network, listener: ln, done: make(chan struct{})}
	f := &registrarTransportFixture{bind: bind.New(), attended: a}
	// One reverse challenge, explicitly joined. No RD session on this port.
	go func() {
		defer close(a.done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		var challenge [32]byte
		if _, err := io.ReadFull(c, challenge[:]); err != nil {
			return
		}
		reply := sha256.Sum256(challenge[:])
		if _, err := c.Write(reply[:]); err != nil {
			return
		}
		a.reverse.Add(1)
	}()
	t.Cleanup(func() {
		ln.Close()
		select {
		case <-a.done:
		case <-time.After(4 * time.Second):
			t.Error("reverse challenge owner not joined")
		}
	})
	f.dev = device.NewDevice(injected, f.bind, device.NewLogger(device.LogLevelSilent, "enrolled-attended-peer: "))
	t.Cleanup(f.dev.Close)
	if err := f.dev.IpcSetOperation(strings.NewReader("private_key=" + hex.EncodeToString(key[:]) + "\nlisten_port=0\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.dev.Up(); err != nil {
		t.Fatal(err)
	}
	return f
}
func runRegistrarAttendedFixture(t *testing.T, s *Store, f *registrarTransportFixture, dir string, cmd *exec.Cmd, output *bytes.Buffer, current func() (*netmap.Map, error), firstID, secondID string, nativeSecret func() string) {
	t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	var pin attendedFixtureCoordinates
	var nativeID string
	for {
		m, err := current()
		if err != nil {
			t.Fatal("current enrolled counterpart authorization", err)
		}
		var native *netmap.Node
		for _, p := range m.Peers {
			if p.ID != secondID {
				copy := p
				native = &copy
				break
			}
		}
		if native != nil && len(native.Endpoints) > 0 && strings.HasPrefix(native.Endpoints[0], "127.0.0.1:") {
			f.tick(t, m, *native)
			nativeID = native.ID
			next := attendedFixtureCoordinates{Version: 1, Host: daemon.SessionPeer{Name: native.Name, IPv4: native.IPv4, PublicKey: native.PubKey}, Viewer: daemon.SessionPeer{Name: m.Self.Name, IPv4: m.Self.IPv4, PublicKey: m.Self.PubKey}}
			if pin.Version == 0 {
				pin = next
				raw, _ := json.Marshal(pin)
				if err := writeRegistrarFixtureAtomic(dir, "peer-attended-coordinates.json", raw); err != nil {
					t.Fatal(err)
				}
			} else if pin != next {
				t.Fatal("attended peer pin changed")
			}
			if _, err := os.Stat(filepath.Join(dir, "native-attended-failed.json")); err == nil {
				raw, _ := os.ReadFile(filepath.Join(dir, "native-attended-failed.json"))
				t.Fatalf("native attended worker failure: %s", raw)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "native-attended-listening.json"))
			if err == nil {
				var ready struct {
					attendedFixtureCoordinates
					FakeUI bool `json:"fake_ui"`
				}
				if json.Unmarshal(raw, &ready) != nil || ready.attendedFixtureCoordinates != pin || !ready.FakeUI {
					t.Fatal("native listening coordinates not current enrolled map")
				}
				break
			}
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("native attended listener timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	validate := func() error {
		m, err := current()
		if err != nil {
			return err
		}
		if m.Self.PubKey != pin.Viewer.PublicKey || m.Self.IPv4 != pin.Viewer.IPv4 || m.Self.Name != pin.Viewer.Name {
			return fmt.Errorf("viewer identity changed")
		}
		for _, p := range m.Peers {
			if p.ID == nativeID && p.Approved && p.KeyExpiry != nil && time.Now().Before(*p.KeyExpiry) && p.Name == pin.Host.Name && p.IPv4 == pin.Host.IPv4 && p.PubKey == pin.Host.PublicKey {
				return nil
			}
		}
		return fmt.Errorf("host current identity unavailable")
	}
	if f.attended.reverse.Load() != 1 {
		t.Fatal("listener ready before actual reverse challenge completed")
	}
	if err := validate(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := f.attended.network.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(netip.MustParseAddr(pin.Host.IPv4), 49174))
	if err != nil {
		t.Fatal("actual encrypted attended dial", err)
	}
	defer conn.Close()
	far, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok || far == nil || !far.IP.Equal(net.ParseIP(pin.Host.IPv4)) {
		t.Fatal("actual attended TCP far address mismatch")
	}
	if err := validate(); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	verified := false
	mux, err := session.DialAttendedView(conn, pin.Viewer.Name, func(reply session.HelloReply, from net.Addr) error {
		verified = true
		if reply.NodeName != pin.Host.Name {
			return fmt.Errorf("wrong host name")
		}
		return validate()
	})
	if mux != nil {
		mux.Close()
		t.Fatal("denial-only fixture admitted viewer")
	}
	if err == nil || !strings.HasPrefix(err.Error(), "session: refused:") {
		t.Fatalf("not an explicit protocol refusal: %v", err)
	}
	// Refused replies return before the library's accepted-reply verifier.
	if verified {
		t.Fatal("unexpected accepted-reply verifier on refusal")
	}
	if err := validate(); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	refused, _ := json.Marshal(map[string]any{"version": 1, "fake_ui": true, "protocolRefused": true, "actualFarIP": far.IP.String(), "acceptedReplyVerifierCalled": false, "joined": true})
	if err := writeRegistrarFixtureAtomic(dir, "peer-attended-refused.json", refused); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	for {
		if _, err := os.Stat(filepath.Join(dir, "native-attended-failed.json")); err == nil {
			raw, _ := os.ReadFile(filepath.Join(dir, "native-attended-failed.json"))
			t.Fatalf("native attended worker failure: %s", raw)
		}
		raw, err = os.ReadFile(filepath.Join(dir, "native-attended-outcome.json"))
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("native attended outcome timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	var outcome attendedFixtureOutcome
	if json.Unmarshal(raw, &outcome) != nil || outcome.Version != 1 || !outcome.ChallengeVerified || !outcome.FakeUI || !outcome.ListenerClosed || !outcome.ConnClosed || outcome.MuxClosed != nil || !outcome.LateOwnDenied || outcome.ScopeReason == "" || outcome.Case != "deny" || outcome.AdmissionCalls != 1 || outcome.UIAsks != 1 || outcome.Approved != 0 || outcome.Muxes != 0 || outcome.CaptureCalls != 0 || outcome.InputCreates != 0 || !outcome.Joined {
		t.Fatalf("invalid actual host denial counters: %s", raw)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("native attended process failed: %v; %s", err, output.String())
	}
	raw, err = os.ReadFile(filepath.Join(dir, "native-workflow-outcome.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Receipt                                                                  contract.WorkflowReceiptWire
		NativeOwnedKeyConfirmed, NativeProofStateUnchanged, NativeStateFinalized bool
		NativeCurrentPeers                                                       int
	}
	if json.Unmarshal(raw, &result) != nil || !result.NativeOwnedKeyConfirmed || !result.NativeProofStateUnchanged || !result.NativeStateFinalized || result.NativeCurrentPeers != 2 || result.Receipt.Receipt.NodeID != nativeID || result.Receipt.Receipt.DevicePublicKey != pin.Host.PublicKey {
		t.Fatal("actual native result absent or not pinned")
	}
	s.mu.Lock()
	record := s.registrarNodeLocked(nativeID)
	s.mu.Unlock()
	if record == nil {
		t.Fatal("native record absent")
	}
	ref := registrarCredentialReference{NodeID: record.NodeID, NetworkID: record.NetworkID, DevicePublicKey: record.DevicePublicKey, Scope: record.Scope, ReceiptID: record.ReceiptID, ReceiptDigest: record.ReceiptDigest, DescriptorDigest: record.DescriptorDigest, TranscriptDigest: record.Consumption.TranscriptDigest}
	if m, err := s.registrarCredentialMap(ref, nativeSecret()); err != nil || len(m.Peers) != 2 {
		t.Fatal("actual native current map absent", err)
	}
	for _, id := range []string{firstID, secondID, nativeID} {
		if err := s.DeleteNode(id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.registrarCredentialMap(ref, nativeSecret()); err == nil {
		t.Fatal("deleted native credential still authorized")
	}
	final, _ := json.Marshal(map[string]any{"version": 1, "fake_ui": true, "attendedDenied": true, "actualEnrolledPeers": true, "joined": true, "inputAuthorized": false})
	if err := writeRegistrarFixtureAtomic(dir, "outcome.json", final); err != nil {
		t.Fatal(err)
	}
	t.Log("actual enrolled two-stack WireGuard/proofnet attended protocol refusal; exact current pins; fakeUI denial; no capture/input; owners joined")
}
