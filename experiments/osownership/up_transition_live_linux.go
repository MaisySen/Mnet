//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"runtime"
)

// Supported amd64 stats ABI. The global default_qdisc sysctl is init_net-only
// in v6.17, so do not demand it inside the isolated target or claim its value.
// One held UP is followed by exact actual graph admission or owned cleanup.
func (o *originalHeldPacketResources) collectUpQdiscSource(ctx context.Context) (upQdiscObservation, error) {
	if !kernelExecutionReleased {
		return upQdiscObservation{}, ErrExecutorUnreleased
	}
	if o == nil || o.namespace == nil || runtime.GOARCH != "amd64" {
		return upQdiscObservation{}, afPhase
	}
	if e := o.namespace.checkCurrent(); e != nil {
		return upQdiscObservation{}, e
	}
	header, e := DumpRequestHeader(38)
	if e != nil {
		return upQdiscObservation{}, e
	}
	bodies, e := (residentNetlink{ns: o.namespace}).Request(ctx, 38, header, true)
	if e != nil {
		return upQdiscObservation{}, e
	}
	if e = o.namespace.checkCurrent(); e != nil {
		return upQdiscObservation{}, e
	}
	return upQdiscObservation{bodies: bodies}, nil
}
func (s *operation1ScopedDiagnostic) applyOwnedUp(ctx context.Context, r *postAddressDownResult) (result *postUpResult, err error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if r == nil || s == nil || s.scope == nil || s.source == nil || s.source.resources == nil {
		return nil, afPhase
	}
	scope := s.scope
	o := s.source.resources
	if scope.owner == nil || !scope.owner.adapter.authorized(ctx) || r != scope.owner.adapter.addressed || o.owner != scope.owner.tx || o.session != scope.owner.adapter.session || o.namespace != scope.ns || o.ledger != r.ledger || o.creation == nil || o.creation.fd == nil || scope.owner.attempt == nil || o.creation.fd != scope.owner.attempt.custody {
		return nil, afPhase
	}
	if e := scope.check(ctx, scope.owner.attempt); e != nil {
		return nil, e
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	held, e := heldSourceSnapshot(o.ledger, o.owner, o.session)
	if e != nil {
		return nil, e
	}
	collect := func(ctx context.Context) (RawInventory, SysctlSnapshot, upQdiscObservation, error) {
		raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
		if e != nil {
			return RawInventory{}, nil, upQdiscObservation{}, e
		}
		sys, e := readPolicySysctlsSource(ctx, o.namespace, raw)
		if e != nil {
			return RawInventory{}, nil, upQdiscObservation{}, e
		}
		q, e := o.collectUpQdiscSource(ctx)
		return raw, sys, q, e
	}
	raw, sys, q, e := collect(ctx)
	if e != nil {
		return nil, e
	}
	pre := ownedSourceDiagnostic{raw: raw, sys: sys, held: held, lineageUsable: o.ledger.matches(held.identity)}
	if _, e = scope.owner.adapter.composeSetupOnly(4, pre); e != nil {
		return nil, e
	}
	check := func() error {
		if e := scope.check(ctx, scope.owner.attempt); e != nil {
			return e
		}
		after, e := o.verifyOriginalDeviceLockedSource(ctx)
		if e != nil {
			return e
		}
		if !sameDisableOriginalIdentity(before.identity, after.identity) || before.deviceNamespace != after.deviceNamespace || before.tunName != after.tunName {
			return ErrConflict
		}
		return nil
	}
	socket, e := openRouteSocket(o.namespace)
	if e != nil {
		return nil, e
	}
	defer func() {
		err = errors.Join(err, socket.Close())
		if err != nil {
			if result != nil {
				result.lifetime.revoke()
				_ = result.ledger.invalidate("custody-loss")
			}
			result = nil
		}
	}()
	return finishOwnedUp(ctx, r, pre, q, 1, socket.pid, ownedUpDependencies{check: check, collect: collect, send: func(ctx context.Context, p *upSendPermit) (Datagram, error) {
		if e := check(); e != nil {
			return Datagram{}, e
		}
		ds, e := socket.exchangeUpSource(ctx, p.request, p)
		if e != nil {
			return Datagram{}, e
		}
		if len(ds) != 1 {
			return Datagram{}, afPhase
		}
		if e = check(); e != nil {
			return Datagram{}, e
		}
		return ds[0], nil
	}})
}
