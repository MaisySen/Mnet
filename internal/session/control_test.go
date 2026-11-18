package session

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func waitControl(t *testing.T, c *ViewerControl, state string) ControlStatus {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		s := c.Snapshot()
		if s.State == state {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("control want %s got %+v", state, c.Snapshot())
	return ControlStatus{}
}
func TestAttendedControlRequiresBoundHostDecisionAndRevocationPreservesView(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	requests := make(chan ControlStatus, 8)
	hc := NewHostControl(func(s ControlStatus) { requests <- s })
	vc := NewViewerControl(nil)
	v := ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	inj := &authorizationInjector{}
	h := ServeHostWithOptions(host, &authorizationCapturer{}, inj, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	waitControl(t, vc, "available")
	ev := InputEvent{Kind: "pointer", Action: "click"}
	if v.SendInput(ev) == nil {
		t.Fatal("view admission granted input")
	}
	if vc.Request() != nil {
		t.Fatal("request failed")
	}
	var request ControlStatus
	select {
	case request = <-requests:
	case <-time.After(time.Second):
		t.Fatal("host did not receive request")
	}
	if request.State != "pending" {
		t.Fatal(request)
	}
	if hc.Decide("other-session", request.Request, true) == nil || hc.Decide(request.Session, "other-request", true) == nil {
		t.Fatal("unbound decision accepted")
	}
	if v.SendInput(ev) == nil {
		t.Fatal("pending request granted input")
	}
	if hc.Decide(request.Session, request.Request, true) != nil {
		t.Fatal("local decision failed")
	}
	waitControl(t, vc, "allowed")
	if v.SendInput(ev) != nil {
		t.Fatal("bound input refused")
	}
	until := time.Now().Add(time.Second)
	for inj.calls.Load() != 1 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if inj.calls.Load() != 1 {
		t.Fatal("granted input lost")
	}
	if hc.Pause(request.Session, request.Request) != nil {
		t.Fatal("host pause failed")
	}
	waitControl(t, vc, "paused")
	if v.SendInput(ev) == nil {
		t.Fatal("paused dialog allowed input")
	}
	if hc.Decide(request.Session, request.Request, true) != nil {
		t.Fatal("explicit Keep failed")
	}
	waitControl(t, vc, "allowed")
	if hc.Revoke(request.Session, request.Request) != nil {
		t.Fatal("revoke failed")
	}
	waitControl(t, vc, "revoked")
	// Bypass viewer gating to prove enforcement is not a UI-only convention.
	_ = client.SendJSON(StreamInput, PriInteractive, InputEvent{Kind: "pointer", Action: "click", ControlSession: request.Session, ControlRequest: request.Request})
	time.Sleep(20 * time.Millisecond)
	if inj.calls.Load() != 1 {
		t.Fatal("revoked raw input reached platform")
	}
	select {
	case <-client.Closed():
		t.Fatal("revoke ended viewing")
	default:
	}
	if hc.Decide(request.Session, request.Request, true) == nil {
		t.Fatal("old answer revived revoked request")
	}
	if vc.Request() != nil {
		t.Fatal("new request unavailable")
	}
	next := <-requests
	if next.Request == request.Request {
		t.Fatal("request identity reused")
	}
	if hc.Decide(request.Session, request.Request, true) == nil {
		t.Fatal("stale answer approved newer request")
	}
	if hc.Decide(next.Session, next.Request, false) != nil {
		t.Fatal("decline failed")
	}
	waitControl(t, vc, "declined")
}
func TestControlUnsupportedAndCanceledRequestsStayViewOnly(t *testing.T) {
	for _, negotiate := range []bool{false, true} {
		client, host := pipe(t)
		hc := NewHostControl(nil)
		vc := NewViewerControl(nil)
		v := ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
		h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: negotiate})
		if !negotiate {
			if vc.Request() == nil || v.SendInput(InputEvent{}) == nil {
				t.Fatal("unsupported peer granted control")
			}
		} else {
			waitControl(t, vc, "available")
			if vc.Request() != nil {
				t.Fatal("request failed")
			}
			pending := vc.Snapshot()
			if vc.Cancel() != nil {
				t.Fatal("cancel failed")
			}
			// Even a late allowed status with a newer revision must not revive a canceled request.
			hc.mu.Lock()
			hc.status.Request = pending.Request
			hc.status.State = "allowed"
			_ = hc.emitLocked()
			hc.mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			if v.SendInput(InputEvent{}) == nil {
				t.Fatal("late allowed revived canceled request")
			}
		}
		h.Stop()
		client.Close()
		host.Close()
	}
}

