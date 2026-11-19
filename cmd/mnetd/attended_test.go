package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
)

type localPromptCall struct {
	prompt attendedPrompt
	ctx    context.Context
	answer chan string
}
type fixtureAttendedUI struct{ calls chan localPromptCall }

func (u *fixtureAttendedUI) Ask(ctx context.Context, p attendedPrompt) (string, error) {
	c := localPromptCall{p, ctx, make(chan string, 1)}
	select {
	case u.calls <- c:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	select {
	case a := <-c.answer:
		if a == "error" {
			return "", errors.New("dialog unavailable")
		}
		return a, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func takePrompt(t *testing.T, u *fixtureAttendedUI, kind string) localPromptCall {
	t.Helper()
	select {
	case c := <-u.calls:
		if c.prompt.Kind != kind {
			t.Fatalf("want %s got %s", kind, c.prompt.Kind)
		}
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("missing local prompt " + kind)
	}
	return localPromptCall{}
}

type attendedFixtureCapture struct {
	closed atomic.Bool
	image  []byte
}

func (c *attendedFixtureCapture) Capture() ([]byte, bool, error) { return c.image, true, nil }
func (c *attendedFixtureCapture) Geometry() (int, int)           { return 16, 16 }
func (c *attendedFixtureCapture) Close() error                   { c.closed.Store(true); return nil }

type attendedFixtureInput struct{ calls, closes atomic.Int32 }

func (i *attendedFixtureInput) Inject(session.InputEvent) error { i.calls.Add(1); return nil }
func (i *attendedFixtureInput) Close() error                    { i.closes.Add(1); return nil }
func waitAttended(t *testing.T, vc *session.ViewerControl, state string) session.ControlStatus {
	t.Helper()
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		s := vc.Snapshot()
		if s.State == state {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("want %s got %+v", state, vc.Snapshot())
	return session.ControlStatus{}
}
func attendedPair(t *testing.T, capable bool) (*session.Mux, *session.Mux) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	host := make(chan *session.Mux, 1)
	fail := make(chan error, 1)
	go func() {
		m, err := listenHeadlessSession(ln, "host", time.Second, func(session.Hello, net.Addr) (bool, string) { return true, "" })
		if err != nil {
			fail <- err
		} else {
			host <- m
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := session.DialWithControl(conn, "fixture-peer", nil, capable)
	if err != nil {
		t.Fatal(err)
	}
	var server *session.Mux
	select {
	case server = <-host:
	case err := <-fail:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("handshake")
	}
	t.Cleanup(func() { viewer.Close(); server.Close() })
	return viewer, server
}
func newAttendedFixture(t *testing.T, capable bool) (*session.Viewer, *session.ViewerControl, *fixtureAttendedUI, *attendedFixtureInput, *attendedFixtureCapture, <-chan struct{}) {
	t.Helper()
	client, server := attendedPair(t, capable)
	vc := session.NewViewerControl(nil)
	v := session.ConnectViewerWithOptions(client, session.ViewerOptions{Control: vc})
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatal(err)
	}
	capture := &attendedFixtureCapture{image: b.Bytes()}
	inj := &attendedFixtureInput{}
	ui := &fixtureAttendedUI{make(chan localPromptCall, 8)}
	_, done := serveAttendedSession(context.Background(), server, "fixture-peer", capable, ui, capture, func() (session.Injector, error) { return inj, nil }, func() bool { return true })
	t.Cleanup(func() {
		client.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("local session workers not joined")
		}
	})
	return v, vc, ui, inj, capture, done
}
func TestAttendedLocalGrantPauseRevokeAndEnd(t *testing.T) {
	v, vc, ui, inj, capture, done := newAttendedFixture(t, true)
	active := takePrompt(t, ui, "active")
	waitAttended(t, vc, "available")
	ev := session.InputEvent{Kind: "pointer", Action: "click"}
	if v.SendInput(ev) == nil {
		t.Fatal("view allowed input")
	}
	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	control := takePrompt(t, ui, "control")
	if v.SendInput(ev) == nil {
		t.Fatal("pending allowed input")
	}
	control.answer <- "allow"
	grant := waitAttended(t, vc, "allowed")
	if err := v.SendInput(ev); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	for inj.calls.Load() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if inj.calls.Load() != 1 {
		t.Fatal("approved input missing")
	}
	active.answer <- "pause"
	// The active window and resume question can arrive in either order.
	var resumed localPromptCall
	for n := 0; n < 2; n++ {
		select {
		case c := <-ui.calls:
			if c.prompt.Kind == "active" {
				active = c
			} else {
				resumed = c
			}
		case <-time.After(time.Second):
			t.Fatal("pause UI")
		}
	}
	waitAttended(t, vc, "paused")
	if inj.closes.Load() != 1 || v.SendInput(ev) == nil {
		t.Fatal("pause did not release/deny input")
	}
	resumed.answer <- "allow"
	waitAttended(t, vc, "allowed")
	active.answer <- "revoke"
	active = takePrompt(t, ui, "active")
	waitAttended(t, vc, "revoked")
	// Raw stale traffic cannot bypass the second check in attendedInput.
	_ = v.Mux.SendJSON(session.StreamInput, session.PriInteractive, session.InputEvent{Kind: "pointer", Action: "click", ControlSession: grant.Session, ControlRequest: grant.Request})
	time.Sleep(20 * time.Millisecond)
	if inj.calls.Load() != 1 || inj.closes.Load() != 2 {
		t.Fatal("revoke retained input")
	}
	// Files are unavailable in the actual attended host receiver.
	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if session.SendFile(v.Mux, path) == nil {
		t.Fatal("attended file reception enabled")
	}
	select {
	case <-v.Mux.Closed():
		t.Fatal("ordinary revoke ended viewing")
	default:
	}
	active.answer <- "end"
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("end not joined")
	}
	if !capture.closed.Load() {
		t.Fatal("capture retained")
	}
}
func TestAttendedCancelAndLateLocalAnswerCannotGrant(t *testing.T) {
	_, vc, ui, inj, _, _ := newAttendedFixture(t, true)
	takePrompt(t, ui, "active")
	waitAttended(t, vc, "available")
	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	old := takePrompt(t, ui, "control")
	if err := vc.Cancel(); err != nil {
		t.Fatal(err)
	}
	waitAttended(t, vc, "declined")
	select {
	case <-old.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("retired prompt not canceled")
	}
	old.answer <- "allow"
	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	next := takePrompt(t, ui, "control")
	if inj.calls.Load() != 0 {
		t.Fatal("late grant injected")
	}
	next.answer <- "deny"
	waitAttended(t, vc, "declined")
}
func TestAttendedUnsupportedPeerAndDialogFailureFailClosed(t *testing.T) {
	v, _, ui, inj, _, done := newAttendedFixture(t, false)
	active := takePrompt(t, ui, "active")
	if err := v.Mux.SendJSON(session.StreamInput, session.PriInteractive, session.InputEvent{Kind: "key", Text: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if inj.calls.Load() != 0 {
		t.Fatal("legacy peer injected")
	}
	active.answer <- "error"
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dialog failure did not end")
	}
}
func TestAttendedViewApprovalAndPeerLabel(t *testing.T) {
	for _, a := range []string{"allow", "deny", "error"} {
		ui := &fixtureAttendedUI{make(chan localPromptCall, 1)}
		done := make(chan bool, 1)
		go func() { done <- approveAttendedView(context.Background(), ui, "<b>peer</b>\n\u202eevil") }()
		p := takePrompt(t, ui, "view")
		if strings.ContainsAny(p.prompt.Peer, "\n\u202e") {
			t.Fatal("control characters in peer label")
		}
		p.answer <- a
		if got := <-done; got != (a == "allow") {
			t.Fatal(a, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if approveAttendedView(ctx, &fixtureAttendedUI{make(chan localPromptCall)}, "peer") {
		t.Fatal("canceled view approved")
	}
}
func TestAttendedOptionConflictsPreserveLegacy(t *testing.T) {
	if validateAttendedOptions(options{sessHost: true, sessAuto: true, sessRec: "legacy"}) != nil {
		t.Fatal("legacy changed")
	}
	if err := validateAttendedOptions(options{sessHost: true, sessAttended: true}); err != nil {
		t.Fatal(err)
	}
	for _, o := range []options{{sessAttended: true}, {sessHost: true, sessAttended: true, sessAuto: true}, {sessHost: true, sessAttended: true, sessRec: "record"}, {sessHost: true, sessAttended: true, serviceAction: "print-unit"}, {sessHost: true, sessAttended: true, sessProof: true}} {
		if validateAttendedOptions(o) == nil {
			t.Fatal("conflict accepted")
		}
	}
}

type failingAttendedInput struct{ closed atomic.Int32 }

func (i *failingAttendedInput) Inject(session.InputEvent) error { return nil }
func (i *failingAttendedInput) Close() error {
	i.closed.Add(1)
	return errors.New("synthetic close failure")
}
func TestAttendedInputCleanupFailureIsTerminal(t *testing.T) {
	gate := session.NewHostControl(nil)
	backend := &failingAttendedInput{}
	var failed atomic.Bool
	input := &attendedInput{gate: gate, backend: backend, fail: func() { failed.Store(true) }}
	input.revoke()
	if !failed.Load() || backend.closed.Load() != 1 || !input.failed || input.backend != nil {
		t.Fatal("cleanup failure did not retire session")
	}
	if input.decide(gate.Snapshot(), true) == nil {
		t.Fatal("failed input resumed")
	}
}
func TestAttendedParentCancellationJoinsPendingDialogs(t *testing.T) {
	client, server := attendedPair(t, true)
	vc := session.NewViewerControl(nil)
	session.ConnectViewerWithOptions(client, session.ViewerOptions{Control: vc})
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil)
	capture := &attendedFixtureCapture{image: b.Bytes()}
	ui := &fixtureAttendedUI{make(chan localPromptCall, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	_, done := serveAttendedSession(ctx, server, "peer", true, ui, capture, func() (session.Injector, error) { t.Error("input before approval"); return nil, nil }, func() bool { return true })
	active := takePrompt(t, ui, "active")
	waitAttended(t, vc, "available")
	_ = vc.Request()
	pending := takePrompt(t, ui, "control")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("parent stop not joined")
	}
	if !capture.closed.Load() || active.ctx.Err() == nil || pending.ctx.Err() == nil {
		t.Fatal("parent stop retained owner")
	}
}

func TestAttendedAdmissionRevalidatesAfterLocalAnswer(t *testing.T) {
	for _, withdraw := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "withdrawn"}[withdraw], func(t *testing.T) {
			ui := &fixtureAttendedUI{make(chan localPromptCall, 1)}
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			var current atomic.Bool
			current.Store(true)
			done := make(chan bool, 1)
			go func() { done <- approveAttendedConnection(context.Background(), ui, "peer", a, current.Load) }()
			prompt := takePrompt(t, ui, "view")
			if withdraw {
				current.Store(false)
			}
			prompt.answer <- "allow"
			if got := <-done; got == withdraw {
				t.Fatal("admission ignored current authorization", got)
			}
		})
	}
	ui := &fixtureAttendedUI{make(chan localPromptCall, 1)}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if approveAttendedConnection(context.Background(), ui, "peer", a, func() bool { return false }) {
		t.Fatal("unverified peer accepted")
	}
	select {
	case <-ui.calls:
		t.Fatal("unverified peer prompted host")
	default:
	}
}

func TestAttendedRealHelloWaitsForViewDecisionThenClearsDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ui := &fixtureAttendedUI{make(chan localPromptCall, 1)}
	ready := make(chan *session.Mux, 1)
	fail := make(chan error, 2)
	go func() {
		m, err := listenSessionWithApproval(ln, "host", 40*time.Millisecond, func(h session.Hello, _ net.Addr, c net.Conn) (bool, string) {
			return approveAttendedConnection(context.Background(), ui, h.NodeName, c, func() bool { return true }), "view approval"
		})
		if err != nil {
			fail <- err
		} else {
			ready <- m
		}
	}()
	viewer := make(chan *session.Mux, 1)
	go func() {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			fail <- err
			return
		}
		m, err := session.Dial(c, "peer")
		if err != nil {
			fail <- err
		} else {
			viewer <- m
		}
	}()
	p := takePrompt(t, ui, "view")
	time.Sleep(70 * time.Millisecond)
	select {
	case <-viewer:
		t.Fatal("view approval bypassed")
	case err := <-fail:
		t.Fatal(err)
	default:
	}
	p.answer <- "allow"
	var host, client *session.Mux
	for n := 0; n < 2; n++ {
		select {
		case host = <-ready:
		case client = <-viewer:
		case err := <-fail:
			t.Fatal(err)
		case <-time.After(time.Second):
			t.Fatal("view handshake did not finish")
		}
	}
	defer host.Close()
	defer client.Close()
	host.Keepalive(5*time.Millisecond, time.Second)
	until := time.Now().Add(time.Second)
	for host.RTT() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if host.RTT() == 0 {
		t.Fatal("post-approval transport retained hello deadline")
	}
}

func TestAttendedAuthorizationWithdrawalEndsLocalOwner(t *testing.T) {
	client, server := attendedPair(t, true)
	vc := session.NewViewerControl(nil)
	v := session.ConnectViewerWithOptions(client, session.ViewerOptions{Control: vc})
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil)
	capture := &attendedFixtureCapture{image: b.Bytes()}
	ui := &fixtureAttendedUI{make(chan localPromptCall, 4)}
	var authorized atomic.Bool
	authorized.Store(true)
	_, done := serveAttendedSession(context.Background(), server, "peer", true, ui, capture, func() (session.Injector, error) { return nil, errors.New("input unavailable") }, authorized.Load)
	active := takePrompt(t, ui, "active")
	waitAttended(t, vc, "available")
	_ = vc.Request()
	p := takePrompt(t, ui, "control")
	p.answer <- "allow"
	waitAttended(t, vc, "declined") // A host answer cannot grant a missing OS backend.
	authorized.Store(false)
	_ = v.SendChat("peer", "synthetic authorization check")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("withdrawn authorization kept sharing")
	}
	if !capture.closed.Load() || active.ctx.Err() == nil {
		t.Fatal("withdrawn session retained UI/capture")
	}
}

