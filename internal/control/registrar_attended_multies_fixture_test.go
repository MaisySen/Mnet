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
	"errors"
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
	PromptCancelled   *bool  `json:"promptCancelled,omitempty"`
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
	caseName := os.Getenv("MNET_DEVELOPMENT_ATTENDED_CASE")
	if caseName == "" {
		caseName = "deny"
	}
	if caseName != "deny" && caseName != "withdraw-pending" {
		t.Fatal("unknown attended case")
	}
	withdraw := caseName == "withdraw-pending"
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
	type dialResult struct {
		err      error
		verified bool
		admitted bool
	}
	dialDone := make(chan dialResult, 1)
	go func() {
		verified := false
		mux, err := session.DialAttendedView(conn, pin.Viewer.Name, func(reply session.HelloReply, from net.Addr) error {
			verified = true
			if reply.NodeName != pin.Host.Name {
				return fmt.Errorf("wrong host name")
			}
			return validate()
		})
		admitted := mux != nil
		if mux != nil {
			mux.Close()
		}
		dialDone <- dialResult{err, verified, admitted}
	}()
	var localClose attendedLocalCloseEvidence
	if withdraw {
		for {
			pendingRaw, err := os.ReadFile(filepath.Join(dir, "native-attended-pending.json"))
			if err == nil {
				var pending struct {
					Version                int    `json:"version"`
					FakeUI                 bool   `json:"fake_ui"`
					Case                   string `json:"case"`
					AdmissionCalls, UIAsks int
					Host, Viewer           daemon.SessionPeer
				}
				if json.Unmarshal(pendingRaw, &pending) != nil {
					t.Fatal("invalid pending prompt marker")
				}
				// Admission counter fields use lower camel case, accepted by encoding/json.
				if pending.Version != 1 || !pending.FakeUI || pending.Case != caseName || pending.AdmissionCalls != 1 || pending.UIAsks != 1 || pending.Host != pin.Host || pending.Viewer != pin.Viewer {
					t.Fatal("pending prompt not correlated to exact real peer")
				}
				break
			}
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatal("pending prompt marker timeout")
			}
			time.Sleep(5 * time.Millisecond)
		}
		if err := validate(); err != nil {
			t.Fatal("prewithdraw current pin", err)
		}
		if err := s.DeleteNode(firstID); err != nil {
			t.Fatal(err)
		}
		if _, err := current(); err == nil {
			t.Fatal("deleted viewer credential remained authorized")
		}
		s.mu.Lock()
		record := s.registrarNodeLocked(nativeID)
		s.mu.Unlock()
		if record == nil {
			t.Fatal("native record absent during withdrawal")
		}
		nativeRef := registrarCredentialReference{NodeID: record.NodeID, NetworkID: record.NetworkID, DevicePublicKey: record.DevicePublicKey, Scope: record.Scope, ReceiptID: record.ReceiptID, ReceiptDigest: record.ReceiptDigest, DescriptorDigest: record.DescriptorDigest, TranscriptDigest: record.Consumption.TranscriptDigest}
		m, err := s.registrarCredentialMap(nativeRef, nativeSecret())
		if err != nil || len(m.Peers) != 1 || !m.Self.Approved || m.Self.KeyExpiry == nil || !time.Now().Before(*m.Self.KeyExpiry) {
			t.Fatal("actual native current map after viewer removal", err)
		}
		for _, p := range m.Peers {
			if p.ID == firstID || p.PubKey == pin.Viewer.PublicKey || p.IPv4 == pin.Viewer.IPv4 {
				t.Fatal("native map retained withdrawn viewer")
			}
		}
		// The revoked viewer retires its own dial; a removed WireGuard key cannot promise delivery of a remote FIN.
		closeCtx, stopClose := context.WithTimeout(context.Background(), time.Second)
		localClose, err = closeAttendedOwnedConn(closeCtx, conn)
		stopClose()
		if err != nil {
			t.Fatal("local withdrawal connection close did not join", err)
		}
		marker, _ := json.Marshal(map[string]any{"version": 1, "fake_ui": true, "withdrawn": true})
		if err := writeRegistrarFixtureAtomic(dir, "peer-attended-withdrawn.json", marker); err != nil {
			t.Fatal(err)
		}
	}
	var resultDial dialResult
	select {
	case resultDial = <-dialDone:
	case <-time.After(4 * time.Second):
		conn.Close()
		<-dialDone
		t.Fatal("attended dial did not join within bound")
	}
	if resultDial.admitted || resultDial.err == nil || resultDial.verified {
		t.Fatalf("denial-only case admitted or verified accepted reply: %+v", resultDial)
	}
	transportClass := ""
	if strings.HasPrefix(resultDial.err.Error(), "session: refused:") {
		transportClass = "explicit-protocol-refusal"
	} else if withdraw && errors.Is(resultDial.err, net.ErrClosed) {
		transportClass = "closed-read-after-verified-withdrawal"
	} else if withdraw && (errors.Is(resultDial.err, io.EOF) || errors.Is(resultDial.err, io.ErrUnexpectedEOF) || errors.Is(resultDial.err, net.ErrClosed)) {
		transportClass = "eof-after-verified-withdrawal"
	} else {
		t.Fatalf("unaccepted transport result for %s: %v", caseName, resultDial.err)
	}
	if !withdraw {
		if err := validate(); err != nil {
			t.Fatal(err)
		}
	}
	conn.Close()
	if !withdraw {
		refused, _ := json.Marshal(map[string]any{"version": 1, "fake_ui": true, "protocolRefused": true, "actualFarIP": far.IP.String(), "acceptedReplyVerifierCalled": false, "joined": true})
		if err := writeRegistrarFixtureAtomic(dir, "peer-attended-refused.json", refused); err != nil {
			t.Fatal(err)
		}
	} else {
		transport, _ := json.Marshal(map[string]any{"version": 1, "fake_ui": true, "case": caseName, "localClose": localClose, "transportClass": transportClass, "protocolRefused": transportClass == "explicit-protocol-refusal", "acceptedReplyVerifierCalled": false, "joined": true})
		if err := writeRegistrarFixtureAtomic(dir, "peer-attended-transport.json", transport); err != nil {
			t.Fatal(err)
		}
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
	if json.Unmarshal(raw, &outcome) != nil || outcome.Version != 1 || !outcome.ChallengeVerified || !outcome.FakeUI || !outcome.ListenerClosed || !outcome.ConnClosed || outcome.MuxClosed != nil || !outcome.LateOwnDenied || outcome.ScopeReason == "" || outcome.Case != caseName || outcome.AdmissionCalls != 1 || outcome.UIAsks != 1 || outcome.Approved != 0 || outcome.Muxes != 0 || outcome.CaptureCalls != 0 || outcome.InputCreates != 0 || !outcome.Joined {
		t.Fatalf("invalid actual host denial counters: %s", raw)
	}
	if withdraw && (outcome.PromptCancelled == nil || !*outcome.PromptCancelled || outcome.ScopeReason != "authorization-changed") {
		t.Fatal("withdrawal did not cancel actual pending prompt via map authority")
	}
	if !withdraw && outcome.PromptCancelled != nil {
		t.Fatal("deny unexpectedly claimed withdrawal")
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
	wantPeers := 2
	if withdraw {
		wantPeers = 1
	}
	if m, err := s.registrarCredentialMap(ref, nativeSecret()); err != nil || len(m.Peers) != wantPeers {
		t.Fatal("actual native current map absent", err)
	}
	ids := []string{firstID, secondID, nativeID}
	if withdraw {
		ids = []string{secondID, nativeID}
	}
	for _, id := range ids {
		if err := s.DeleteNode(id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.registrarCredentialMap(ref, nativeSecret()); err == nil {
		t.Fatal("deleted native credential still authorized")
	}
	finalFields := map[string]any{"version": 1, "fake_ui": true, "attendedDenied": true, "actualEnrolledPeers": true, "joined": true, "inputAuthorized": false}
	if withdraw {
		finalFields["case"] = caseName
		finalFields["promptCancelled"] = true
		finalFields["transportClass"] = transportClass
		finalFields["localClose"] = localClose
	}
	final, _ := json.Marshal(finalFields)
	if err := writeRegistrarFixtureAtomic(dir, "outcome.json", final); err != nil {
		t.Fatal(err)
	}
	t.Log("actual enrolled two-stack WireGuard/proofnet attended protocol refusal; exact current pins; fakeUI denial; no capture/input; owners joined")
}

// This evidence records the LOCAL call and its join, not which close won a read race.
type attendedLocalCloseEvidence struct {
	Invoked   bool  `json:"invoked"`
	Completed bool  `json:"completed"`
	Joined    bool  `json:"joined"`
	ElapsedMS int64 `json:"elapsedMS"`
}

func closeAttendedOwnedConn(ctx context.Context, c net.Conn) (attendedLocalCloseEvidence, error) {
	started := time.Now()
	e := attendedLocalCloseEvidence{Invoked: true}
	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case <-done:
		e.Completed = true
		e.Joined = true
		e.ElapsedMS = time.Since(started).Milliseconds()
		return e, nil
	case <-ctx.Done():
		return e, ctx.Err()
	}
}

// Independent local-ownership control: remote end remains open and never sends
// bytes or FIN. This is NOT an enrolled-WireGuard or remote-delivery test.
func TestAttendedLocalCloseJoinsWithoutRemoteClose(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	defer local.Close()
	entered := make(chan struct{})
	readDone := make(chan error, 1)
	go func() { close(entered); var b [1]byte; _, err := local.Read(b[:]); readDone <- err }()
	<-entered
	select {
	case <-readDone:
		t.Fatal("reader ended before localClose")
	case <-time.After(20 * time.Millisecond):
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e, err := closeAttendedOwnedConn(ctx, local)
	if err != nil || !e.Invoked || !e.Completed || !e.Joined || e.ElapsedMS >= 1000 {
		t.Fatalf("local close not observed/joined: %+v %v", e, err)
	}
	select {
	case err := <-readDone:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("unexpected local blocked-read result: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("owned blocked read did not join")
	}
	// Remote is still open here, no remote Close has happened.
}