// A blocked peer cannot prevent the owner's local revoke or hold the gate lock.
func TestControlBackpressureRevokesWithoutWaiting(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	m := newMux(a, 32)
	defer m.Close()
	m.started = true
	m.queuedBytes = maxQueuedBytes
	c := NewHostControl(nil)
	c.mux = m
	c.stream = 32
	c.status.Request = controlToken()
	c.status.State = "allowed"
	previous := c.Snapshot()
	done := make(chan error, 1)
	go func() { done <- c.Revoke(previous.Session, previous.Request) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("full queue accepted status")
		}
	case <-time.After(time.Second):
		t.Fatal("local revoke blocked on peer")
	}
	if c.Snapshot().State != "unavailable" || c.InputAllowed(InputEvent{ControlSession: previous.Session, ControlRequest: previous.Request}) {
		t.Fatal("capacity failure retained grant")
	}
	select {
	case <-m.Closed():
	default:
		t.Fatal("peer retained stale control session")
	}
}
func TestControlEndCannotBeRevived(t *testing.T) {
	hc := NewHostControl(nil)
	hc.status.Request = controlToken()
	hc.status.State = "pending"
	old := hc.Snapshot()
	hc.End()
	if hc.Decide(old.Session, old.Request, true) == nil || hc.Snapshot().State != "unavailable" {
		t.Fatal("host End revived")
	}
	a, b := pipe(t)
	defer a.Close()
	defer b.Close()
	vc := NewViewerControl(nil)
	vc.End()
	raw, _ := json.Marshal(controlMessage{Kind: controlKind, Session: controlToken()})
	vc.Handle(Stream{Mux: a, ID: 32, OpenJSON: raw})
	if vc.Snapshot().State != "unavailable" || vc.Request() == nil || vc.BindInput(&InputEvent{}) == nil {
		t.Fatal("viewer End revived")
	}
}

func TestControlCancelRacingGrantRevokesHost(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	requests := make(chan ControlStatus, 4)
	hc := NewHostControl(func(s ControlStatus) { requests <- s })
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	waitControl(t, vc, "available")
	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	r := <-requests
	// Host has approved but the viewer has not yet consumed that answer.
	hc.mu.Lock()
	hc.status.State = "allowed"
	hc.mu.Unlock()
	if err := vc.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-requests:
		if s.State != "declined" {
			t.Fatal(s)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel after host grant ignored")
	}
	if hc.InputAllowed(InputEvent{ControlSession: r.Session, ControlRequest: r.Request}) {
		t.Fatal("cancel left host armed")
	}
}
func TestControlRejectsUnsolicitedAndReplayedStatus(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	hc := NewHostControl(nil)
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	waitControl(t, vc, "available")
	initial := vc.Snapshot()
	for _, m := range []controlMessage{
		{Kind: controlKind, Session: initial.Session, Request: controlToken(), State: "allowed", Revision: initial.Revision + 1},
		{Kind: controlKind, Session: controlToken(), State: "available", Revision: initial.Revision + 2},
		{Kind: controlKind, Session: initial.Session, State: "available", Revision: initial.Revision},
	} {
		if err := host.controlJSON(hc.stream, m); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if vc.Snapshot() != initial || vc.BindInput(&InputEvent{}) == nil {
		t.Fatal("foreign/stale status changed gate")
	}
}

func TestControlStreamFailureEndsBothGates(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	hc := NewHostControl(nil)
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	waitControl(t, vc, "available")
	if err := client.SendBytes(hc.stream, PriInteractive, []byte("not-json")); err != nil {
		t.Fatal(err)
	}
	waitControl(t, vc, "unavailable")
	if vc.Request() == nil || hc.InputAllowed(InputEvent{}) {
		t.Fatal("failed control stream retained input")
	}
	select {
	case <-client.Closed():
	case <-time.After(time.Second):
		t.Fatal("failed control stream did not terminate stale peer")
	}
}
