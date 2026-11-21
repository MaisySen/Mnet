//go:build mnet_enrollment_development

// Explicit disposable two-installed-owner feasibility adapter. Never normal Up.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/proofnet"
	"golang.zx2c4.com/wireguard/tun"
)

const pairPort = 49175
const fixedHost = "mnet-disposable.invalid"
const fixedPath = "/owned-document"

var errWindow = errors.New("harness-inconclusive: original window missed")

type config struct {
	Version         int    `json:"version"`
	Role            string `json:"role"`
	SelfKey         string `json:"self_key"`
	PeerKey         string `json:"peer_key"`
	SelfIP          string `json:"self_ip"`
	PeerIP          string `json:"peer_ip"`
	EarlierExpiryMS int64  `json:"earlier_expiry_ms"`
	WorkStartMS     int64  `json:"work_start_ms"`
	BodyHex         string `json:"body_hex"`
}

func parseConfig(raw []byte) (config, error) {
	var c config
	if len(raw) > 2048 {
		return c, errors.New("oversize pair config")
	}
	// Exact key set, reject duplicate/aliased/unknown fields before struct decode.
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return c, errors.New("pair config object required")
	}
	want := map[string]bool{"version": true, "role": true, "self_key": true, "peer_key": true, "self_ip": true, "peer_ip": true, "earlier_expiry_ms": true, "work_start_ms": true, "body_hex": true}
	seen := map[string]bool{}
	for d.More() {
		t, e := d.Token()
		k, ok := t.(string)
		if e != nil || !ok || !want[k] || seen[k] {
			return c, errors.New("invalid pair config key")
		}
		seen[k] = true
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return c, errors.New("invalid value")
		}
	}
	if _, e = d.Token(); e != nil || len(seen) != len(want) {
		return c, errors.New("incomplete pair config")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("trailing pair config")
	}
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || (c.Role != "listen" && c.Role != "dial") {
		return c, errors.New("invalid pair config")
	}
	for _, k := range []string{c.SelfKey, c.PeerKey} {
		b, e := base64.StdEncoding.Strict().DecodeString(k)
		if e != nil || len(b) != 32 || base64.StdEncoding.EncodeToString(b) != k {
			return c, errors.New("invalid public key")
		}
	}
	for _, ip := range []string{c.SelfIP, c.PeerIP} {
		a, e := netip.ParseAddr(ip)
		if e != nil || !a.Is4() || a.String() != ip || !netip.MustParsePrefix("100.64.0.0/10").Contains(a) {
			return c, errors.New("invalid tunnel IPv4")
		}
	}
	body, e := hex.DecodeString(c.BodyHex)
	if e != nil || len(body) != 32 || hex.EncodeToString(body) != c.BodyHex || c.SelfKey == c.PeerKey || c.SelfIP == c.PeerIP || c.WorkStartMS <= 0 || c.EarlierExpiryMS <= c.WorkStartMS || c.EarlierExpiryMS-c.WorkStartMS > 10000 {
		return c, errors.New("invalid finite pair binding")
	}
	return c, nil
}

