//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
)

// Created only for one resident callback. Every source call checks active plus
// namespaceCustody's live FD/TID/current namespace. No namespace ownership is
// transferred or released by this adapter; resident owns it through cleanup.
type operation1ResidentSource struct {
	afOwner  *afSourcePassOwner
	owner    *operation1Controller
	ns       *namespaceCustody
	baseline *CapturedBaseline
	active   bool
	created  *ownedCreation
}

// Intended residentBootstrapSource work body, not installed in a launcher/app.
// Callback cannot keep using the controller's source after returning. Finalize
// the original before namespace scope/lifetime ends, even on callback error.
func withOperation1ResidentSource(ctx context.Context, ns *namespaceCustody, b *CapturedBaseline, id adapterIdentity, binding Binding, work func(*operation1Controller) error) (err error) {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if ns == nil || work == nil || b == nil || !sameNamespace(ns.identity, b.ns) || !sameNamespace(id.Namespace, b.ns) {
		return ErrConflict
	}
	if e := ns.checkCurrent(); e != nil {
		return e
	}
	c, e := newOperation1SourceController(b, id, binding)
	if e != nil {
		return e
	}
	source := &operation1ResidentSource{owner: c, ns: ns, baseline: c.adapter.session.baseline, active: true}
	c.adapter.creation = source
	c.absence = source
	defer func() {
		err = errors.Join(err, c.stop(ctx))
		c.mu.Lock()
		source.active = false
		if source.afOwner != nil {
			source.afOwner.active = false
			source.afOwner.last = nil
		}
		c.mu.Unlock()
	}()
	return work(c)
}
func (s *operation1ResidentSource) check(ctx context.Context, a *originalAttempt) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || s == nil || !s.active || s.owner == nil || s.ns == nil || s.baseline == nil || a == nil || a.owner != s.owner || s.owner.attempt != a || !sameNamespace(a.plan.Binding.Namespace, s.ns.identity) {
		return ErrConflict
	}
	return s.ns.checkCurrent()
}
func (s *operation1ResidentSource) createOriginal(ctx context.Context, p Plan, a *originalAttempt) (Handle, error) {
	if e := s.check(ctx, a); e != nil {
		return Handle{}, e
	}
	// Same source baseline must still exist before any original open. Only the
	// existing pinned comparison-copy counter/timestamp policy may differ.
	raw, e := ReadStableInventory(ctx, residentNetlink{ns: s.ns}, s.ns.checkCurrent)
	if e != nil {
		return Handle{}, e
	}
	sys, e := readPolicySysctlsSource(ctx, s.ns, raw)
	if e != nil {
		return Handle{}, e
	}
	if !originalBaselineMatches(p, s.baseline, raw, sys) {
		return Handle{}, ErrConflict
	}
	created, e := createTUNSource(ctx, s.ns, p, a, func() (Link, error) {
		raw, e := ReadStableInventory(ctx, residentNetlink{ns: s.ns}, s.ns.checkCurrent)
		if e != nil {
			return Link{}, e
		}
		var found *LinkRecord
		for _, l := range raw.Links {
			if l.Name == p.Binding.Name {
				if found != nil {
					return Link{}, ErrConflict
				}
				copy := l
				found = &copy
			}
		}
		if found == nil {
			return Link{}, ErrConflict
		}
		kind, e := linkKind(*found)
		if e != nil || kind != "tun" {
			return Link{}, ErrConflict
		}
		return Link{Index: found.Index, Name: found.Name, Alias: found.Alias, MTU: found.MTU, Kind: kind}, nil
	})
	s.created = created
	if created == nil {
		return Handle{}, e
	}
	if a.handle == nil {
		return Handle{}, errors.Join(e, ErrConflict)
	}
	return *a.handle, e
}
func (s *operation1ResidentSource) bindCreatedOriginal(ctx context.Context, a *originalAttempt) (operation1DiagnosticSource, error) {
	if e := s.check(ctx, a); e != nil {
		return nil, e
	}
	if s.created == nil || s.created.fd != a.custody || a.handle == nil {
		return nil, ErrConflict
	}
	session := s.owner.adapter.session
	v := heldPacketIdentity{generation: 1, target: s.ns.identity, device: s.ns.identity, identity: session.identity, handle: *a.handle}
	resources, e := retainOriginalPacketResources(s.created, s.ns, s.owner.tx, session, v)
	if e != nil {
		return nil, e
	}
	// Retention above is correlation, not device proof. Confirm the original
	// kernel-derived namespace/name before exposing this diagnostic supplier.
	a.custody.mu.Lock()
	_, e = resources.verifyOriginalDeviceLockedSource(ctx)
	a.custody.mu.Unlock()
	if e != nil {
		return nil, e
	}
	return &operation1ScopedDiagnostic{scope: s, source: &operation1OriginalSource{resources: resources}}, nil
}

type operation1ScopedDiagnostic struct {
	scope  *operation1ResidentSource
	source *operation1OriginalSource
}

func (s *operation1ScopedDiagnostic) diagnoseOperation1(ctx context.Context) (ownedSourceDiagnostic, error) {
	if e := s.scope.check(ctx, s.scope.owner.attempt); e != nil {
		return ownedSourceDiagnostic{}, e
	}
	observation, e := s.scope.observeCreatedOriginalAF(ctx, s.source.resources)
	if e != nil {
		return ownedSourceDiagnostic{}, e
	}
	return observation.diagnostic, nil
}
func (s *operation1ScopedDiagnostic) applyOperation1(ctx context.Context, p Plan) (*firstStepCompletion, error) {
	if e := s.scope.check(ctx, s.scope.owner.attempt); e != nil {
		return nil, e
	}
	return s.source.applyOperation1(ctx, p)
}
func (s *operation1ResidentSource) observeOriginalAbsence(ctx context.Context, a *originalAttempt) (*originalAbsenceEvidence, error) {
	if e := s.check(ctx, a); e != nil {
		return nil, e
	}
	ticket := a.absenceTicket
	if ticket == nil || !ticket.active || ticket.used || ticket.owner != s.owner || ticket.attempt != a || ticket.sequence != a.absenceSequence || !a.closeAttempted || a.custody == nil {
		return nil, ErrConflict
	}
	a.custody.mu.Lock()
	closed := a.custody.closed
	a.custody.mu.Unlock()
	if !closed {
		return nil, ErrConflict
	}
	raw, e := ReadStableInventory(ctx, residentNetlink{ns: s.ns}, s.ns.checkCurrent)
	if e != nil {
		return nil, e
	}
	sys, e := readPolicySysctlsSource(ctx, s.ns, raw)
	if e != nil {
		return nil, e
	}
	if e = s.check(ctx, a); e != nil {
		return nil, e
	}
	if a.absenceTicket != ticket || !ticket.active || ticket.used || !originalBaselineMatches(a.plan, s.baseline, raw, sys) {
		return nil, ErrConflict
	}
	// No operational Inventory. Pure parser rechecks ticket/correlation/raw equality.
	return parseOriginalAbsence(a, ticket, s.baseline, raw, sys)
}
