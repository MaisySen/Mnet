//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"
)

// Kernel is an unimplemented executor boundary. Snapshot MUST inspect kernel
// state in the pinned namespace FD, including all route tables/types and policy
// rules, sysctls, address flags/origins and link IPv6 disable readback. No real
// implementation is wired. Fake tests do not establish those kernel facts.
type Kernel interface {
	Snapshot(context.Context) (Inventory, error)
	// Create must return a custody handle for its exclusive nonpersistent FD even
	// on an error after creation. If no handle can be returned, Create MUST close
	// its original FD itself before returning. Uncertain kernel state stays pending.
	Create(context.Context, Plan) (Handle, error)
	Apply(context.Context, Handle, Operation) error
	CloseOriginalFD(Handle) error
}
type Stamp struct {
	Wall      time.Time
	Monotonic time.Duration
}
type Clock interface{ Now() Stamp }
type Deadline struct {
	Wall time.Time
	Mono time.Duration
}

func deadlineAt(start Stamp, absolute time.Time) (Deadline, error) {
	remaining := absolute.Sub(start.Wall)
	if remaining <= 0 {
		return Deadline{}, errors.New("original deadline ended")
	}
	return Deadline{Wall: absolute, Mono: start.Monotonic + remaining}, nil
}
func (d Deadline) expired(now Stamp) bool { return !now.Wall.Before(d.Wall) || now.Monotonic >= d.Mono }

type Evidence struct {
	Step     string
	ACK      bool
	Readback string
	Error    bool
}

// Transaction owns step state and its monotonic deadline independent of the
// caller's Plan value. API never extends it after an ACK or partial success.
type Transaction struct {
	k                Kernel
	clock            Clock
	plan             Plan
	deadline         Deadline
	handle           *Handle
	Evidence         []Evidence
	Ready            bool
	PendingCleanup   bool
	stopStarted      bool
	fdCloseAttempted bool
	fdCloseError     error
	baseline         *Inventory
}

