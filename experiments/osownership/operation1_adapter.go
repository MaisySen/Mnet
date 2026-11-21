//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// Exclusive ownership lane, NOT the packet ledger mutex. All supported entry
// points into the owned transaction/session/clock run while this mutex is held.
// No transaction/session pointers or mutable clock escape this controller.
// Existing free-standing inert Transaction API is not made thread-safe by this.
type operation1Controller struct {
	mu      sync.Mutex
	tx      *Transaction
	adapter *operation1Adapter
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
	return c.withOwner(ctx, func(ctx context.Context) error { return c.tx.Start(ctx) })
}
func (c *operation1Controller) stop(ctx context.Context) error {
	return c.withOwner(ctx, func(ctx context.Context) error { return c.tx.Stop(ctx) })
}

type operation1State struct {
	Ready, PendingCleanup, Stopped bool
	Step                           int
	Handle                         *Handle
}

func (c *operation1Controller) state() operation1State {
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
type operation1Adapter struct {
	controller *operation1Controller
	session    *adapterSession
	source     operation1DiagnosticSource
}

var _ transactionLedgerAdapter = (*operation1Adapter)(nil)

func (a *operation1Adapter) ownershipSession() *adapterSession {
	if a == nil {
		return nil
	}
	return a.session
}
func (a *operation1Adapter) Snapshot(ctx context.Context) (Inventory, error) {
	if !a.authorized(ctx) || a.source == nil {
		return Inventory{}, ErrConflict
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
func (a *operation1Adapter) Create(ctx context.Context, _ Plan) (Handle, error) {
	return Handle{}, ErrExecutorUnreleased
}
func (a *operation1Adapter) CloseOriginalFD(Handle) error { return ErrExecutorUnreleased }
func (a *operation1Adapter) Apply(ctx context.Context, h Handle, op Operation) (err error) {
	if !a.authorized(ctx) || a.source == nil || a.session == nil || a.session.plan == nil || op.Kind != "set-alias-mtu-exact" || op != a.session.plan.Operations[1] {
		return ErrConflict
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
