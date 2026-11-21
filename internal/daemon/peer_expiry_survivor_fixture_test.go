//go:build multies_native_fixture

package daemon

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/controlclient"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	source "fintex.au/mnet/internal/enrollmentsource"
	"fintex.au/mnet/internal/proofnet"
)

func peerExpiryWait(t *testing.T, dir, name string) {
	t.Helper()
	end := time.Now().Add(30 * time.Second)
	for {
		if _, e := os.Stat(filepath.Join(dir, name)); e == nil {
			return
		}
		if time.Now().After(end) {
			t.Fatal("fixture IPC timeout", name)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type expiryTCPReceiver struct {
	listener    net.Listener
	count       atomic.Int64
	mu          sync.Mutex
	connections []net.Conn
	wg          sync.WaitGroup
	closed      bool
}

func newExpiryTCPReceiver(t *testing.T, n *proofnet.Net, ip string) *expiryTCPReceiver {
	t.Helper()
	l, e := n.ListenTCPAddrPort(netip.AddrPortFrom(netip.MustParseAddr(ip), 49174))
	if e != nil {
		t.Fatal(e)
	}
	s := &expiryTCPReceiver{listener: l}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				c.Close()
				return
			}
			s.connections = append(s.connections, c)
			s.wg.Add(1)
			s.mu.Unlock()
			go func() {
				defer s.wg.Done()
				defer c.Close()
				for {
					var challenge [32]byte
					if _, e := io.ReadFull(c, challenge[:]); e != nil {
						return
					}
					s.count.Add(1)
					if _, e := c.Write(challenge[:]); e != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		s.mu.Lock()
		s.closed = true
		s.listener.Close()
		for _, c := range s.connections {
			c.Close()
		}
		s.mu.Unlock()
		done := make(chan struct{})
		go func() { s.wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("receiver cleanup did not join")
		}
	})
	return s
}
func expiryTCPChallenge(c net.Conn, deadline time.Time) error {
	if e := c.SetDeadline(deadline); e != nil {
		return e
	}
	var message, ack [32]byte
	if _, e := rand.Read(message[:]); e != nil {
		return e
	}
	if _, e := c.Write(message[:]); e != nil {
		return e
	}
	if _, e := io.ReadFull(c, ack[:]); e != nil {
		return e
	}
	if !bytes.Equal(message[:], ack[:]) {
		return fmt.Errorf("fresh challenge ack mismatch")
	}
	return nil
}

func TestPeerExpirySurvivorFixture(t *testing.T) {
	dir := os.Getenv("MNET_TEST_PEER_EXPIRY_DIR")
	if dir == "" {
		t.Skip("explicit disposable Multies coordinator required")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "client-config.json"))
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Policy              config.LocalConfigV2 `json:"policy"`
		ChallengeMS         int                  `json:"challenge_ms"`
		MaxLeaseMS          int                  `json:"max_lease_ms"`
		MaxObservationAgeMS int                  `json:"max_observation_age_ms"`
		ClockUncertaintyMS  int                  `json:"clock_uncertainty_ms"`
	}
	if json.Unmarshal(raw, &f) != nil {
		t.Fatal("config")
	}
	limits := registrar.WorkflowLimits{Timeout: 3 * time.Second, MaxChallengeLifetime: time.Duration(f.ChallengeMS) * time.Millisecond, MaxReserveLifetime: time.Minute, Timing: contract.FixtureTiming{MaxObservationAgeMS: int64(f.MaxObservationAgeMS), MaxLeaseMS: int64(f.MaxLeaseMS), ClockUncertaintyMS: int64(f.ClockUncertaintyMS)}}
	clients := controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 16}
	var ds [2]*Daemon
	var ns [2]*proofnet.Net
	var expiry [2]time.Time
	for i := 0; i < 2; i++ {
		if i == 1 {
			time.Sleep(2 * time.Second)
		} // Stagger genuine issuance, never change a deadline.
		d, e := New(Config{StateDir: filepath.Join(dir, fmt.Sprintf("native-%d", i))})
		if e != nil {
			t.Fatal(e)
		}
		ds[i] = d
		t.Cleanup(func() { d.stopDevice(); d.Close() })
		if _, e = d.PrepareIdentity(); e != nil {
			t.Fatal(e)
		}
		policy, _ := json.Marshal(f.Policy)
		if e = os.WriteFile(filepath.Join(d.cfg.StateDir, "enrollment-policy-v2.json"), policy, 0600); e != nil {
			t.Fatal(e)
		}
		launch, e := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
		if e != nil {
			t.Fatal(e)
		}
		public, _ := json.Marshal(launch)
		if e = writeFixtureAtomic(dir, fmt.Sprintf("client-%d-launch.json", i), public); e != nil {
			t.Fatal(e)
		}
		peerExpiryWait(t, dir, fmt.Sprintf("client-%d-continue", i))
		if _, e = d.exchangeNativeEnrollment(context.Background(), source.Limits{Timeout: 3 * time.Second, MaxConnections: 1}); e != nil {
			t.Fatal("source exchange", e)
		}
		result, e := d.proveNativeEnrollment(context.Background(), limits)
		if e != nil {
			t.Fatal("genuine own-key proof", e)
		}
		if e = d.finalizeNativeEnrollment(context.Background()); e != nil {
			t.Fatal("sole native finalization", e)
		}
		expiry[i] = time.UnixMilli(result.Receipt.AccessExpiresAtMS)
		m, e := d.nativeCurrentMap(context.Background(), clients)
		if e != nil {
			t.Fatal(e)
		}
		injected, n, e := proofnet.CreateNetTUN([]netip.Addr{netip.MustParseAddr(m.Self.IPv4)}, nil, 1280)
		if e != nil {
			t.Fatal(e)
		}
		ns[i] = n
		d.cfg.TUN = injected
		if e = d.startDevice(); e != nil {
			t.Fatal(e)
		}
		if e = d.nativeCurrentEndpoints(context.Background(), []string{fmt.Sprintf("127.0.0.1:%d", d.bind.Port())}, clients); e != nil {
			t.Fatal(e)
		}
	}
	a, b := ds[0], ds[1]
	if !expiry[1].After(expiry[0]) {
		t.Fatal("harness-inconclusive: sequential receipts lack survivor margin")
	}
	ma, e := a.nativeCurrentMap(context.Background(), clients)
	if e != nil {
		t.Fatal(e)
	}
	mb, e := b.nativeCurrentMap(context.Background(), clients)
	if e != nil {
		t.Fatal(e)
	}
	if len(ma.Peers) != 1 || len(mb.Peers) != 1 || mb.Peers[0].KeyExpiry == nil || !mb.Peers[0].KeyExpiry.Equal(expiry[0]) {
		t.Fatal("original authenticated peer scope")
	}
	a.applyMap(ma)
	b.applyMap(mb)
	b.mu.Lock()
	b.status = "up"
	b.mu.Unlock()
	// A intentionally has no expiryLoop: bare own-key probe remains physically
	// alive, never evidence of A enforcement or authorization after its deadline.
	b.mapChanged = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); b.expiryLoop(ctx) }()
	defer func() { cancel(); <-done }()
	ra := newExpiryTCPReceiver(t, ns[0], ma.Self.IPv4)
	rb := newExpiryTCPReceiver(t, ns[1], mb.Self.IPv4)
	dial := func(n *proofnet.Net, ip string) net.Conn {
		ctx, cancel := context.WithDeadline(context.Background(), expiry[0].Add(-time.Second))
		defer cancel()
		c, e := n.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(netip.MustParseAddr(ip), 49174))
		if e != nil {
			t.Fatal("harness-inconclusive: positive dial before original A expiry", e)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	ab := dial(ns[0], mb.Self.IPv4)
	ba := dial(ns[1], ma.Self.IPv4)
	for _, c := range []net.Conn{ab, ba} {
		if e = expiryTCPChallenge(c, expiry[0].Add(-500*time.Millisecond)); e != nil {
			t.Fatal("harness-inconclusive: fresh pre-expiry ack", e)
		}
	}
	if ra.count.Load() != 1 || rb.count.Load() != 1 {
		t.Fatal("positive receiver counts")
	}
	if time.Until(expiry[0]) < 500*time.Millisecond || expiry[1].Sub(expiry[0]) < 1200*time.Millisecond {
		t.Fatal("harness-inconclusive: original survivor margin too small")
	}
	if e = writeFixtureAtomic(dir, "positive-traffic", []byte("two genuine receiver-acked TCP challenges")); e != nil {
		t.Fatal(e)
	}
	peerExpiryWait(t, dir, "maps-blocked")
	// Real registrar process is SIGSTOPped by coordinator. A request must return
	// no map while B's expiryLoop/device continue independent of control service.
	mapCtx, mapCancel := context.WithDeadline(context.Background(), expiry[1])
	defer mapCancel()
	mapDone := make(chan error, 1)
	go func() {
		m, e := b.nativeCurrentMap(mapCtx, clients)
		if e == nil || m != nil {
			mapDone <- fmt.Errorf("map unexpectedly returned")
			return
		}
		mapDone <- nil
	}()
	time.Sleep(time.Until(expiry[0]))
	end := expiry[1].Add(-700 * time.Millisecond)
	key := mb.Peers[0].PubKey
	for len(allowedIPsOf(t, b, key)) != 0 && time.Now().Before(end) {
		time.Sleep(time.Millisecond)
	}
	if len(allowedIPsOf(t, b, key)) != 0 {
		t.Fatal("B retained expired A native authorization")
	}
	var native bytes.Buffer
	if e = b.dev.IpcGetOperation(&native); e != nil {
		t.Fatal(e)
	}
	pub, e := base64.StdEncoding.Strict().DecodeString(key)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(native.Bytes(), []byte("public_key="+hex.EncodeToString(pub))) {
		t.Fatal("B retained expired A native peer key")
	}
	clear(pub)
	if time.Now().After(end) {
		t.Fatal("harness-inconclusive: no negative-probe margin")
	}
	beforeA, beforeB := ra.count.Load(), rb.count.Load()
	for _, c := range []net.Conn{ba, ab} {
		deadline := time.Now().Add(150 * time.Millisecond)
		if deadline.After(end) {
			t.Fatal("harness-inconclusive: negative deadline outside survivor window")
		}
		if expiryTCPChallenge(c, deadline) == nil {
			t.Fatal("postexpiry fresh challenge acknowledged")
		}
	}
	if ra.count.Load() != beforeA || rb.count.Load() != beforeB {
		t.Fatal("postexpiry application acceptance advanced")
	}
	b.refreshPaths()
	b.applyMap(mb)
	if len(allowedIPsOf(t, b, key)) != 0 {
		t.Fatal("stale map reinstalled A")
	}
	b.deviceMu.RLock()
	alive := b.dev != nil
	b.deviceMu.RUnlock()
	a.deviceMu.RLock()
	aProbeAlive := a.dev != nil
	a.deviceMu.RUnlock()
	if !aProbeAlive || len(allowedIPsOf(t, a, ma.Peers[0].PubKey)) == 0 {
		t.Fatal("bare A probe was shut down, invalidating negative traffic evidence")
	}
	if !alive || b.Status().Status != "up" || !time.Now().Before(expiry[1]) {
		t.Fatal("B not alive under own original authority")
	}
	if e = <-mapDone; e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Until(expiry[1]))
	for b.Status().Status != "key-expired" && time.Now().Before(expiry[1].Add(time.Second)) {
		time.Sleep(time.Millisecond)
	}
	if b.Status().Status != "key-expired" {
		t.Fatal("B original self expiry not enforced")
	}
	if ra.count.Load() != beforeA || rb.count.Load() != beforeB {
		t.Fatal("late postexpiry application acceptance advanced")
	}
	t.Logf("original A=%s B=%s receiverBeforeA=%d receiverBeforeB=%d receiverFinalA=%d receiverFinalB=%d", expiry[0].UTC().Format(time.RFC3339Nano), expiry[1].UTC().Format(time.RFC3339Nano), beforeA, beforeB, ra.count.Load(), rb.count.Load())
	out, _ := json.Marshal(map[string]any{"bEnforcedA": true, "bAliveAfterA": true, "staleRefreshRefused": true, "bOriginalExpiry": true, "receiverA": ra.count.Load(), "receiverB": rb.count.Load(), "survivorMarginMS": expiry[1].Sub(expiry[0]).Milliseconds(), "twoEndpointEnforcement": false})
	if e = writeFixtureAtomic(dir, "peer-expiry-result.json", out); e != nil {
		t.Fatal(e)
	}
	t.Log(string(out))
}