func NewTransaction(k Kernel, c Clock, b Binding) (*Transaction, error) {
	if k == nil || c == nil {
		return nil, errors.New("kernel and owner clock required")
	}
	stamp := c.Now()
	end := b.SelfExpiry
	if b.PeerExpiry.Before(end) {
		end = b.PeerExpiry
	}
	d, e := deadlineAt(stamp, end)
	if e != nil {
		return nil, e
	}
	return &Transaction{k: k, clock: c, plan: Plan{Binding: b}, deadline: d}, nil
}
func (t *Transaction) live(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if t.deadline.expired(t.clock.Now()) {
		return errors.New("owner monotonic or absolute deadline ended")
	}
	return nil
}
func (t *Transaction) Start(ctx context.Context) error {
	stamp := t.clock.Now()
	remaining := t.deadline.Wall.Sub(stamp.Wall)
	if mono := t.deadline.Mono - stamp.Monotonic; mono < remaining {
		remaining = mono
	}
	if remaining <= 0 {
		return errors.New("owner deadline ended before start")
	}
	bounded, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	ctx = bounded
	if len(t.Evidence) != 0 || t.handle != nil {
		return errors.New("one-shot transaction already attempted")
	}
	if e := t.live(ctx); e != nil {
		return e
	}
	before, e := t.k.Snapshot(ctx)
	if e != nil {
		return e
	}
	p, e := Prepare(t.plan.Binding, before, t.clock.Now().Wall)
	if e != nil {
		return e
	}
	t.plan = p
	copyBaseline := cloneInventory(before)
	t.baseline = &copyBaseline
	t.Evidence = append(t.Evidence, Evidence{Step: "create-intent"})
	t.PendingCleanup = true
	h, createErr := t.k.Create(ctx, p)
	if h.Link.Index > 0 {
		t.handle = &h
		t.PendingCleanup = true
	}
	after, readErr := t.k.Snapshot(ctx)
	if readErr != nil {
		return errors.Join(createErr, readErr, errors.New("creation readback unavailable; pending custody retained"))
	}
	if t.handle == nil {
		return errors.Join(createErr, errors.New("no original FD custody; never adopt name-only link"))
	}
	if e = exactHandle(h, p, after); e != nil {
		return errors.Join(createErr, e)
	}
	t.Evidence = append(t.Evidence, Evidence{Step: "create", ACK: createErr == nil, Readback: "exact-link", Error: createErr != nil})
	if createErr != nil {
		return createErr
	}
	for _, op := range p.Operations[1:] {
		if e = t.live(ctx); e != nil {
			return e
		}
		t.Evidence = append(t.Evidence, Evidence{Step: op.Kind + "-intent"})
		callErr := t.k.Apply(ctx, h, op)
		snapshot, readErr := t.k.Snapshot(ctx)
		if readErr != nil {
			return errors.Join(callErr, readErr)
		}
		if e = exactHandle(h, p, snapshot); e != nil {
			return errors.Join(callErr, e)
		}
		verified := stepPresent(op, h, snapshot)
		t.Evidence = append(t.Evidence, Evidence{Step: op.Kind, ACK: callErr == nil, Readback: fmt.Sprint(verified), Error: callErr != nil})
		if callErr != nil {
			return callErr
		}
		if !verified {
			return errors.New("ACK without exact operation readback")
		}
		if op.Kind == "set-alias-mtu-exact" {
			h.TagConfirmed = true
			t.handle = &h
		}
		if e = t.live(ctx); e != nil {
			return e
		}
	}
	if source, ok := t.k.(*operation1Adapter); ok {
		if e := source.requireSourceCompletion(); e != nil {
			return e
		}
	}
	t.Ready = true
	return nil
}
func exactHandle(h Handle, p Plan, i Inventory) error {
	expected, e := Attach(p, h.Link.Index)
	if e != nil || h.Link != expected.Link || h.Namespace != expected.Namespace || h.Address != expected.Address || h.Route != expected.Route {
		return ErrConflict
	}
	if !sameNamespace(h.Namespace, i.Namespace) {
		return ErrConflict
	}
	if i.ForeignPolicy || i.Forwarding {
		return ErrConflict
	}
	for _, a := range i.Addresses {
		if a.LinkIndex == h.Link.Index {
			if a != h.Address && !kernelDerivedAddress(h, a) {
				return ErrConflict
			}
		} else if !a.Prefix.IsValid() || !a.Prefix.Addr().IsLoopback() {
			return ErrConflict
		}
	}
	for _, r := range i.Routes {
		if r.LinkIndex == h.Link.Index {
			if r != h.Route && !kernelDerivedRoute(h, r) {
				return ErrConflict
			}
		} else if !r.Destination.IsValid() || !r.Destination.Addr().IsLoopback() {
			return ErrConflict
		}
	}
	matches := 0
	for _, l := range i.Links {
		if l.Index != h.Link.Index && !baselineLink(i, l) {
			return ErrConflict
		}
		if l.Index == h.Link.Index || l.Name == h.Link.Name || l.Alias == h.Link.Alias {
			if !handleLinkMatches(h, l) {
				return ErrConflict
			}
			matches++
		}
	}
	if matches != 1 {
		return ErrConflict
	}
	return nil
}
func stepPresent(op Operation, h Handle, i Inventory) bool {
	switch op.Kind {
	case "set-alias-mtu-exact":
		for _, l := range i.Links {
			if l == h.Link {
				return true
			}
		}
	case "disable-owned-link-ipv6":
		return i.IPv6Disabled[h.Link.Index]
	case "address-add-exclusive":
		for _, a := range i.Addresses {
			if a == h.Address {
				return true
			}
		}
	case "link-up-exact":
		if !i.IPv6Disabled[h.Link.Index] {
			return false
		}
		for _, a := range i.Addresses {
			if a.LinkIndex == h.Link.Index && a.Prefix.Addr().Is6() {
				return false
			}
		}
		for _, r := range i.Routes {
			if r.LinkIndex == h.Link.Index && r.Destination.Addr().Is6() {
				return false
			}
		}
		return i.LinkUp[h.Link.Index]
	case "route-add-exclusive":
		for _, r := range i.Routes {
			if r == h.Route {
				return true
			}
		}
	}
	return false
}

// Stop has an independent finite cleanup context, not an expired grant context.
// No new traffic is allowed. It reads back before EVERY mutation, retains
// pending state on mismatch/unknown, and never assumes timeout means no effect.
// A private owned adapter instead closes the original before ticketed absence,
// while retaining the active ownership lane. It never uses generic rollback.
func (t *Transaction) Stop(ctx context.Context) error {
	if owned, ok := t.k.(interface{ stopOwnedTransaction(context.Context) error }); ok {
		return owned.stopOwnedTransaction(ctx)
	}
	t.stopStarted = true // invalidate display authority before the first cleanup read
	bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	ctx = bounded
	t.Ready = false
	if t.fdCloseAttempted && t.fdCloseError != nil {
		t.PendingCleanup = true
		return t.fdCloseError
	}
	if t.handle == nil {
		if t.PendingCleanup {
			return errors.New("ambiguous creation without FD custody; cleanup pending, never adopt")
		}
		return nil
	}
	h := *t.handle
	for steps := 0; steps < 4; steps++ {
		i, e := t.k.Snapshot(ctx)
		if e != nil {
			return e
		}
		ops, e := Rollback(h, i)
		if e != nil {
			return e
		}
		if len(ops) == 0 {
			e = t.closeFDOnce(h)
			if e != nil {
				return e
			}
			if e = verifyAbsence(h, i, t.baseline); e != nil {
				return e
			}
			t.PendingCleanup = false
			return nil
		}
		op := ops[0]
		t.Evidence = append(t.Evidence, Evidence{Step: op.Kind + "-intent"})
		var applyErr error
		if op.Kind == "close-owned-tun-fd" {
			applyErr = t.closeFDOnce(h)
		} else {
			applyErr = t.k.Apply(ctx, h, op)
		}
		after, readErr := t.k.Snapshot(ctx)
		if readErr != nil {
			return errors.Join(applyErr, readErr)
		}
		if op.Kind == "close-owned-tun-fd" && applyErr != nil {
			t.PendingCleanup = true
			return errors.Join(applyErr, readErr)
		}
		remaining, conflict := Rollback(h, after)
		if conflict != nil {
			return errors.Join(applyErr, conflict)
		}
		if operationPresent(op, remaining) {
			return errors.Join(applyErr, errors.New("cleanup not confirmed, remains pending"))
		}
		t.Evidence = append(t.Evidence, Evidence{Step: op.Kind, ACK: applyErr == nil, Readback: "absent", Error: applyErr != nil})
	}
	return errors.New("cleanup step bound reached; pending")
}
func operationPresent(op Operation, remaining []Operation) bool {
	for _, r := range remaining {
		if r == op {
			return true
		}
	}
	return false
}

