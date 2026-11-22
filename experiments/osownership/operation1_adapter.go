//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// Exclusive ownership lane, NOT the packet ledger mutex. All supported entry
// points into the owned transaction/session/clock run while this mutex is held.
// No transaction/session pointers or mutable clock escape this controller.
// Existing free-standing inert Transaction API is not made thread-safe by this.
type ownershipLaneSource interface{ ownerThreadCurrent() bool }
type operation1Controller struct {
	// Non-nil only for the actual inherited endpoint controller. No caller flag.
	endpointCleanup       *endpointOriginalCleanupResult
	lane                  ownershipLaneSource
	precreationSequence   uint64
	precreationAdmitted   bool
	precreationGeneration uint64
	mu                    sync.Mutex
	tx                    *Transaction
	adapter               *operation1Adapter
	attempt               *originalAttempt
	absence               originalAbsenceSource
}
type operation1Lease struct {
	owner  *operation1Controller
	active atomic.Bool
}
type operation1ContextKey struct{}

func (c *operation1Controller) withOwner(ctx context.Context, fn func(context.Context) error) error {
	if c == nil || ctx == nil || fn == nil {
		return ErrConflict
	}
	if c.lane != nil && !c.lane.ownerThreadCurrent() {
		return ErrConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	lease := &operation1Lease{owner: c}
	lease.active.Store(true)
	defer lease.active.Store(false)
	return fn(context.WithValue(ctx, operation1ContextKey{}, lease))
}
func (c *operation1Controller) start(ctx context.Context) error {
	if c != nil && c.lane != nil && !c.lane.ownerThreadCurrent() {
		return ErrConflict
	}
	e := c.withOwner(ctx, func(ctx context.Context) error { return c.tx.Start(ctx) })
	if e == nil || c == nil {
		return e
	}
	// Even a canceled context rejected before entering Start cannot skip an
	// already-recorded original. Serialize finalization independently of ctx.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attempt != nil {
		return errors.Join(e, c.stopOwnedLocked(ctx))
	}
	return e
}
func (c *operation1Controller) stop(ctx context.Context) error {
	// Cancellation while waiting must not skip original close. Do not call the
	// withOwner early-context refusal used for forward setup.
	if c == nil || (c.lane != nil && !c.lane.ownerThreadCurrent()) {
		return ErrConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopOwnedLocked(ctx)
}

type operation1State struct {
	Ready, PendingCleanup, Stopped bool
	Step                           int
	Handle                         *Handle
}

func (c *operation1Controller) state() operation1State {
	if c == nil || (c.lane != nil && !c.lane.ownerThreadCurrent()) {
		return operation1State{PendingCleanup: true}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := operation1State{Ready: c.tx.Ready, PendingCleanup: c.tx.PendingCleanup, Stopped: c.tx.stopStarted, Step: c.adapter.session.forwardStep}
	if c.tx.handle != nil {
		h := *c.tx.handle
		s.Handle = &h
	}
	return s
}
func (a *operation1Adapter) authorized(ctx context.Context) bool {
	if a == nil || a.controller == nil || ctx == nil {
		return false
	}
	lease, ok := ctx.Value(operation1ContextKey{}).(*operation1Lease)
	return ok && lease.owner == a.controller && lease.active.Load()
}

// The production clock has no externally mutable backing object or setter.
// Reads only occur on the ownership lane; elapsed keeps monotonic time.
type operation1Clock struct{ origin time.Time }

func (c operation1Clock) Now() Stamp { return Stamp{Wall: time.Now(), Monotonic: time.Since(c.origin)} }

// Typed diagnostic boundary: deliberately no Inventory conversion method.
// Live supplier returns only original-custody-correlated source diagnostics.
type operation1DiagnosticSource interface {
	diagnoseOperation1(context.Context) (ownedSourceDiagnostic, error)
	applyOperation1(context.Context, Plan) (*firstStepCompletion, error)
}

// Plan zero-based index2 is the third operation overall (create index0,
// alias/MTU index1). forwardStep2 means next op is disable, not op index3.
type operationDisableSource interface {
	applyOwnedDisable(context.Context) (*postDisableResult, error)
}
type operationAddressSource interface {
	applyOwnedAddress(context.Context, *postDisableResult) (*postAddressDownResult, error)
}
type operationUpSource interface {
	applyOwnedUp(context.Context, *postAddressDownResult) (*postUpResult, error)
}
type operationPeerRouteSource interface {
	applyOwnedPeerRoute(context.Context, *postUpResult) (*postPeerRouteResult, error)
}
type operation1Adapter struct {
	peerRouteAttempted bool
	routed             *postPeerRouteResult
	phaseCreation      bool
	phaseSequence      uint64
	phaseAdmitted      int
	phaseIdentity      heldPacketIdentity
	aliasPost          *firstStepCompletion
	disableAttempted   bool // controller lane, one attempt even on source error
	disabled           *postDisableResult
	addressAttempted   bool
	addressed          *postAddressDownResult
	upAttempted        bool
	upped              *postUpResult
	controller         *operation1Controller
	session            *adapterSession
	source             operation1DiagnosticSource
	creation           operation1CreationSource
}

var _ transactionLedgerAdapter = (*operation1Adapter)(nil)

func (a *operation1Adapter) ownershipSession() *adapterSession {
	if a == nil {
		return nil
	}
	return a.session
}
func (a *operation1Adapter) Snapshot(ctx context.Context) (Inventory, error) {
	if !a.authorized(ctx) {
		return Inventory{}, ErrConflict
	}
	if a.controller.attempt == nil && a.session.canonical == nil && !a.session.createAttempted {
		a.controller.precreationAdmitted = false
		supplier, ok := a.creation.(operation1PrecreationSource)
		if !ok {
			return Inventory{}, ErrConflict
		}
		observation, e := supplier.observePrecreation(ctx)
		if e != nil {
			return Inventory{}, e
		}
		return consumePrecreation(a.controller, observation)
	}
	if a.source == nil {
		return Inventory{}, ErrConflict
	}
	if source, ok := a.source.(setupPhaseSource); ok {
		a.phaseAdmitted = 0
		return a.snapshotSetupPhase(ctx, source)
	}
	diagnostic, e := a.source.diagnoseOperation1(ctx)
	if e != nil {
		return Inventory{}, e
	}
	if diagnostic.held == nil || diagnostic.held.owner != a.controller.tx || diagnostic.held.session != a.session || !diagnostic.held.ledger.matches(diagnostic.held.identity) {
		return Inventory{}, ErrConflict
	}
	if _, e = sourceDiagnosticFromRecords(diagnostic.raw, diagnostic.sys, *a.session.canonical, diagnostic.lineageUsable); e != nil {
		return Inventory{}, e
	}
	// No .inventory field extraction, no fabricated afReadPass, no active gate
	// bypass. Canonical diagnostic-to-Inventory policy is still incomplete.
	return Inventory{}, ownedAFPolicyIncomplete
}

// Private creation dependency never returns an untracked raw descriptor.
type operation1CreationSource interface {
	createOriginal(context.Context, Plan, *originalAttempt) (Handle, error)
	bindCreatedOriginal(context.Context, *originalAttempt) (operation1DiagnosticSource, error)
}

func (a *operation1Adapter) Create(ctx context.Context, p Plan) (Handle, error) {
	if !a.authorized(ctx) || a.creation == nil || a.controller.attempt != nil || a.session == nil || a.session.createAttempted {
		return Handle{}, ErrConflict
	}
	c := a.controller
	if _, ok := a.creation.(setupPhaseCreationSource); ok {
		if !c.precreationAdmitted {
			return Handle{}, afPhase
		}
		c.precreationAdmitted = false
		a.phaseCreation = true
	}
	frozen := p
	frozen.Operations = append([]Operation(nil), p.Operations...)
	c.attempt = &originalAttempt{owner: c, plan: frozen, stage: "unopened"}
	c.tx.PendingCleanup = true
	a.session.createAttempted = true
	h, e := a.creation.createOriginal(ctx, p, c.attempt)
	if e != nil {
		a.session.poisoned = true
		return h, c.attempt.fail(e)
	}
	if e = ctx.Err(); e != nil {
		a.session.poisoned = true
		return h, c.attempt.fail(e)
	}
	if c.attempt.custody == nil || c.attempt.handle == nil || *c.attempt.handle != h {
		a.session.poisoned = true
		return h, c.attempt.fail(ErrConflict)
	}
	if e = a.session.bindCreated(p, h); e != nil {
		a.session.poisoned = true
		copy := h
		a.session.rejectedHandle = &copy
		return h, c.attempt.fail(e)
	}
	source, e := a.creation.bindCreatedOriginal(ctx, c.attempt)
	if e != nil || source == nil {
		a.session.poisoned = true
		return h, c.attempt.fail(errors.Join(e, ErrConflict))
	}
	a.source = source
	c.attempt.stage = "session-bound"
	return h, nil
}
func (a *operation1Adapter) CloseOriginalFD(Handle) error { return ErrExecutorUnreleased }
func (a *operation1Adapter) Apply(ctx context.Context, h Handle, op Operation) (err error) {
	if !a.authorized(ctx) || a.source == nil || a.session == nil || a.session.plan == nil || len(a.session.plan.Operations) != 6 {
		return ErrConflict
	}
	if op.Kind == "route-add-exclusive" {
		return a.applyPeerRoute(ctx, h, op)
	}
	if op.Kind == "link-up-exact" {
		return a.applyUp(ctx, h, op)
	}
	if op.Kind == "address-add-exclusive" {
		return a.applyAddress(ctx, h, op)
	}
	if op.Kind == "disable-owned-link-ipv6" {
		return a.applyDisable(ctx, h, op)
	}
	if op.Kind != "set-alias-mtu-exact" || op != a.session.plan.Operations[1] {
		return ErrConflict
	}
	if _, ok := a.source.(setupPhaseSource); ok {
		if !a.setupAdmissionCurrent(1) {
			return afPhase
		}
		a.phaseAdmitted = 0
	}
	if e := a.session.begin(h, op); e != nil {
		return e
	}
	completion, e := a.source.applyOperation1(ctx, *a.session.plan)
	defer func() {
		if err != nil && completion != nil && completion.token != nil {
			completion.token.abandon()
			_ = completion.token.ledger.invalidate("custody-loss")
		}
	}()
	if e != nil {
		return a.session.recordACK(e)
	}
	if e = ctx.Err(); e != nil {
		return a.session.recordACK(e)
	}
	if e = a.session.joinFirstStep(completion, a.controller.tx); e != nil {
		a.session.poisoned = true
		return e
	}
	a.aliasPost = completion
	return nil
}

// Joins ONLY already-finished exact first-step evidence. No new send, new
// Inventory, generic observed flags or externally asserted success boolean.
func (s *adapterSession) joinFirstStep(c *firstStepCompletion, owner *Transaction) error {
	if c == nil || c.consumed || c.token == nil || c.ack == nil || c.post == nil {
		return ErrConflict
	}
	c.consumed = true
	t := c.token
	if s == nil || owner == nil || s.owner != owner || t.owner != owner || s.canonical == nil || s.plan == nil || s.forwardStep != 1 || s.pending == nil || *s.pending != t.operation || !reflect.DeepEqual(*s.plan, t.plan) || !sameSessionHandle(*s.canonical, t.original.handle) || s.poisoned || s.closeAttempted || s.cleanupMode || owner.stopStarted || owner.fdCloseAttempted || owner.clock == nil || owner.deadline.expired(owner.clock.Now()) || !t.done || c.ack.transition != t || c.post.transition != t {
		return ErrConflict
	}
	l := t.ledger
	if l == nil {
		return ErrConflict
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	expected := c.post.identity
	expected.generation++
	expected.uncertain = false
	expected.handle.TagConfirmed = true
	if l.setup != nil || l.current != expected || l.current.closed || l.current.uncertain {
		return ErrConflict
	}
	if e := s.recordACK(nil); e != nil {
		return e
	}
	// Typed post was already parsed against exact expected raw/sys/features and
	// finish checked the same ACK. This advances only op1 canonical bookkeeping.
	s.canonical.TagConfirmed = true
	s.forwardStep = 2
	s.pending = nil
	s.pendingACK = false
	return nil
}

// No generic mutation dispatch. Index2 is disable; index3 has its separate
// typed address path. UP(index4) and peer-route(index5) have distinct typed paths.
func (a *operation1Adapter) applyDisable(ctx context.Context, h Handle, op Operation) (err error) {
	s := a.session
	if !a.authorized(ctx) || a.disableAttempted || s == nil || s.plan == nil || len(s.plan.Operations) != 6 || op != s.plan.Operations[2] || s.forwardStep != 2 || s.canonical == nil || !s.canonical.TagConfirmed || !sameSessionHandle(h, *s.canonical) || s.pending != nil || s.poisoned || s.cleanupMode || s.closeAttempted || a.controller.tx.stopStarted || a.controller.tx.fdCloseAttempted {
		return ErrConflict
	}
	source, ok := a.source.(operationDisableSource)
	if !ok {
		return ErrConflict
	}
	if _, ok := a.source.(setupPhaseSource); ok {
		if !a.setupAdmissionCurrent(2) {
			return afPhase
		}
		a.phaseAdmitted = 0
	}
	a.disableAttempted = true
	defer func() {
		if err != nil {
			s.poisoned = true
		}
	}()
	result, e := source.applyOwnedDisable(ctx)
	defer func() {
		if err != nil && result != nil {
			result.lifetime.revoke()
			if result.ledger != nil {
				_ = result.ledger.invalidate("custody-loss")
			}
		}
	}()
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if a.controller.tx.clock == nil || a.controller.tx.deadline.expired(a.controller.tx.clock.Now()) {
		return ErrConflict
	}
	if result == nil || result.consumed || result.owner != a.controller.tx || result.session != s || !result.current() || !sameSessionHandle(result.identity.handle, *s.canonical) {
		return ErrConflict
	}
	result.consumed = true
	a.disabled = result
	s.forwardStep = 3
	return nil
}

// Address advances only the reviewed isolated-target DOWN construction.
// All failures poison the session and retain owned cleanup.
func (a *operation1Adapter) applyAddress(ctx context.Context, h Handle, op Operation) (err error) {
	s := a.session
	if !a.authorized(ctx) || a.addressAttempted || s == nil || s.plan == nil || len(s.plan.Operations) != 6 || op != s.plan.Operations[3] || s.forwardStep != 3 || s.canonical == nil || !sameSessionHandle(h, *s.canonical) || s.pending != nil || s.poisoned || s.cleanupMode || s.closeAttempted || a.disabled == nil || !a.disabled.current() || !a.disabled.consumed {
		return afPhase
	}
	source, ok := a.source.(operationAddressSource)
	if !ok {
		return afPhase
	}
	if _, ok := a.source.(setupPhaseSource); ok {
		if !a.setupAdmissionCurrent(3) {
			return afPhase
		}
		a.phaseAdmitted = 0
	}
	a.addressAttempted = true
	var r *postAddressDownResult
	defer func() {
		if err != nil {
			s.poisoned = true
			a.disabled.lifetime.revoke()
			if r != nil && r.ledger != nil {
				_ = r.ledger.invalidate("custody-loss")
			}
		}
	}()
	r, err = source.applyOwnedAddress(ctx, a.disabled)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if r == nil || r.consumed || r.disabledAncestry != a.disabled || r.ledger != a.disabled.ledger || !r.current() || !sameSessionHandle(r.identity.handle, *s.canonical) || a.controller.tx.stopStarted || a.controller.tx.fdCloseAttempted || a.controller.tx.deadline.expired(a.controller.tx.clock.Now()) {
		return afPhase
	}
	r.consumed = true
	a.addressed = r
	s.forwardStep = 4
	return nil
}

func (a *operation1Adapter) applyUp(ctx context.Context, h Handle, op Operation) (err error) {
	s := a.session
	if !a.authorized(ctx) || a.upAttempted || s == nil || s.plan == nil || len(s.plan.Operations) != 6 || op != s.plan.Operations[4] || s.forwardStep != 4 || s.canonical == nil || !sameSessionHandle(h, *s.canonical) || s.pending != nil || s.poisoned || s.cleanupMode || s.closeAttempted || a.addressed == nil || !a.addressed.current() || !a.addressed.consumed {
		return afPhase
	}
	source, ok := a.source.(operationUpSource)
	if !ok {
		return afPhase
	}
	if _, ok := a.source.(setupPhaseSource); ok {
		if !a.setupAdmissionCurrent(4) {
			return afPhase
		}
		a.phaseAdmitted = 0
	}
	a.upAttempted = true
	var r *postUpResult
	defer func() {
		if err != nil {
			s.poisoned = true
			a.addressed.lifetime.revoke()
			if r != nil && r.ledger != nil {
				_ = r.ledger.invalidate("custody-loss")
			}
		}
	}()
	r, err = source.applyOwnedUp(ctx, a.addressed)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if r == nil || r.consumed || r.ancestry != a.addressed || r.ledger != a.addressed.ledger || !r.current() || !sameSessionHandle(r.identity.handle, *s.canonical) || a.controller.tx.stopStarted || a.controller.tx.fdCloseAttempted || a.controller.tx.deadline.expired(a.controller.tx.clock.Now()) {
		return afPhase
	}
	r.consumed = true
	a.upped = r
	s.forwardStep = 5
	return nil
}

func (a *operation1Adapter) applyPeerRoute(ctx context.Context, h Handle, op Operation) (err error) {
	s := a.session
	if !a.authorized(ctx) || a.peerRouteAttempted || s == nil || s.plan == nil || len(s.plan.Operations) != 6 || op != s.plan.Operations[5] || s.forwardStep != 5 || s.canonical == nil || !sameSessionHandle(h, *s.canonical) || s.pending != nil || s.poisoned || s.cleanupMode || s.closeAttempted || a.upped == nil || !a.upped.current() || !a.upped.consumed {
		return afPhase
	}
	source, ok := a.source.(operationPeerRouteSource)
	if !ok {
		return afPhase
	}
	if _, ok := a.source.(setupPhaseSource); ok {
		if !a.setupAdmissionCurrent(5) {
			return afPhase
		}
		a.phaseAdmitted = 0
	}
	a.peerRouteAttempted = true
	var r *postPeerRouteResult
	defer func() {
		if err != nil {
			s.poisoned = true
			a.upped.lifetime.revoke()
			if r != nil && r.ledger != nil {
				_ = r.ledger.invalidate("custody-loss")
			}
		}
	}()
	r, err = source.applyOwnedPeerRoute(ctx, a.upped)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if r == nil || r.consumed || r.ancestry != a.upped || r.ledger != a.upped.ledger || !r.current() || !sameSessionHandle(r.identity.handle, *s.canonical) || a.controller.tx.stopStarted || a.controller.tx.fdCloseAttempted || a.controller.tx.deadline.expired(a.controller.tx.clock.Now()) {
		return afPhase
	}
	r.consumed = true
	a.routed = r
	s.forwardStep = 6
	return nil
}
