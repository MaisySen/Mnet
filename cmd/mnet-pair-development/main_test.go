//go:build mnet_enrollment_development

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
)

func fixtureConfig() config {
	return config{Version: 1, Role: "dial", SelfKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)), PeerKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)), SelfIP: "100.64.0.1", PeerIP: "100.64.0.2", WorkStartMS: time.Now().UnixMilli(), EarlierExpiryMS: time.Now().Add(10 * time.Second).UnixMilli(), BodyHex: hex.EncodeToString(bytes.Repeat([]byte{3}, 32))}
}
func TestStrictPairConfig(t *testing.T) {
	c := fixtureConfig()
	raw, _ := json.Marshal(c)
	if _, e := parseConfig(raw); e != nil {
		t.Fatal(e)
	}
	for name, raw := range map[string][]byte{"duplicate": append([]byte(`{"version":1,`), raw[1:]...), "aliased": []byte(strings.Replace(string(raw), `"version"`, `"Version"`, 1)), "trailing": append(raw, []byte(`{}`)...), "oversized": bytes.Repeat([]byte{' '}, 2049), "null": []byte(`null`)} {
		t.Run(name, func(t *testing.T) {
			if _, e := parseConfig(raw); e == nil {
				t.Fatal("accepted malformed config")
			}
		})
	}
	for name, mutate := range map[string]func(*config){"wrong-role": func(c *config) { c.Role = "legacy" }, "same-key": func(c *config) { c.PeerKey = c.SelfKey }, "same-ip": func(c *config) { c.PeerIP = c.SelfIP }, "loopback": func(c *config) { c.PeerIP = "127.0.0.1" }, "not-pool": func(c *config) { c.PeerIP = "10.0.0.1" }, "key-padding": func(c *config) { c.PeerKey = strings.TrimRight(c.PeerKey, "=") }, "window-widened": func(c *config) { c.EarlierExpiryMS = c.WorkStartMS }, "past-work": func(c *config) { c.WorkStartMS = 0 }, "body-cap": func(c *config) { c.BodyHex = "ab" }, "extra-version": func(c *config) { c.Version = 2 }} {
		t.Run(name, func(t *testing.T) {
			bad := c
			mutate(&bad)
			raw, _ := json.Marshal(bad)
			if _, e := parseConfig(raw); e == nil {
				t.Fatal("accepted bad binding")
			}
		})
	}
}
func TestExactPeerRefusesWrongReplacedAmbiguous(t *testing.T) {
	c := fixtureConfig()
	p := daemon.DevelopmentNetworkPeer{IPv4: c.PeerIP, PublicKey: c.PeerKey, Name: "peer", HasEndpoints: true}
	if _, e := exactPeer([]daemon.DevelopmentNetworkPeer{p}, c); e != nil {
		t.Fatal(e)
	}
	for name, peers := range map[string][]daemon.DevelopmentNetworkPeer{"absent": nil, "duplicate": {p, p}, "replaced-key": {p, {IPv4: p.IPv4, PublicKey: c.SelfKey, Name: p.Name}}, "replaced-ip": {p, {IPv4: c.SelfIP, PublicKey: p.PublicKey, Name: p.Name}}, "unnamed": {{IPv4: p.IPv4, PublicKey: p.PublicKey}}} {
		t.Run(name, func(t *testing.T) {
			if _, e := exactPeer(peers, c); e == nil {
				t.Fatal("accepted wrong binding")
			}
		})
	}
}
func TestPhaseNeverWidensOriginalWindow(t *testing.T) {
	c := fixtureConfig()
	c.EarlierExpiryMS = c.WorkStartMS + 1500
	ctx, cancel, e := phase(context.Background(), c, 3*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer cancel()
	d, _ := ctx.Deadline()
	if d.After(time.UnixMilli(c.EarlierExpiryMS).Add(-time.Second)) {
		t.Fatal("deadline widened")
	}
	c.WorkStartMS = time.Now().Add(-10 * time.Second).UnixMilli()
	if _, _, e = phase(context.Background(), c, time.Second); e == nil {
		t.Fatal("late work accepted")
	}
}
func TestFreshChallengeThenFixedHTTP(t *testing.T) {
	c := fixtureConfig()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	done := make(chan error, 1)
	go func() {
		if e := answer(b, time.Now().Add(time.Second)); e != nil {
			done <- e
			return
		}
		done <- serveHTTP(b, c)
	}()
	if e := challenge(a, time.Now().Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	if e := boundedHTTP(a, c); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestChallengeRejectsReplayedAck(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go func() {
		var challenge [32]byte
		io.ReadFull(b, challenge[:])
		old := sha256.Sum256(bytes.Repeat([]byte{42}, 32))
		b.Write(old[:])
	}()
	if e := challenge(a, time.Now().Add(time.Second)); e == nil {
		t.Fatal("accepted stale ack")
	}
}
func TestHTTPClientRejectsRedirectWrongBodyAndOversizeHeader(t *testing.T) {
	c := fixtureConfig()
	for name, response := range map[string]string{"redirect": "HTTP/1.1 302 Found\r\nLocation: http://other.invalid/\r\nContent-Length: 0\r\n\r\n", "wrong-body": "HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\nx", "large-header": "HTTP/1.1 200 OK\r\nX-Extra: " + strings.Repeat("x", 5000) + "\r\nContent-Length: 0\r\n\r\n"} {
		t.Run(name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			done := make(chan struct{})
			go func() { defer close(done); buf := make([]byte, 4096); b.Read(buf); b.Write([]byte(response)) }()
			if e := boundedHTTP(a, c); e == nil {
				t.Fatal("accepted invalid HTTP")
			}
			a.Close()
			<-done
		})
	}
}
func TestHTTPServerRejectsUnexpectedAndCappedRequests(t *testing.T) {
	c := fixtureConfig()
	for name, request := range map[string]string{"wrong-host": "GET /owned-document HTTP/1.1\r\nHost: other.invalid\r\nConnection: close\r\n\r\n", "wrong-path": "GET /else HTTP/1.1\r\nHost: mnet-disposable.invalid\r\nConnection: close\r\n\r\n", "keepalive": "GET /owned-document HTTP/1.1\r\nHost: mnet-disposable.invalid\r\n\r\n", "large-header": "GET /owned-document HTTP/1.1\r\nHost: mnet-disposable.invalid\r\nX-Extra: " + strings.Repeat("x", 4096) + "\r\n\r\n", "body": "POST /owned-document HTTP/1.1\r\nHost: mnet-disposable.invalid\r\nConnection: close\r\nContent-Length: 1\r\n\r\nx"} {
		t.Run(name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			done := make(chan error, 1)
			go func() { b.SetDeadline(time.Now().Add(time.Second)); done <- serveHTTP(b, c) }()
			go a.Write([]byte(request))
			if e := <-done; e == nil {
				t.Fatal("accepted invalid request")
			}
			a.Close()
		})
	}
}

type countedClosed struct {
	net.Conn
	writes int
}

func (c *countedClosed) Write(p []byte) (int, error) { c.writes++; return c.Conn.Write(p) }
func TestPostExpiryStillAttemptsOldStreamWrite(t *testing.T) {
	a, b := net.Pipe()
	a.Close()
	b.Close()
	c := &countedClosed{Conn: a}
	if e := postExpiryChallenge(c); e == nil {
		t.Fatal("closed stream acknowledged")
	}
	if c.writes != 1 {
		t.Fatal("post expiry only checked deadline, no Write")
	}
}
func TestOutcomeClassificationDoesNotHideSafetyFailure(t *testing.T) {
	for _, e := range []error{errWindow, context.Canceled, context.DeadlineExceeded} {
		if !inconclusive(e) {
			t.Fatal("window/session end not inconclusive")
		}
	}
	for _, e := range []error{io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe} {
		if inconclusive(e) {
			t.Fatal("non timing error hidden as inconclusive")
		}
	}
}
func testEmitter() (*emitter, *bytes.Buffer) {
	b := &bytes.Buffer{}
	return &emitter{enc: json.NewEncoder(b)}, b
}
func TestNaturalRetirementDoesNotForceCloseToPass(t *testing.T) {
	out, _ := testEmitter()
	scope := make(chan struct{})
	joined := false
	e := observeNaturalRetirement(nil, scope, time.Now().Add(-time.Second), time.Now().Add(-time.Second), out, func() { joined = true; close(scope) })
	if e == nil || inconclusive(e) {
		t.Fatal("missing natural retirement hidden")
	}
	if joined {
		t.Fatal("forced Close manufactured retirement")
	}
}
func TestNaturalRetirementUsesPeerExpiryNotEarlierExpiry(t *testing.T) {
	out, _ := testEmitter()
	earlier := time.Now().Add(-time.Second)
	peer := time.Now().Add(-400 * time.Millisecond)
	scope := make(chan struct{})
	go func() { time.Sleep(10 * time.Millisecond); close(scope) }()
	joined := false
	if e := observeNaturalRetirement(nil, scope, peer, peer, out, func() { joined = true }); e != nil || !joined {
		t.Fatalf("actual later peer deadline failed: %v", e)
	}
	scope2 := make(chan struct{})
	if e := observeNaturalRetirement(nil, scope2, earlier, earlier, out, func() { t.Fatal("join before natural") }); e == nil {
		t.Fatal("swapped earlier expiry passed")
	}
}
func TestOuterContextCannotStandInForScopeRetirement(t *testing.T) {
	out, log := testEmitter()
	outer := make(chan struct{})
	close(outer)
	scope := make(chan struct{})
	if e := observeNaturalRetirement(outer, scope, time.Now().Add(-time.Second), time.Now().Add(-time.Second), out, func() { t.Fatal("cleanup before natural") }); e == nil || inconclusive(e) {
		t.Fatal("outer cancellation hid missing scope retirement")
	}
	close(scope)
	if e := observeNaturalRetirement(outer, scope, time.Now(), time.Now().Add(-time.Second), out, func() {}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(log.String(), `"natural_before_cleanup":true`) || !strings.Contains(log.String(), `"outer_context_ended":true`) {
		t.Fatal("signal provenance missing")
	}
}
func TestExtraAcceptClassificationBoundaries(t *testing.T) {
	e := time.Now()
	for _, at := range []time.Time{e, e.Add(50 * time.Millisecond), e.Add(500 * time.Millisecond)} {
		if !inconclusive(classifyExtraAccept(at, e)) {
			t.Fatal("declared lag marked safety failure")
		}
	}
	for _, at := range []time.Time{e.Add(-time.Nanosecond), e.Add(500*time.Millisecond + time.Nanosecond)} {
		if inconclusive(classifyExtraAccept(at, e)) {
			t.Fatal("unexpected accept hidden as timing")
		}
	}
}
func TestRetirementPinIsPeerNotSelfOrEarlier(t *testing.T) {
	a := time.UnixMilli(1000)
	b := time.UnixMilli(1800)
	if !peerRetirementExpiry(daemon.DevelopmentPairSnapshot{SelfExpiry: a, PeerExpiry: b}).Equal(b) {
		t.Fatal("A retirement incorrectly pinned to self/earlier expiry")
	}
	if !peerRetirementExpiry(daemon.DevelopmentPairSnapshot{SelfExpiry: b, PeerExpiry: a}).Equal(a) {
		t.Fatal("B retirement incorrectly pinned to self/later expiry")
	}
}
func TestEarlyRetirementUsesEffectiveExpiryAndCannotBeHiddenByJoin(t *testing.T) {
	out, _ := testEmitter()
	done := make(chan struct{})
	close(done)
	now := time.Now()
	if e := observeNaturalRetirement(nil, done, now.Add(time.Second), now.Add(-time.Second), out, func() {}); e != nil {
		t.Fatalf("expected inherited self expiry rejected: %v", e)
	}
	effective := time.Now().Add(20 * time.Millisecond)
	e := observeNaturalRetirement(nil, done, effective, effective, out, func() { time.Sleep(25 * time.Millisecond) })
	if !inconclusive(e) {
		t.Fatal("early retirement hidden by cleanup latency or marked safety failure")
	}
}
func TestPrecommitWorkOriginDoesNotPretendToBeReceiptDuration(t *testing.T) {
	c := fixtureConfig()
	c.WorkStartMS = 1795283313943
	c.EarlierExpiryMS = 1795283324095
	raw, _ := json.Marshal(c)
	if _, e := parseConfig(raw); e != nil {
		t.Fatal("precommit work origin rejected as lease duration", e)
	}
	if _, _, e := phase(context.Background(), c, time.Second); e == nil {
		t.Fatal("historical window revived by DATA parse")
	}
}