func TestAttendedInputOwnerCheckedAfterBackendCreation(t *testing.T) {
	client, server := attendedPair(t, true)
	vc := session.NewViewerControl(nil)
	_ = session.ConnectViewerWithOptions(client, session.ViewerOptions{Control: vc})
	gate := session.NewHostControl(nil)
	gate.Start(server)
	defer gate.End()
	waitAttended(t, vc, "available")
	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	waitAttended(t, vc, "pending")
	until := time.Now().Add(time.Second)
	for gate.Snapshot().State != "pending" {
		if time.Now().After(until) {
			t.Fatal("no host request")
		}
		time.Sleep(time.Millisecond)
	}
	alive := true
	backend := &attendedFixtureInput{}
	input := &attendedInput{gate: gate, live: func() bool { return alive }, create: func() (session.Injector, error) { alive = false; return backend, nil }}
	if err := input.decide(gate.Snapshot(), true); err == nil {
		t.Fatal("owner lost during backend creation granted control")
	}
	if gate.Snapshot().State == "allowed" || backend.closes.Load() != 1 {
		t.Fatal("retired owner kept backend or grant")
	}
	input.backend = backend
	if err := input.Inject(session.InputEvent{Kind: "pointer", Action: "click"}); err != nil {
		t.Fatal(err)
	}
	if backend.calls.Load() != 0 {
		t.Fatal("retired input owner dispatched")
	}
}