// Expired is owner-enforced; a real runtime MUST arm its own monotonic timer and
// call Stop on expiry/context loss. This pure model cannot schedule kernel work.
func (t *Transaction) Expired() bool { return t.deadline.expired(t.clock.Now()) }

// Run owns a monotonic time.Timer as well as each operation context. It never
// renews/rebases a deadline after setup. Kernel methods MUST honor context; a
// stuck syscall/driver still needs the separately reviewed process supervisor.
func (t *Transaction) Run(ctx context.Context) error {
	startErr := t.Start(ctx)
	if startErr != nil {
		return errors.Join(startErr, t.Stop(context.WithoutCancel(ctx)))
	}
	now := t.clock.Now()
	remaining := t.deadline.Wall.Sub(now.Wall)
	if mono := t.deadline.Mono - now.Monotonic; mono < remaining {
		remaining = mono
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	var ended error
	select {
	case <-timer.C:
		ended = errors.New("original owner deadline reached")
	case <-ctx.Done():
		ended = ctx.Err()
	}
	return errors.Join(ended, t.Stop(context.WithoutCancel(ctx)))
}

func (t *Transaction) closeFDOnce(h Handle) error {
	if t.fdCloseAttempted {
		return t.fdCloseError
	}
	t.fdCloseAttempted = true
	t.fdCloseError = t.k.CloseOriginalFD(h)
	if t.fdCloseError != nil {
		t.PendingCleanup = true
		t.Evidence = append(t.Evidence, Evidence{Step: "close-owned-tun-fd-result", ACK: false, Readback: "custody-unresolved", Error: true})
	}
	return t.fdCloseError
}
func cloneInventory(i Inventory) Inventory {
	i.baselineLinks = append([]Link(nil), i.baselineLinks...)
	i.IPv6Disabled = maps.Clone(i.IPv6Disabled)
	i.LinkUp = maps.Clone(i.LinkUp)
	i.Links = append([]Link(nil), i.Links...)
	i.Routes = append([]Route(nil), i.Routes...)
	i.Addresses = append([]Address(nil), i.Addresses...)
	return i
}
func verifyAbsence(h Handle, i Inventory, baseline *Inventory) error {
	for _, l := range i.Links {
		if l.Index == h.Link.Index || l.Name == h.Link.Name || l.Alias == h.Link.Alias {
			return errors.New("original or replaced link remains")
		}
	}
	for _, a := range i.Addresses {
		if a.LinkIndex == h.Link.Index || a.Prefix == h.Address.Prefix || a.Prefix == h.Route.Destination {
			return errors.New("owned destination address remains")
		}
	}
	for _, r := range i.Routes {
		if r.LinkIndex == h.Link.Index || r.Destination == h.Route.Destination || r.Destination == h.Address.Prefix {
			return errors.New("owned destination route remains")
		}
	}
	if baseline == nil {
		return errors.New("baseline unavailable")
	}
	if !equalInventoryBaseline(*baseline, i) {
		return errors.New("unowned baseline changed")
	}
	return nil
}
func equalInventoryBaseline(a, b Inventory) bool {
	if a.baselineFingerprint != b.baselineFingerprint || !sameMultiset(a.baselineLinks, b.baselineLinks) || a.Namespace != b.Namespace || a.ForeignPolicy != b.ForeignPolicy || a.Forwarding != b.Forwarding || len(a.Links) != len(b.Links) || len(a.Routes) != len(b.Routes) || len(a.Addresses) != len(b.Addresses) {
		return false
	}
	return maps.Equal(a.IPv6Disabled, b.IPv6Disabled) && maps.Equal(a.LinkUp, b.LinkUp) && sameMultiset(a.Links, b.Links) && sameMultiset(a.Routes, b.Routes) && sameMultiset(a.Addresses, b.Addresses)
}
func sameMultiset[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[T]int)
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}
