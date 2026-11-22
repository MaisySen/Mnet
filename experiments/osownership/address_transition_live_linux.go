//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
)

// Resident controller lease and original file mutex span every operation here.
// Mechanically unreleased; no launcher/device authority is added by this path.
func (s *operation1ScopedDiagnostic) applyOwnedAddress(ctx context.Context, d *postDisableResult) (result *postAddressDownResult, err error) {
	if s == nil || s.scope == nil {
		return result, afPhase
	}
	piece, finish, pieceErr := s.scope.beginForwardPieceSource(ctx)
	if pieceErr != nil {
		return result, pieceErr
	}
	ctx = piece
	defer func() {
		err = finish(err)
		if err != nil {
			result = nil
		}
	}()

	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if d == nil || s == nil || s.scope == nil || s.source == nil || s.source.resources == nil {
		return nil, afPhase
	}
	scope := s.scope
	o := s.source.resources
	if scope.owner == nil || !scope.owner.adapter.authorized(ctx) || d != scope.owner.adapter.disabled || o.owner != scope.owner.tx || o.session != scope.owner.adapter.session || o.namespace != scope.ns || o.ledger != d.ledger || o.creation == nil || o.creation.fd == nil || scope.owner.attempt == nil || o.creation.fd != scope.owner.attempt.custody {
		return nil, afPhase
	}
	if e := scope.checkForwardSource(ctx, scope.owner.attempt); e != nil {
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
	collect := func(ctx context.Context) (RawInventory, SysctlSnapshot, error) {
		raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
		if e != nil {
			return RawInventory{}, nil, e
		}
		sys, e := readPolicySysctlsSource(ctx, o.namespace, raw)
		return raw, sys, e
	}
	raw, sys, e := collect(ctx)
	if e != nil {
		return nil, e
	}
	pre := ownedSourceDiagnostic{raw: raw, sys: sys, held: held, lineageUsable: o.ledger.matches(held.identity)}
	// Re-admit the freshly collected full baseline/sys state under phase3,
	// not merely the earlier Snapshot ticket identity. Drift before intent
	// must fail before creating a send permit.
	if _, e = scope.owner.adapter.composeSetupOnly(3, pre); e != nil {
		return nil, e
	}
	check := func() error {
		if e := scope.checkForwardSource(ctx, scope.owner.attempt); e != nil {
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
	return finishOwnedAddressDown(ctx, d, pre, 1, socket.pid, ownedAddressDependencies{
		check: check, collect: collect,
		send: func(ctx context.Context, p *addressSendPermit) (Datagram, error) {
			if e := check(); e != nil {
				return Datagram{}, e
			}
			ds, e := socket.exchangeAddressSource(ctx, p.request, p)
			if e != nil {
				return Datagram{}, e
			}
			if len(ds) != 1 {
				return Datagram{}, afPhase
			}
			if e := check(); e != nil {
				return Datagram{}, e
			}
			return ds[0], nil
		},
	})
}
