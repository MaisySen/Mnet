//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"sync"
	"testing"
	"time"
)

type operation1FakeSource struct {
	l                *heldPacketLedger
	p                Plan
	d                ownedSourceDiagnostic
	sends, reads     int
	entered, release chan struct{}
	after            func()
}

func (f *operation1FakeSource) diagnoseOperation1(context.Context) (ownedSourceDiagnostic, error) {
	held, e := heldSourceSnapshot(f.l, f.d.held.session.owner, f.d.held.session)
	if e != nil {
		return ownedSourceDiagnostic{}, e
	}
	return diagnosticForHeldRecords(f.d.raw, f.d.sys, held)
}
func (f *operation1FakeSource) applyOperation1(ctx context.Context, p Plan) (*firstStepCompletion, error) {
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	result, e := orchestrateFirstStepResult(ctx, f.l, p, f.d, 33, 77, firstStepDependencies{
		send: func(ctx context.Context, t *packetSetupTransition, permit *firstLinkSendPermit) (Datagram, error) {
			e := permit.send(t.request, 77, func() error { f.sends++; return nil })
			return setupACKTest(t), e
		},
		collect: func(ctx context.Context, t *packetSetupTransition) (setupReadbackRecord, error) {
			f.reads++
			return (&setupMemoryWire{token: t}).setupReadback(ctx)
		},
	})
	if e == nil {
		f.d.raw = cloneRawExact(result.post.raw)
		f.d.sys = cloneSysExact(result.post.sys)
		if f.after != nil {
			f.after()
		}
	}
	return result, e
}
func operation1AdapterRig(t *testing.T) (*operation1Controller, *operation1FakeSource) {
	l, p, d := firstStepRig(t)
	c := &operation1Controller{tx: d.held.owner}
	source := &operation1FakeSource{l: l, p: p, d: d}
	a := &operation1Adapter{controller: c, session: d.held.session, source: source}
	c.adapter = a
	c.tx.k = a
	c.tx.PendingCleanup = true
	return c, source
}
func TestOperation1AdapterOneCompletionJoinsCanonicalWithoutInventory(t *testing.T) {
	c, f := operation1AdapterRig(t)
	h := *c.adapter.session.canonical
	if e := c.adapter.Apply(context.Background(), h, f.p.Operations[1]); e == nil || f.sends != 0 {
		t.Fatal("unleased caller")
	}
	e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.adapter.Apply(ctx, h, f.p.Operations[1]) })
	if e != nil || f.sends != 1 || f.reads != 1 || c.adapter.session.forwardStep != 2 || !c.adapter.session.canonical.TagConfirmed {
		t.Fatal("join", e, f.sends, f.reads)
	}
	for _, op := range f.p.Operations[1:] {
		e = c.withOwner(context.Background(), func(ctx context.Context) error { return c.adapter.Apply(ctx, h, op) })
		if e == nil || f.sends != 1 {
			t.Fatal("replay/later op", op, e, f.sends)
		}
	}
	e = c.withOwner(context.Background(), func(ctx context.Context) error { _, e := c.adapter.Snapshot(ctx); return e })
	if e != ownedAFPolicyIncomplete || c.tx.Ready || !c.tx.PendingCleanup {
		t.Fatal("diagnostic bypass", e)
	}
}
func TestOperation1OwnershipLaneSerializesStopAndState(t *testing.T) {
	c, f := operation1AdapterRig(t)
	f.entered = make(chan struct{})
	f.release = make(chan struct{})
	h := *c.adapter.session.canonical
	var wg sync.WaitGroup
	wg.Add(3)
	applyErr := make(chan error, 1)
	stopErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		applyErr <- c.withOwner(context.Background(), func(ctx context.Context) error { return c.adapter.Apply(ctx, h, f.p.Operations[1]) })
	}()
	<-f.entered
	// Mark arrival immediately before attempting the owner lock, then require
	// BOTH operations remain blocked for a bounded interval while Apply owns it.
	stopArrived, stateArrived := make(chan struct{}), make(chan struct{})
	stopReturned, stateReturned := make(chan struct{}), make(chan struct{})
	go func() {
		defer wg.Done()
		close(stopArrived)
		stopErr <- c.stop(context.Background())
		close(stopReturned)
	}()
	go func() { defer wg.Done(); close(stateArrived); _ = c.state(); close(stateReturned) }()
	for _, arrived := range []<-chan struct{}{stopArrived, stateArrived} {
		select {
		case <-arrived:
		case <-time.After(time.Second):
			close(f.release)
			wg.Wait()
			t.Fatal("contender did not reach pre-lock arrival")
		}
	}
	timer := time.NewTimer(30 * time.Millisecond)
	select {
	case <-stopReturned:
		timer.Stop()
		close(f.release)
		wg.Wait()
		t.Fatal("stop returned while Apply held lane")
	case <-stateReturned:
		timer.Stop()
		close(f.release)
		wg.Wait()
		t.Fatal("state returned while Apply held lane")
	case <-timer.C:
	}
	close(f.release)
	wg.Wait()
	if e := <-applyErr; e != nil {
		t.Fatal(e)
	}
	if e := <-stopErr; e != ownedAFPolicyIncomplete {
		t.Fatal("held Snapshot gate", e)
	}
	state := c.state()
	if state.Ready || !state.Stopped || !state.PendingCleanup || state.Step != 2 || f.sends != 1 {
		t.Fatal("stop/custody", state, f.sends)
	}
}

