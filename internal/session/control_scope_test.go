package session

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHostPointerScopeRejectsKeyAndHoldAfterLocalApproval(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	requests := make(chan ControlStatus, 4)
	hc := NewHostControlWithPointerScope(func(s ControlStatus) { requests <- s })
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	available := waitControl(t, vc, "available")
	token := controlToken()
	vc.mu.Lock()
	vc.status.Request = token
	vc.status.State = "pending"
	vc.mu.Unlock()
	if err := client.controlJSON(vc.stream, map[string]any{"kind": controlKind, "session": available.Session, "request": token, "state": "request", "scope": "pointer-v1"}); err != nil {
		t.Fatal(err)
	}
	var request ControlStatus
	select {
	case request = <-requests:
	case <-time.After(time.Second):
		t.Fatal("no local request")
	}
	if err := hc.Decide(request.Session, request.Request, true); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []InputEvent{
		{Kind: "key", Action: "down", Text: "a"},
		{Kind: "pointer", Action: "down", Button: "left"},
		{Kind: "pointer", Action: "up", Button: "left"},
		{Kind: "pointer", Action: "click", Button: "left", Text: "hidden key"},
	} {
		ev.ControlSession = request.Session
		ev.ControlRequest = request.Request
		if hc.InputAllowed(ev) {
			t.Fatalf("pointer approval accepted broader input: %+v", ev)
		}
	}
	click := InputEvent{Kind: "pointer", Action: "click", Button: "left", X: 2, Y: 2, ControlSession: request.Session, ControlRequest: request.Request}
	if !hc.InputAllowed(click) {
		t.Fatal("approved atomic pointer click refused")
	}
}

