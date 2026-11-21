//go:build linux && mnet_enrollment_development

package main

// Stage one of the disposable-network attended-host checks.
//
// These tests prove PROTOCOL denial over an actual userspace TCP/IP
// connection (internal/proofnet, real *net.TCPAddr far address, no wrapper):
// when the local approval is declined, errors, is closed, times out or loses
// authority while pending, the viewer is never admitted, no session exists,
// and no capture or input backend is ever created. The local approval here
// is a FAKE UI. This is NOT proof of the real zenity dialog, of default
// decline in the real dialog, of real display capture or input, or of any
// positive view or pointer session. It uses a single in-memory network
// with the host and viewer on one stack, port 49174, never 5909. It is
// selected only by the development tag so it never joins the ordinary Linux
// suite, and it is not a controller-bound two-device test.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/daemon"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"fintex.au/mnet/internal/proofnet"
	"fintex.au/mnet/internal/session"
	"golang.zx2c4.com/wireguard/tun"
)

const proofnetAttendedPort = 49174

type proofnetAnswerUI struct {
	asks   atomic.Int32
	answer string        // "deny", "error" or "" to wait for ctx
	delay  time.Duration // wait before answering
}

func (u *proofnetAnswerUI) Ask(ctx context.Context, _ attendedPrompt) (string, error) {
	u.asks.Add(1)
	if u.answer == "" {
		<-ctx.Done()
		return "", ctx.Err()
	}
	select {
	case <-time.After(u.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if u.answer == "error" {
		return "", errors.New("dialog unavailable")
	}
	return u.answer, nil
}

func runProofnetDenial(t *testing.T, ui *proofnetAnswerUI, hostCtxTimeout time.Duration, authorized func() bool) {
	t.Helper()
	addr := netip.MustParseAddr("100.64.0.5")
	dev, network, err := proofnet.CreateNetTUN([]netip.Addr{addr}, nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	ln, err := network.ListenTCPAddrPort(netip.AddrPortFrom(addr, proofnetAttendedPort))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	peer := daemon.SessionPeer{Name: "viewer-peer", IPv4: addr.String(), PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
	lookup := func() []daemon.SessionPeer { return []daemon.SessionPeer{peer} }

	// Explicit admission instrumentation: every accept-callback decision and
	// the approval result are recorded, and an admitted session would reach
	// the counted backend factories below. Any admitted session fails.
	var decisions, approved, backendsCreated atomic.Int32
	hostCtx, cancel := context.WithTimeout(context.Background(), hostCtxTimeout)
	defer cancel()
	type hostResult struct {
		mux *session.Mux
		err error
	}
	host := make(chan hostResult, 1)
	go func() {
		m, err := listenAttendedSessionWithApproval(ln, "host", 15*time.Second, func(h session.Hello, from net.Addr, c net.Conn) (bool, string) {
			decisions.Add(1)
			if _, _, ok := daemon.MatchSessionPeer(lookup(), h.NodeName, from); !ok {
				return false, "unknown peer"
			}
			ok := approveAttendedConnection(hostCtx, ui, h.NodeName, c, authorized)
			if ok {
				approved.Add(1)
			}
			return ok, "view approval"
		})
		if m != nil {
			// Production would call serveAttendedSession here; the counted
			// factories make an unexpected admission visible.
			_, done := serveAttendedSession(hostCtx, m, "viewer-peer", true, ui, &proofnetCountedCapture{&backendsCreated},
				func() (session.Injector, error) { backendsCreated.Add(1); return nil, errors.New("not expected") }, authorized)
			_ = done
		}
		host <- hostResult{m, err}
	}()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()
	conn, err := network.DialContextTCPAddrPort(dialCtx, netip.AddrPortFrom(addr, proofnetAttendedPort))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if far, ok := conn.RemoteAddr().(*net.TCPAddr); !ok || far.IP.String() != addr.String() {
		t.Fatalf("far address is not the real proofnet TCP address: %v", conn.RemoteAddr())
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	viewerMux, viewerErr := session.DialAttendedView(conn, "viewer-peer", func(session.HelloReply, net.Addr) error { return nil })
	if viewerErr == nil {
		viewerMux.Close()
		t.Fatal("viewer was admitted without local approval")
	}
	select {
	case res := <-host:
		if res.mux != nil {
			res.mux.Close()
			t.Fatal("host created a session without local approval")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("host did not retire the refused attempt")
	}
	if ui.asks.Load() == 0 && authorized() {
		t.Fatal("local approval was never asked")
	}
	if decisions.Load() == 0 {
		t.Fatal("host admission callback never ran; denial was not exercised")
	}
	if approved.Load() != 0 || backendsCreated.Load() != 0 {
		t.Fatalf("approved=%d backends=%d for a refused viewer", approved.Load(), backendsCreated.Load())
	}
}

func TestAttendedProofnetLocalDenialAdmitsNothing(t *testing.T) {
	always := func() bool { return true }
	for name, tc := range map[string]struct {
		ui      *proofnetAnswerUI
		timeout time.Duration
	}{
		"declined":              {&proofnetAnswerUI{answer: "deny"}, 5 * time.Second},
		"dialog-error":          {&proofnetAnswerUI{answer: "error"}, 5 * time.Second},
		"dialog-closed-timeout": {&proofnetAnswerUI{}, 300 * time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) { runProofnetDenial(t, tc.ui, tc.timeout, always) })
	}
}

// Authority lost while the prompt is pending retires the prompt and refuses
// the viewer, even though the fake UI would otherwise wait indefinitely.
func TestAttendedProofnetAuthorityLossWhilePendingRetires(t *testing.T) {
	var live atomic.Bool
	live.Store(true)
	go func() {
		time.Sleep(300 * time.Millisecond)
		live.Store(false)
	}()
	runProofnetDenial(t, &proofnetAnswerUI{}, 10*time.Second, live.Load)
}

type proofnetCountedCapture struct{ n *atomic.Int32 }

func (c *proofnetCountedCapture) Capture() ([]byte, bool, error) {
	c.n.Add(1)
	return nil, false, errors.New("not expected")
}
func (c *proofnetCountedCapture) Geometry() (int, int) { c.n.Add(1); return 16, 16 }

// ---------------------------------------------------------------------------
// Controller-bound negative fixture (native role).
//
// Runs only when MNET_NATIVE_OWNED_WORKFLOW_DIR names an owned fixture
// directory. It uses the exported RunDisposableEnrollmentNetwork entry, never
// a hand-built session, and a deny-only FAKE UI: it proves protocol denial,
// scope ownership and retirement, not the real dialog, capture, input or any
// positive view or pointer session.

type nativeFixtureConfig struct {
	Policy config.LocalConfigV2
	Limits registrar.WorkflowLimits
}

// Coordinates are written by the control coordinator from its CURRENT enrolled
// map; native only reads and compares them. Field names are the control's.
type nativeCoordinates struct {
	Version int                `json:"version"`
	Host    daemon.SessionPeer `json:"host"`
	Viewer  daemon.SessionPeer `json:"viewer"`
}

type nativeListening struct {
	nativeCoordinates
	FakeUI bool   `json:"fake_ui"`
	UIKind string `json:"ui_kind,omitempty"`
}

type nativePeerRefused struct {
	Version                     int    `json:"version"`
	FakeUI                      bool   `json:"fake_ui"`
	ProtocolRefused             bool   `json:"protocolRefused"`
	ActualFarIP                 string `json:"actualFarIP"`
	AcceptedReplyVerifierCalled bool   `json:"acceptedReplyVerifierCalled"`
	Joined                      bool   `json:"joined"`
}

type nativeAttendedOutcome struct {
	Version           int    `json:"version"`
	Case              string `json:"case"`
	FakeUI            bool   `json:"fake_ui"`
	UIKind            string `json:"ui_kind"`
	DialogClosed      *bool  `json:"dialogClosedWithoutAllow,omitempty"`
	AllowAttempts     int32  `json:"allowAttempts"`
	ChallengeVerified bool   `json:"challengeVerified"`
	AdmissionCalls    int32  `json:"admissionCalls"`
	UIAsks            int32  `json:"uiAsks"`
	Approved          int32  `json:"approved"`
	Muxes             int32  `json:"muxes"`
	CaptureCalls      int32  `json:"captureCalls"`
	InputCreates      int32  `json:"inputCreates"`
	Joined            bool   `json:"joined"`
	ListenerClosed    bool   `json:"listenerClosed"`
	ConnClosed        bool   `json:"connClosed"`
	MuxClosed         *bool  `json:"muxClosed"`
	PromptCancelled   *bool  `json:"promptCancelled,omitempty"`
	LateOwnDenied     bool   `json:"lateOwnDenied"`
	ScopeReason       string `json:"scopeReason"`
}

type nativeFailed struct {
	Version    int    `json:"version"`
	FakeUI     bool   `json:"fake_ui"`
	UIKind     string `json:"ui_kind"`
	Stage      string `json:"stage"`
	ErrorClass string `json:"errorClass"`
}

// writeNativeFile publishes a file atomically and privately. exclusive makes
// a second writer of the same name fail instead of replacing the first.
func writeNativeFile(dir, name string, raw []byte, exclusive bool) error {
	final := filepath.Join(dir, name)
	if exclusive {
		f, err := os.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, werr := f.Write(raw)
		cerr := f.Close()
		if werr != nil {
			return werr
		}
		return cerr
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// publishNativeResult writes exactly one of outcome and failed per run.
func publishNativeResult(dir string, outcome *nativeAttendedOutcome, failed *nativeFailed) error {
	name, value := "native-attended-outcome.json", any(outcome)
	other := "native-attended-failed.json"
	if failed != nil {
		name, value, other = "native-attended-failed.json", any(failed), "native-attended-outcome.json"
	}
	if _, err := os.Lstat(filepath.Join(dir, other)); err == nil {
		return errors.New("the other result file already exists")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeNativeFile(dir, name, raw, true)
}

// ensureNativeRoot creates the private disposable root from the config, or,
// when it already exists, requires its files to equal the config exactly.
func ensureNativeRoot(root string, cfg nativeFixtureConfig) error {
	policy, err := json.Marshal(cfg.Policy)
	if err != nil {
		return err
	}
	limits, err := json.Marshal(cfg.Limits)
	if err != nil {
		return err
	}
	want := map[string][]byte{
		"DISPOSABLE-MNET-SETUP":     []byte("mnet-disposable-enrollment-v1\n"),
		"enrollment-policy-v2.json": policy,
		"workflow-limits.json":      limits,
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		if err := os.Mkdir(root, 0o700); err != nil {
			return err
		}
		for name, raw := range want {
			if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	for name, raw := range want {
		have, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if !bytes.Equal(have, raw) {
			return errors.New("existing root file differs from native-workflow-config: " + name)
		}
	}
	return nil
}

// nativeOwnedListener lets the peer scope own the accepted connection too.
type nativeOwnedListener struct {
	net.Listener
	scope *daemon.DevelopmentPeerScope
	mu    sync.Mutex
	conn  net.Conn
}

func (l *nativeOwnedListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.conn = c
	l.mu.Unlock()
	if err := l.scope.Own(c); err != nil {
		return nil, err
	}
	return c, nil
}

func waitNativeFile(ctx context.Context, path string, limit time.Duration) ([]byte, error) {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if raw, err := os.ReadFile(path); err == nil {
			return raw, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("timeout")
		case <-tick.C:
		}
	}
}

type nativeClosed struct{ closed atomic.Bool }

func (c *nativeClosed) Close() error { c.closed.Store(true); return nil }

func TestActualMultiesAttendedNativeWorkflowFixture(t *testing.T) {
	runNativeAttendedCase(t, "deny")
}

// Withdrawal while the host prompt is pending. Selected by its own test name
// so the ordinary deny case is unchanged.
func TestActualMultiesAttendedWithdrawNativeWorkflowFixture(t *testing.T) {
	runNativeAttendedCase(t, "withdraw-pending")
}

// Real dialog negative case: the REAL zenity UI on an isolated Xvfb display,
// never a physical one. An "Allow" answer is never honoured here (this case
// proves default decline only); it is counted and fails the case.
func TestActualMultiesAttendedRealDialogNativeWorkflowFixture(t *testing.T) {
	runNativeAttendedCase(t, "deny-real-dialog")
}

const nativeRealDialogBound = 20 * time.Second

// nativeCaseUI is the FAKE UI: deny answers at once, withdraw blocks until
// its context is cancelled and records that it was cancelled.
type nativeCaseUI struct {
	asks      atomic.Int32
	cancelled atomic.Bool
	wait      bool
	// Real dialog mode (inner != nil).
	inner         attendedUI
	allowAttempts atomic.Int32
	timedOut      atomic.Bool
	closed        atomic.Bool // zenity ended without Allow before the bound
	badKind       atomic.Int32
}

func (u *nativeCaseUI) Ask(ctx context.Context, p attendedPrompt) (string, error) {
	u.asks.Add(1)
	if u.inner != nil {
		if _, ok := ctx.Deadline(); !ok {
			ctx2, cancel := context.WithTimeout(ctx, nativeRealDialogBound)
			defer cancel()
			ctx = ctx2
		}
		if p.Kind != "view" {
			u.badKind.Add(1)
			return "", errors.New("unexpected prompt kind")
		}
		ans, err := u.inner.Ask(ctx, p)
		switch {
		case err == nil && strings.EqualFold(ans, "allow"):
			u.allowAttempts.Add(1)
			return "", errors.New("allow not honoured in the negative case")
		case ctx.Err() == context.DeadlineExceeded:
			u.timedOut.Store(true)
		case ctx.Err() != nil:
			u.cancelled.Store(true)
		default:
			u.closed.Store(true)
		}
		return "", errors.New("dialog not allowed")
	}
	if !u.wait {
		return "deny", nil
	}
	<-ctx.Done()
	u.cancelled.Store(true)
	return "", ctx.Err()
}

func runNativeAttendedCase(t *testing.T, caseName string) {
	dir := os.Getenv("MNET_NATIVE_OWNED_WORKFLOW_DIR")
	if dir == "" {
		t.Skip("MNET_NATIVE_OWNED_WORKFLOW_DIR not set")
	}
	real := caseName == "deny-real-dialog"
	uiKind := "fake"
	if real {
		uiKind = "real-zenity"
	}
	stage := "config"
	fail := func(class string) {
		t.Helper()
		_ = publishNativeResult(dir, nil, &nativeFailed{Version: 1, FakeUI: !real, UIKind: uiKind, Stage: stage, ErrorClass: class})
		t.Fatalf("attended native fixture failed at %s (%s)", stage, class)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "native-workflow-config.json"))
	if err != nil {
		fail("config-unreadable")
	}
	var cfg nativeFixtureConfig
	if json.Unmarshal(raw, &cfg) != nil || !registrar.ValidWorkflowLimits(cfg.Limits) {
		fail("config-invalid")
	}
	if _, err := config.ValidateLocalV2(cfg.Policy); err != nil {
		fail("policy-invalid")
	}
	var realUI attendedUI
	if real {
		// Never inherit a physical display: the driver must name the isolated
		// one explicitly and DISPLAY must equal it.
		stage = "display-isolation"
		iso := os.Getenv("MNET_ISOLATED_X11_DISPLAY")
		if iso == "" || os.Getenv("DISPLAY") != iso || os.Getenv("WAYLAND_DISPLAY") != "" {
			fail("display-not-isolated")
		}
		var err error
		if realUI, err = newAttendedUI(); err != nil {
			fail("real-ui-unavailable")
		}
		stage = "config"
	}
	root := filepath.Join(dir, "attended-setup")
	if err := ensureNativeRoot(root, cfg); err != nil {
		fail("root-mismatch")
	}

	var (
		network                       *proofnet.Net
		selfIP                        string
		launch                        *contract.SourceLaunch
		result                        *daemon.DevelopmentEnrollmentResult
		out                           nativeAttendedOutcome
		decisions, approved, backends atomic.Int32
		ranOK                         bool
	)
	withdraw := caseName == "withdraw-pending"
	ui := &nativeCaseUI{wait: withdraw, inner: realUI}
	out = nativeAttendedOutcome{Version: 1, Case: caseName, FakeUI: !real, UIKind: uiKind}

	emit := func(ev daemon.DevelopmentEnrollmentEvent) error {
		switch ev.Stage {
		case "waiting-for-browser":
			stage = "launch"
			if ev.Launch == nil {
				return errors.New("no launch")
			}
			l := *ev.Launch
			launch = &l
			public, _ := json.Marshal(l)
			if err := writeNativeFile(dir, "native-launch.json", public, false); err != nil {
				return err
			}
			if _, err := waitNativeFile(context.Background(), filepath.Join(dir, "native-continue"), 90*time.Second); err != nil {
				return err
			}
		case "joined-not-connected":
			stage = "result"
			if ev.Result == nil || !ev.Result.NativeOwnedKeyConfirmed || !ev.Result.NativeProofStateUnchanged || !ev.Result.NativeStateFinalized {
				return errors.New("actual result flags not all true")
			}
			r := *ev.Result
			result = &r
			rawResult, _ := json.Marshal(r)
			if err := writeNativeFile(dir, "native-workflow-outcome.json", rawResult, false); err != nil {
				return err
			}
		}
		return nil
	}

	options := daemon.DevelopmentNetworkOptions{
		Limits: controlclient.RegistrarLimits{Timeout: cfg.Limits.Timeout, MaxConcurrent: 1, MaxRequests: 16},
		Device: func(ip string) (tun.Device, error) {
			addr, err := netip.ParseAddr(ip)
			if err != nil || !addr.Is4() {
				return nil, errors.New("invalid disposable assigned address")
			}
			injected, n, err := proofnet.CreateNetTUN([]netip.Addr{addr}, nil, 1280)
			network, selfIP = n, ip
			return injected, err
		},
		Run: func(ctx context.Context, s *daemon.DevelopmentNetworkSession) error {
			stage = "peer-select"
			var selected *daemon.DevelopmentNetworkPeer
			for _, p := range s.Peers() {
				if !p.HasEndpoints {
					continue
				}
				if selected != nil {
					return errors.New("ambiguous endpoint-bearing peer")
				}
				c := p
				selected = &c
			}
			if selected == nil || launch == nil || result == nil {
				return errors.New("no unique current peer, launch or result")
			}
			pin := daemon.SessionPeer{Name: selected.Name, IPv4: selected.IPv4, PublicKey: selected.PublicKey}
			stage = "bind"
			scope, err := s.BindPeer(pin)
			if err != nil {
				return err
			}
			defer scope.Close()

			// Outbound path first: a fresh 32 byte challenge to the counterpart
			// on 49173. This proves the outbound path only.
			stage = "challenge"
			dialCtx, cancel := context.WithTimeout(scope.Context(), 3*time.Second)
			defer cancel()
			conn, err := network.DialContextTCPAddrPort(dialCtx, netip.AddrPortFrom(netip.MustParseAddr(pin.IPv4), 49173))
			if err != nil {
				return err
			}
			if err := scope.Own(conn); err != nil {
				return err
			}
			if far, ok := conn.RemoteAddr().(*net.TCPAddr); !ok || !far.IP.Equal(net.ParseIP(pin.IPv4)) {
				return errors.New("challenge far address is not the pinned peer")
			}
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			var challenge, response [32]byte
			if _, err := rand.Read(challenge[:]); err != nil {
				return err
			}
			if _, err := conn.Write(challenge[:]); err != nil {
				return err
			}
			if _, err := io.ReadFull(conn, response[:]); err != nil {
				return err
			}
			if response != sha256.Sum256(challenge[:]) {
				return errors.New("challenge response mismatch")
			}
			_ = conn.Close()
			out.ChallengeVerified = true

			// Local bounded snapshot lookup, distinct from the finite Refresh.
			lookup := func() []daemon.SessionPeer {
				var peers []daemon.SessionPeer
				for _, p := range s.Peers() {
					peers = append(peers, daemon.SessionPeer{Name: p.Name, IPv4: p.IPv4, PublicKey: p.PublicKey})
				}
				return peers
			}
			authorized := func() bool {
				if scope.Context().Err() != nil {
					return false
				}
				for _, p := range lookup() {
					if p == pin {
						return true
					}
				}
				return false
			}

			stage = "listen"
			raw, err := network.ListenTCPAddrPort(netip.AddrPortFrom(netip.MustParseAddr(selfIP), proofnetAttendedPort))
			if err != nil {
				return err
			}
			ln := &nativeOwnedListener{Listener: raw, scope: scope}
			if err := scope.Own(ln); err != nil {
				return err
			}
			stage = "coordinates"
			rawCoords, err := waitNativeFile(ctx, filepath.Join(dir, "peer-attended-coordinates.json"), 25*time.Second)
			if err != nil {
				return err
			}
			var coords nativeCoordinates
			if json.Unmarshal(rawCoords, &coords) != nil || coords.Version != 1 || coords.Viewer != pin ||
				coords.Host.IPv4 != selfIP || !sameNativeKey(coords.Host.PublicKey, launch.DevicePublicKey) || coords.Host.Name == "" {
				return errors.New("coordinates differ from native endpoint or current unique peer")
			}
			stage = "listening"
			rawReady, _ := json.Marshal(nativeListening{nativeCoordinates: coords, FakeUI: !real, UIKind: uiKind})
			if err := writeNativeFile(dir, "native-attended-listening.json", rawReady, false); err != nil {
				return err
			}

			stage = "admission"
			var withdrawDone chan error
			if withdraw || real {
				withdrawDone = make(chan error, 1)
				go func() {
					// Only after the callback is inside the fake ask.
					for ui.asks.Load() == 0 {
						select {
						case <-ctx.Done():
							withdrawDone <- ctx.Err()
							return
						case <-time.After(5 * time.Millisecond):
						}
					}
					pending, _ := json.Marshal(map[string]any{"version": 1, "fake_ui": !real, "ui_kind": uiKind, "case": caseName, "admissionCalls": decisions.Load(), "uiAsks": ui.asks.Load(), "host": coords.Host, "viewer": coords.Viewer})
					if err := writeNativeFile(dir, "native-attended-pending.json", pending, false); err != nil {
						withdrawDone <- err
						return
					}
					if !withdraw {
						withdrawDone <- nil // real dialog: pending marker only
						return
					}
					rawW, err := waitNativeFile(ctx, filepath.Join(dir, "peer-attended-withdrawn.json"), 30*time.Second)
					if err != nil {
						withdrawDone <- err
						return
					}
					var w struct {
						Version   int  `json:"version"`
						FakeUI    bool `json:"fake_ui"`
						Withdrawn bool `json:"withdrawn"`
					}
					if json.Unmarshal(rawW, &w) != nil || w.Version != 1 || !w.FakeUI || !w.Withdrawn {
						withdrawDone <- errors.New("withdrawal marker invalid")
						return
					}
					withdrawDone <- s.Refresh() // real map refresh retires the exact peer scope
				}()
			}
			type hostResult struct {
				mux *session.Mux
				err error
			}
			host := make(chan hostResult, 1)
			var joinHost sync.WaitGroup
			joinHost.Add(1)
			go func() {
				defer joinHost.Done()
				m, err := listenAttendedSessionWithApproval(ln, coords.Host.Name, 15*time.Second, func(h session.Hello, from net.Addr, c net.Conn) (bool, string) {
					decisions.Add(1)
					name, key, ok := daemon.MatchSessionPeer(lookup(), h.NodeName, from)
					if !ok || name != pin.Name || key != pin.PublicKey {
						return false, "unknown peer"
					}
					ok = approveAttendedConnection(scope.Context(), ui, h.NodeName, c, authorized)
					if ok {
						approved.Add(1)
					}
					return ok, "view approval"
				})
				if m != nil {
					backends.Add(1) // any admitted session is a failure below
					m.Close()
				}
				host <- hostResult{m, err}
			}()
			select {
			case res := <-host:
				joinHost.Wait()
				out.Muxes = backends.Load()
				if res.mux != nil || res.err == nil {
					return errors.New("host admitted or returned no error")
				}
				if !withdraw && !strings.Contains(res.err.Error(), "refused by accept callback") {
					return errors.New("host did not explicitly refuse by callback")
				}
			case <-time.After(30 * time.Second):
				return errors.New("no counterpart admission attempt")
			case <-ctx.Done():
				return ctx.Err()
			}
			if real {
				stage = "dialog-result"
				select {
				case err := <-withdrawDone:
					if err != nil {
						return err
					}
				case <-time.After(5 * time.Second):
					return errors.New("pending marker goroutine not joined")
				}
				if ui.badKind.Load() != 0 || ui.timedOut.Load() || ui.cancelled.Load() || !ui.closed.Load() && ui.allowAttempts.Load() == 0 {
					return errors.New("real dialog did not end by a decline or close")
				}
				closed := ui.closed.Load()
				out.DialogClosed = &closed
				out.AllowAttempts = ui.allowAttempts.Load()
				if out.AllowAttempts != 0 {
					return errors.New("real dialog was allowed; not honoured, case fails")
				}
				stage = "admission"
			}
			out.AdmissionCalls, out.UIAsks, out.Approved = decisions.Load(), ui.asks.Load(), approved.Load()
			if out.AdmissionCalls != 1 || out.UIAsks != 1 || out.Approved != 0 || out.Muxes != 0 {
				return errors.New("admission counters differ from one refused attempt")
			}

			if withdraw {
				stage = "withdraw-refresh"
				select {
				case err := <-withdrawDone:
					if err != nil {
						return err
					}
				case <-time.After(5 * time.Second):
					return errors.New("withdraw refresh did not finish")
				}
				if scope.Context().Err() == nil || scope.Reason() == "" {
					return errors.New("scope not retired by actual map withdrawal")
				}
				cancelled := ui.cancelled.Load()
				if !cancelled {
					return errors.New("pending prompt was not cancelled")
				}
				out.PromptCancelled = &cancelled
			} else {
				stage = "peer-refused"
				rawPeer, err := waitNativeFile(ctx, filepath.Join(dir, "peer-attended-refused.json"), 30*time.Second)
				if err != nil {
					return err
				}
				var peerRefused nativePeerRefused
				dec := json.NewDecoder(bytes.NewReader(rawPeer))
				dec.DisallowUnknownFields()
				if dec.Decode(&peerRefused) != nil || peerRefused.Version != 1 || peerRefused.FakeUI != !real || !peerRefused.ProtocolRefused ||
					peerRefused.AcceptedReplyVerifierCalled || !peerRefused.Joined || peerRefused.ActualFarIP != selfIP {
					return errors.New("counterpart did not report an explicit protocol refusal from the host address")
				}
			}

			// Retirement: close the scope and observe the owned resources.
			stage = "retirement"
			if err := scope.Close(); err != nil && !withdraw {
				return err
			}
			if _, err := raw.Accept(); err == nil {
				return errors.New("listener still accepts after scope retirement")
			}
			out.ListenerClosed = true
			ln.mu.Lock()
			hostConn := ln.conn
			ln.mu.Unlock()
			if hostConn == nil {
				return errors.New("no accepted connection was owned")
			}
			_ = hostConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			if _, err := hostConn.Read(make([]byte, 1)); err == nil {
				return errors.New("owned connection still readable after retirement")
			}
			out.ConnClosed = true
			probe := &nativeClosed{}
			if err := scope.Own(probe); err == nil || !probe.closed.Load() {
				return errors.New("late Own was not denied and closed")
			}
			out.LateOwnDenied = true
			out.ScopeReason = scope.Reason()
			if out.ScopeReason == "" {
				return errors.New("scope retired without a reason")
			}
			out.Joined = true
			ranOK = true
			return nil
		},
	}
	if err := daemon.RunDisposableEnrollmentNetwork(context.Background(), root, emit, options); err != nil || !ranOK {
		fail("run-failed")
	}
	if err := publishNativeResult(dir, &out, nil); err != nil {
		t.Fatal(err)
	}
}

// sameNativeKey compares a map key (padded standard base64) with the launch
// key (strict unpadded base64url) by their decoded 32 bytes. Both must decode
// strictly and canonically; the text forms are never compared.
func sameNativeKey(mapKey, launchKey string) bool {
	a, err := base64.StdEncoding.Strict().DecodeString(mapKey)
	if err != nil || len(a) != 32 {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(launchKey)
	if err != nil || len(b) != 32 {
		return false
	}
	return bytes.Equal(a, b)
}

func TestSameNativeKeyRepresentations(t *testing.T) {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(0xfb - i) // exercises + / vs - _ alphabets
	}
	std, url := base64.StdEncoding.EncodeToString(k), base64.RawURLEncoding.EncodeToString(k)
	if !sameNativeKey(std, url) {
		t.Fatal("equal keys not matched")
	}
	other := append([]byte(nil), k...)
	other[31] ^= 1
	if sameNativeKey(std, base64.RawURLEncoding.EncodeToString(other)) || sameNativeKey(url, url) || sameNativeKey(std, std) || sameNativeKey(std, "") {
		t.Fatal("different, wrong-alphabet or empty key accepted")
	}
}

type nativeStubUI struct {
	ans string
	err error
}

func (u nativeStubUI) Ask(context.Context, attendedPrompt) (string, error) { return u.ans, u.err }

// The real-dialog wrapper never honours Allow and classifies other ends.
func TestNativeRealDialogWrapperNeverHonoursAllow(t *testing.T) {
	ctx := context.Background()
	view := attendedPrompt{Kind: "view", Peer: "p"}
	u := &nativeCaseUI{inner: nativeStubUI{ans: "allow"}}
	if a, err := u.Ask(ctx, view); a != "" || err == nil || u.allowAttempts.Load() != 1 || u.closed.Load() {
		t.Fatal("allow was honoured or not counted")
	}
	u = &nativeCaseUI{inner: nativeStubUI{err: errors.New("local dialog closed or unavailable")}}
	if a, err := u.Ask(ctx, view); a != "" || err == nil || !u.closed.Load() || u.allowAttempts.Load() != 0 {
		t.Fatal("close not classified")
	}
	u = &nativeCaseUI{inner: nativeStubUI{ans: "allow"}}
	if _, err := u.Ask(ctx, attendedPrompt{Kind: "control"}); err == nil || u.badKind.Load() != 1 || u.allowAttempts.Load() != 0 {
		t.Fatal("non-view prompt not refused")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	u = &nativeCaseUI{inner: nativeStubUI{err: errors.New("x")}}
	if _, err := u.Ask(cctx, view); err == nil || !u.cancelled.Load() || u.closed.Load() {
		t.Fatal("scope cancel not classified apart from close")
	}
}