type emitter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (e *emitter) stage(name string, fields map[string]any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if fields == nil {
		fields = map[string]any{}
	}
	fields["stage"] = name
	fields["at_ms"] = time.Now().UnixMilli()
	return e.enc.Encode(fields)
}
func main() {
	root := flag.String("disposable-root", "", "existing saved owned disposable root")
	rawConfig := flag.String("public-config", "", "strict public pair config file")
	flag.Parse()
	if *root == "" || *rawConfig == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "explicit saved disposable root and public config required")
		os.Exit(2)
	}
	f, e := os.Open(*rawConfig)
	if e != nil {
		fmt.Fprintln(os.Stderr, "pair config unavailable")
		os.Exit(2)
	}
	raw, e := io.ReadAll(io.LimitReader(f, 2049))
	f.Close()
	if e != nil {
		fmt.Fprintln(os.Stderr, "pair config unavailable")
		os.Exit(2)
	}
	c, e := parseConfig(raw)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	out := &emitter{enc: json.NewEncoder(os.Stdout)}
	if e = runPair(ctx, *root, c, out, os.Stdin); e != nil {
		out.stage("pair-failed", map[string]any{"error": e.Error(), "partial_slices_remain_spent": true})
		os.Exit(1)
	}
}
func phase(parent context.Context, c config, limit time.Duration) (context.Context, context.CancelFunc, error) {
	deadline := time.UnixMilli(c.EarlierExpiryMS).Add(-time.Second)
	work := time.UnixMilli(c.WorkStartMS).Add(9 * time.Second)
	if work.Before(deadline) {
		deadline = work
	}
	if t := time.Now().Add(limit); t.Before(deadline) {
		deadline = t
	}
	if !time.Now().Before(deadline) {
		return nil, nil, errWindow
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	return ctx, cancel, nil
}
func exactPeer(peers []daemon.DevelopmentNetworkPeer, c config) (daemon.SessionPeer, error) {
	var selected daemon.SessionPeer
	matches := 0
	for _, p := range peers {
		if p.PublicKey == c.PeerKey && p.IPv4 == c.PeerIP {
			matches++
			selected = daemon.SessionPeer{Name: p.Name, IPv4: p.IPv4, PublicKey: p.PublicKey}
		} else if p.PublicKey == c.PeerKey || p.IPv4 == c.PeerIP {
			return selected, errors.New("replaced peer key/IP")
		}
	}
	if matches != 1 || selected.Name == "" {
		return selected, errors.New("missing or ambiguous exact peer")
	}
	return selected, nil
}
func beforePayload(s *daemon.DevelopmentNetworkSession, pin daemon.SessionPeer) error {
	peers, e := s.CurrentSessionPeers()
	if e != nil {
		return e
	}
	matches := 0
	for _, p := range peers {
		if p == pin {
			matches++
		} else if p.PublicKey == pin.PublicKey || p.IPv4 == pin.IPv4 {
			return errors.New("peer changed")
		}
	}
	if matches != 1 {
		return errors.New("peer no longer current")
	}
	return nil
}
func challenge(conn net.Conn, deadline time.Time) error {
	if e := conn.SetDeadline(deadline); e != nil {
		return e
	}
	var b, ack [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		return e
	}
	if _, e := conn.Write(b[:]); e != nil {
		return e
	}
	if _, e := io.ReadFull(conn, ack[:]); e != nil {
		return e
	}
	sum := sha256.Sum256(b[:])
	if ack != sum {
		return errors.New("fresh proof mismatch")
	}
	return nil
}
func answer(conn net.Conn, deadline time.Time) error {
	if e := conn.SetDeadline(deadline); e != nil {
		return e
	}
	var b [32]byte
	if _, e := io.ReadFull(conn, b[:]); e != nil {
		return e
	}
	sum := sha256.Sum256(b[:])
	if !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	_, e := conn.Write(sum[:])
	return e
}
func boundedHTTP(conn net.Conn, c config) error {
	used := false
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 4096, ResponseHeaderTimeout: time.Second, DialContext: func(context.Context, string, string) (net.Conn, error) {
		if used {
			return nil, errors.New("second dial forbidden")
		}
		used = true
		return conn, nil
	}}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect forbidden") }}
	req, e := http.NewRequest("GET", "http://"+fixedHost+fixedPath, nil)
	if e != nil {
		return e
	}
	req.Host = fixedHost
	req.Close = true
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("unexpected HTTP status")
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 257))
	if e != nil || string(b) != c.BodyHex {
		return errors.New("HTTP body mismatch or cap")
	}
	return nil
}
func serveHTTP(conn net.Conn, c config) error {
	header := make([]byte, 0, 4096)
	one := make([]byte, 1)
	for !bytes.HasSuffix(header, []byte("\r\n\r\n")) {
		if len(header) >= 4096 {
			return errors.New("request headers exceed cap")
		}
		if _, e := io.ReadFull(conn, one); e != nil {
			return e
		}
		header = append(header, one[0])
	}
	r := bufio.NewReader(bytes.NewReader(header))
	req, e := http.ReadRequest(r)
	if e != nil {
		return e
	}
	defer req.Body.Close()
	if req.Method != "GET" || req.Host != fixedHost || req.RequestURI != fixedPath || req.ContentLength != 0 || len(req.TransferEncoding) != 0 || !req.Close {
		return errors.New("HTTP request refused")
	}
	_, e = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\nContent-Type: text/plain\r\n\r\n%s", len(c.BodyHex), c.BodyHex)
	return e
}
func runPair(ctx context.Context, root string, c config, out *emitter, ready io.ReadCloser) error {
	if _, cancel, e := phase(ctx, c, time.Second); e != nil {
		return e
	} else {
		cancel()
	}
	if time.Now().After(time.UnixMilli(c.WorkStartMS).Add(time.Second)) {
		return errWindow
	}
	out.stage("pair-connect-start", nil)
	var network *proofnet.Net
	var selfIP string
	finished := false
	options := daemon.DevelopmentNetworkOptions{Limits: controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 14}, Device: func(ip string) (tun.Device, error) {
		selfIP = ip
		if ip != c.SelfIP {
			return nil, errors.New("self IP mismatch")
		}
		a, e := netip.ParseAddr(ip)
		if e != nil {
			return nil, e
		}
		t, n, e := proofnet.CreateNetTUN([]netip.Addr{a}, nil, 1280)
		network = n
		return t, e
	}, Run: func(ctx context.Context, s *daemon.DevelopmentNetworkSession) error {
		if selfIP != c.SelfIP {
			return errors.New("self binding mismatch")
		}
		pin, e := exactPeer(s.Peers(), c)
		if e != nil {
			return e
		}
		scope, e := s.BindPeer(pin)
		if e != nil {
			return e
		}
		defer scope.Close()
		snapshot, e := s.DevelopmentPairSnapshot(pin)
		if e != nil {
			return e
		}
		earlier := snapshot.SelfExpiry
		if snapshot.PeerExpiry.Before(earlier) {
			earlier = snapshot.PeerExpiry
		}
		if snapshot.SelfPublicKey != c.SelfKey || snapshot.SelfIPv4 != c.SelfIP || earlier.UnixMilli() != c.EarlierExpiryMS {
			return errors.New("original receipt binding mismatch")
		}
		if time.Now().After(time.UnixMilli(c.WorkStartMS).Add(3 * time.Second)) {
			return errWindow
		}
		out.stage("pair-bound-current", map[string]any{"self_ip": c.SelfIP, "peer_ip": pin.IPv4, "earlier_expiry_ms": earlier.UnixMilli()})
		if _, cancel, e := phase(ctx, c, 2*time.Second); e != nil {
			return e
		} else {
			cancel()
		}
		if c.Role == "listen" {
			e = runListener(ctx, scope, s, network, c, pin, out)
		} else {
			e = runDialer(ctx, scope, s, network, c, out, ready)
		}
		if e == nil {
			finished = true
		}
		return e
	}}
	e := daemon.ConnectDisposableEnrollment(ctx, root, func(event daemon.DevelopmentEnrollmentEvent) error { return out.stage(event.Stage, nil) }, options)
	if !finished {
		if e == nil {
			return errors.New("adapter outcome missing")
		}
		return e
	}
	if e != nil && !errors.Is(e, context.DeadlineExceeded) && !errors.Is(e, context.Canceled) {
		return e
	}
	return out.stage("pair-finished", map[string]any{"owners_joined": true, "slice_refunded": false})
}
func runListener(ctx context.Context, scope *daemon.DevelopmentPeerScope, s *daemon.DevelopmentNetworkSession, n *proofnet.Net, c config, pin daemon.SessionPeer, out *emitter) error {
	l, e := n.ListenTCPAddrPort(netip.AddrPortFrom(netip.MustParseAddr(c.SelfIP), pairPort))
	if e != nil {
		return e
	}
	if e = scope.Own(l); e != nil {
		return e
	}
	// Never emit readiness until exact current peer, endpoints and listener exist.
	if time.Now().After(time.UnixMilli(c.WorkStartMS).Add(3 * time.Second)) {
		return errWindow
	}
	if e = out.stage("pair-listener-ready", map[string]any{"ip": c.SelfIP, "port": pairPort}); e != nil {
		return e
	}
	type result struct {
		proofs, http int
		err          error
	}
	done := make(chan result, 1)
	joined := make(chan struct{})
	var oldDone chan struct{}
	var oldProofs atomic.Int32
	defer func() {
		scope.Close()
		<-joined
		if oldDone != nil {
			<-oldDone
		}
	}()
	go func() {
		defer close(joined)
		r := result{}
		defer func() { done <- r }()
		for i := 0; i < 2; i++ {
			conn, e := l.Accept()
			if e != nil {
				r.err = e
				return
			}
			if e = scope.Own(conn); e != nil {
				r.err = e
				return
			}
			remote, ok := conn.RemoteAddr().(*net.TCPAddr)
			if !ok || !remote.IP.Equal(net.ParseIP(c.PeerIP)) {
				conn.Close()
				r.err = errors.New("unapproved remote IP")
				return
			}
			if e = beforePayload(s, pin); e != nil {
				r.err = e
				return
			}
			deadline := time.UnixMilli(c.EarlierExpiryMS).Add(-time.Second)
			if w := time.UnixMilli(c.WorkStartMS).Add(9 * time.Second); w.Before(deadline) {
				deadline = w
			}
			if time.Now().Add(time.Second).Before(deadline) {
				deadline = time.Now().Add(time.Second)
			}
			if e = conn.SetDeadline(deadline); e != nil {
				r.err = e
				return
			}
			var mode [1]byte
			if _, e = io.ReadFull(conn, mode[:]); e != nil {
				r.err = e
				return
			}
			if (i == 0 && mode[0] != 'P') || (i == 1 && mode[0] != 'H') {
				r.err = errors.New("mode order refused")
				return
			}
			if e = answer(conn, deadline); e != nil {
				r.err = e
				return
			}
			r.proofs++
			out.stage("pair-server-proof", map[string]any{"count": r.proofs})
			if mode[0] == 'H' {
				if e = serveHTTP(conn, c); e != nil {
					r.err = e
					return
				}
				r.http++
				conn.Close()
				out.stage("pair-http-served", nil)
			} else if mode[0] == 'P' {
				// Keep this exact old stream owned/open until peer expiry; no buffered app reply.
				oldDone = make(chan struct{})
				go func() {
					defer close(oldDone)
					for {
						if e := answer(conn, time.UnixMilli(c.EarlierExpiryMS)); e != nil {
							return
						}
						count := oldProofs.Add(1)
						out.stage("pair-old-stream-live-proof", map[string]any{"count": count})
					}
				}()
			} else {
				r.err = errors.New("mode refused")
				return
			}
		}
		extra, err := l.Accept()
		if err == nil {
			extra.Close()
			r.err = errors.New("unexpected extra connection")
			return
		}
		if scope.Context().Err() == nil {
			r.err = errors.New("listener stopped before scope")
			return
		}
		out.stage("pair-accept-loop-joined", nil)
	}()
	select {
	case <-ctx.Done():
	case <-scope.Context().Done():
	}
	if time.Now().UnixMilli() < c.EarlierExpiryMS {
		return errors.New("session stopped before expiry oracle")
	}
	// Scope closes listener and both accepted connections; join accept/application owner.
	scope.Close()
	select {
	case r := <-done:
		if r.err != nil {
			return r.err
		}
		if oldDone != nil {
			<-oldDone
		}
		if r.proofs != 2 || r.http != 1 || oldProofs.Load() != 1 {
			return errors.New("incomplete application proof")
		}
		if e := out.stage("pair-listener-closed", map[string]any{"proof_count": r.proofs, "http_count": r.http, "old_live_proof_count": oldProofs.Load()}); e != nil {
			return e
		}
	case <-time.After(time.Second):
		return errors.New("listener did not join")
	}
	return nil
}
func runDialer(ctx context.Context, scope *daemon.DevelopmentPeerScope, s *daemon.DevelopmentNetworkSession, n *proofnet.Net, c config, out *emitter, ready io.ReadCloser) error {
	wait, cancel, e := phase(ctx, c, 2*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	// Owned coordinator pipe, only a public readiness gate. Never an authority source.
	if e = scope.Own(ready); e != nil {
		return e
	}
	line := make(chan error, 1)
	go func() {
		b := make([]byte, len("peer-ready\n"))
		_, e := io.ReadFull(ready, b)
		if e == nil && string(b) != "peer-ready\n" {
			e = errors.New("invalid public readiness")
		}
		line <- e
	}()
	select {
	case e := <-line:
		if e != nil {
			return e
		}
	case <-wait.Done():
		ready.Close()
		<-line
		return errWindow
	}
	pin, e := exactPeer(s.Peers(), c)
	if e != nil {
		return e
	}
	if e = beforePayload(s, pin); e != nil {
		return e
	}
	for _, p := range s.Peers() {
		if p.PublicKey == pin.PublicKey && !p.HasEndpoints {
			return errors.New("peer endpoint absent")
		}
	}
	out.stage("pair-ready-observed", nil)
	dial := func(mode byte) (net.Conn, error) {
		bounded, stop, e := phase(ctx, c, 3*time.Second)
		if e != nil {
			return nil, e
		}
		defer stop()
		out.stage("pair-dial-start", map[string]any{"mode": string(mode)})
		conn, e := n.DialContextTCPAddrPort(bounded, netip.AddrPortFrom(netip.MustParseAddr(c.PeerIP), pairPort))
		if e != nil {
			return nil, e
		}
		if e = scope.Own(conn); e != nil {
			return nil, e
		}
		remote, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok || !remote.IP.Equal(net.ParseIP(c.PeerIP)) {
			return nil, errors.New("dial remote mismatch")
		}
		if e = beforePayload(s, pin); e != nil {
			return nil, e
		}
		out.stage("pair-dial-current", map[string]any{"mode": string(mode)})
		proofCtx, proofStop, e := phase(ctx, c, time.Second)
		if e != nil {
			return nil, e
		}
		defer proofStop()
		deadline, _ := proofCtx.Deadline()
		if e = conn.SetDeadline(deadline); e != nil {
			return nil, e
		}
		if _, e = conn.Write([]byte{mode}); e != nil {
			return nil, e
		}
		if e = challenge(conn, deadline); e != nil {
			return nil, e
		}
		out.stage("pair-client-proof", nil)
		return conn, nil
	}
	old, e := dial('P')
	if e != nil {
		return e
	}
	liveCtx, liveCancel, e := phase(ctx, c, time.Second)
	if e != nil {
		return e
	}
	liveDeadline, _ := liveCtx.Deadline()
	e = challenge(old, liveDeadline)
	liveCancel()
	if e != nil {
		return e
	}
	out.stage("pair-old-stream-live-verified", nil)
	if e = old.SetDeadline(time.Time{}); e != nil {
		return e
	}
	httpConn, e := dial('H')
	if e != nil {
		return e
	}
	httpCtx, httpCancel, e := phase(ctx, c, time.Second)
	if e != nil {
		return e
	}
	defer httpCancel()
	deadline, _ := httpCtx.Deadline()
	if e = httpConn.SetDeadline(deadline); e != nil {
		return e
	}
	if e = boundedHTTP(httpConn, c); e != nil {
		return e
	}
	if !time.Now().Before(time.UnixMilli(c.EarlierExpiryMS).Add(-time.Second)) || time.Since(time.UnixMilli(c.WorkStartMS)) >= 9*time.Second {
		return errWindow
	}
	out.stage("pair-http-verified", map[string]any{"remaining_ms": c.EarlierExpiryMS - time.Now().UnixMilli()})
	timer := time.NewTimer(time.Until(time.UnixMilli(c.EarlierExpiryMS).Add(50 * time.Millisecond)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-scope.Context().Done():
		if time.Now().UnixMilli() < c.EarlierExpiryMS {
			return errors.New("scope ended before expiry oracle")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// A NEW independent context makes this an actual fresh dial attempt, not merely
	// a trivially pre-cancelled scope context. Userspace network is already retiring.
	probe, probeCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer probeCancel()
	fresh, e := n.DialContextTCPAddrPort(probe, netip.AddrPortFrom(netip.MustParseAddr(c.PeerIP), pairPort))
	if e == nil {
		fresh.Close()
		return errors.New("fresh post-expiry dial unexpectedly succeeded")
	}
	if e = postExpiryChallenge(old); e == nil {
		return errors.New("old stream delivered after expiry")
	}
	return out.stage("pair-post-expiry-denied", map[string]any{"new_dial_denied": true, "old_stream_ack_denied": true})
}

// Even a closed old stream receives a fresh Write attempt; a SetDeadline refusal
// alone is not counted as attempted post-expiry application I/O.
func postExpiryChallenge(conn net.Conn) error {
	deadlineErr := conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	var fresh, ack [32]byte
	if _, e := rand.Read(fresh[:]); e != nil {
		return e
	}
	_, writeErr := conn.Write(fresh[:])
	if writeErr != nil {
		return writeErr
	}
	if deadlineErr != nil {
		conn.Close()
		return deadlineErr
	}
	if _, e := io.ReadFull(conn, ack[:]); e != nil {
		return e
	}
	sum := sha256.Sum256(fresh[:])
	if ack != sum {
		return errors.New("post-expiry ack mismatch")
	}
	return nil
}