func TestPointerScopeNegotiatesAndHostDropsBroaderRawInput(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	requests := make(chan ControlStatus, 4)
	hc := NewHostControlWithPointerScope(func(s ControlStatus) { requests <- s })
	vc := NewViewerControl(nil)
	v := ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	inj := &authorizationInjector{}
	h := ServeHostWithOptions(host, &authorizationCapturer{}, inj, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	if !waitControl(t, vc, "available").PointerV1 {
		t.Fatal("host did not advertise narrow permission")
	}
	if err := vc.RequestPointer(); err != nil {
		t.Fatal(err)
	}
	request := <-requests
	if request.Scope != ControlScopePointer {
		t.Fatal("local decision lost scope", request)
	}
	if err := hc.Decide(request.Session, request.Request, true); err != nil {
		t.Fatal(err)
	}
	grant := waitControl(t, vc, "allowed")
	if grant.Scope != ControlScopePointer {
		t.Fatal("grant broadened", grant)
	}
	for _, ev := range []InputEvent{{Kind: "key", Action: "down", Text: "a"}, {Kind: "pointer", Action: "down", Button: "left"}, {Kind: "pointer", Action: "scroll", DY: 1000}, {Kind: "pointer", Action: "click", Button: "other"}} {
		if v.SendInputFor(grant, ev) == nil {
			t.Fatal("viewer accepted broader input", ev)
		}
		ev.ControlSession = grant.Session
		ev.ControlRequest = grant.Request
		if err := client.SendJSON(StreamInput, PriInteractive, ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.SendInputFor(grant, InputEvent{Kind: "pointer", Action: "click", Button: "left", X: 1, Y: 1}); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	for inj.calls.Load() < 1 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if inj.calls.Load() != 1 {
		t.Fatal("host injected broader raw input or lost ordered click", inj.calls.Load())
	}
	if err := hc.Pause(grant.Session, grant.Request); err != nil {
		t.Fatal(err)
	}
	paused := waitControl(t, vc, "paused")
	if paused.Scope != ControlScopePointer {
		t.Fatal("pause lost scope")
	}
	if err := hc.Decide(grant.Session, grant.Request, true); err != nil {
		t.Fatal(err)
	}
	resumed := waitControl(t, vc, "allowed")
	if resumed.Scope != ControlScopePointer || resumed.Revision <= grant.Revision {
		t.Fatal("resume lost scope/revision")
	}
	if v.SendInputFor(grant, InputEvent{Kind: "pointer", Action: "click", Button: "left"}) == nil {
		t.Fatal("pre-pause pointer event rebound after resume")
	}
	if v.SendInputFor(resumed, InputEvent{Kind: "key", Action: "down", Text: "a"}) == nil {
		t.Fatal("resume broadened pointer permission")
	}
	if err := vc.Withdraw(); err != nil {
		t.Fatal(err)
	}
	waitControl(t, vc, "declined")
	select {
	case <-requests:
	case <-time.After(time.Second):
		t.Fatal("host did not observe withdrawal")
	}
	if hc.InputAllowed(InputEvent{Kind: "pointer", Action: "click", Button: "left", ControlSession: grant.Session, ControlRequest: grant.Request}) {
		t.Fatal("withdrawal retained permission")
	}
}

func TestPointerRequestRefusesOldHostAndNeverDowngrades(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	token := controlToken()
	stream, err := host.open(PriInteractive, controlMessage{Kind: controlKind, Session: token}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.controlJSON(stream.ID, controlMessage{Kind: controlKind, Session: token, State: "available", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	available := waitControl(t, vc, "available")
	if available.PointerV1 || vc.RequestPointer() == nil || vc.Snapshot() != available {
		t.Fatal("old peer got broader request", vc.Snapshot())
	}
	select {
	case <-client.Closed():
		t.Fatal("unsupported scope ended view")
	default:
	}
	// Explicit generic control remains available for the existing engineering API.
	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var message controlMessage
	if err = json.Unmarshal(frame.Payload, &message); err != nil || message.Scope != "" || message.State != "request" {
		t.Fatal("generic request changed", message, err)
	}
}

func TestPointerGrantCannotBeBroadenedOrRelabeled(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	token := controlToken()
	stream, err := host.open(PriInteractive, controlMessage{Kind: controlKind, Session: token, PointerV1: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	emit := func(msg controlMessage) {
		t.Helper()
		if err := host.controlJSON(stream.ID, msg); err != nil {
			t.Fatal(err)
		}
	}
	emit(controlMessage{Kind: controlKind, Session: token, State: "available", Revision: 1})
	waitControl(t, vc, "available")
	if err := vc.RequestPointer(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(time.Second); err != nil {
		t.Fatal(err)
	}
	pending := vc.Snapshot()
	// Queue a broader grant before a correctly scoped one. The revision check and
	// scope check must reject it, not accept its revision or relabel the request.
	emit(controlMessage{Kind: controlKind, Session: token, Request: pending.Request, State: "allowed", Revision: 999})
	emit(controlMessage{Kind: controlKind, Session: token, Request: pending.Request, Scope: ControlScopePointer, State: "allowed", Revision: 2})
	grant := waitControl(t, vc, "allowed")
	if grant.Scope != ControlScopePointer || grant.Revision != 2 {
		t.Fatal("broader grant accepted", grant)
	}
	if err := vc.Withdraw(); err != nil {
		t.Fatal(err)
	}
	emit(controlMessage{Kind: controlKind, Session: token, Request: pending.Request, Scope: ControlScopePointer, State: "allowed", Revision: 1000})
	emit(controlMessage{Kind: controlKind, Session: token, Request: pending.Request, Scope: ControlScopePointer, State: "declined", Revision: 1001})
	// Host receipt of the cancel then an explicit response fences both queued frames.
	if _, err := stream.Recv(time.Second); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	for vc.Snapshot().Revision != 1001 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if got := vc.Snapshot(); got.State != "declined" || got.Revision != 1001 {
		t.Fatal("late scoped grant revived request", got)
	}
}

func TestHostUnknownScopeNeverBecomesGenericApproval(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	requests := make(chan ControlStatus, 4)
	hc := NewHostControlWithPointerScope(func(s ControlStatus) { requests <- s })
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	available := waitControl(t, vc, "available")
	bad, good := controlToken(), controlToken()
	for _, msg := range []controlMessage{{Kind: controlKind, Session: available.Session, Request: bad, State: "request", Scope: "unknown-scope"}, {Kind: controlKind, Session: available.Session, Request: good, State: "request"}} {
		if err := client.controlJSON(vc.stream, msg); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case request := <-requests:
		if request.Request != good || request.Scope != "" {
			t.Fatal("unknown scope became a local generic request", request)
		}
	case <-time.After(time.Second):
		t.Fatal("following generic request missing")
	}
}

func TestLegacyGenericHostDoesNotOptIntoPointerTier(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	requests := make(chan ControlStatus, 4)
	hc := NewHostControl(func(s ControlStatus) { requests <- s })
	vc := NewViewerControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	available := waitControl(t, vc, "available")
	if available.PointerV1 || vc.RequestPointer() == nil {
		t.Fatal("generic host silently opted into narrow UI")
	}
	for _, msg := range []controlMessage{{Kind: controlKind, Session: available.Session, Request: controlToken(), State: "request", Scope: ControlScopePointer}, {Kind: controlKind, Session: available.Session, Request: controlToken(), State: "request"}} {
		if err := client.controlJSON(vc.stream, msg); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case request := <-requests:
		if request.Scope != "" {
			t.Fatal("generic-only host displayed pointer request", request)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy generic request lost")
	}
}