// This shim is explicitly FAKE canonical Inventory, not an implementation of
// the held source Snapshot conversion or real creation. It drives Transaction
// control flow only. Production operation1Adapter has neither capability.
type operation1StartFixture struct {
	*operation1Adapter
	before  Inventory
	created bool
	handle  Handle
}

func (f *operation1StartFixture) Snapshot(ctx context.Context) (Inventory, error) {
	if !f.authorized(ctx) {
		return Inventory{}, ErrConflict
	}
	out := cloneInventory(f.before)
	if f.created {
		h := f.handle
		l := h.CreationLink
		if f.session.canonical.TagConfirmed {
			l = h.Link
		}
		out.Links = append(out.Links, l)
	}
	return out, nil
}
func (f *operation1StartFixture) Create(ctx context.Context, p Plan) (Handle, error) {
	if !f.authorized(ctx) || f.created {
		return Handle{}, ErrConflict
	}
	f.created = true
	return f.handle, nil
}
func TestTransactionStartFakeCanonicalSnapshotStopsAtOperation2(t *testing.T) {
	c, source := operation1AdapterRig(t)
	s := c.adapter.session
	before, e := s.baseline.Compose(s.baseline.raw, SysctlSnapshot(s.baseline.sys), nil, false)
	if e != nil {
		t.Fatal(e)
	}
	fixture := &operation1StartFixture{operation1Adapter: c.adapter, before: before, handle: *s.canonical}
	// Fresh transaction control flow over an already-modeled fake creation record.
	// No original FD constructor/source helper or actual Snapshot promotion.
	tx, e := NewTransaction(fixture, c.tx.clock, source.p.Binding)
	if e != nil {
		t.Fatal(e)
	}
	c.tx = tx
	s.owner = tx
	held, e := heldSourceSnapshot(source.l, tx, s)
	if e != nil {
		t.Fatal(e)
	}
	source.d, e = diagnosticForHeldRecords(source.d.raw, source.d.sys, held)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.start(context.Background()); e == nil {
		t.Fatal("later op accepted")
	}
	state := c.state()
	if state.Ready || !state.PendingCleanup || state.Step != 2 || state.Handle == nil || !state.Handle.TagConfirmed || source.sends != 1 || source.reads != 1 {
		t.Fatal("fake Start join", state, source.sends, source.reads)
	}
}

func TestOperation1CompletionGapCannotAdvanceSession(t *testing.T) {
	for _, mode := range []string{"close", "invalidate", "cancel", "expired"} {
		t.Run(mode, func(t *testing.T) {
			c, f := operation1AdapterRig(t)
			h := *c.adapter.session.canonical
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.after = func() {
				switch mode {
				case "close":
					_ = f.l.invalidate("close-intent")
				case "invalidate":
					_ = f.l.invalidate("config-intent")
				case "cancel":
					cancel()
				case "expired":
					c.tx.clock.(*fakeClock).s.Monotonic = c.tx.deadline.Mono
				}
			}
			if e := c.withOwner(ctx, func(ctx context.Context) error { return c.adapter.Apply(ctx, h, f.p.Operations[1]) }); e == nil {
				t.Fatal("gap accepted")
			}
			state := c.state()
			if state.Ready || !state.PendingCleanup || state.Step != 1 || c.adapter.session.canonical.TagConfirmed || !c.adapter.session.poisoned || !f.l.snapshot().uncertain || f.sends != 1 {
				t.Fatal("gap advanced", state)
			}
		})
	}
}
