//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
)

// Forward phases only. This never consumes input, creates a watcher, renews
// membership, or supplies completion evidence. Cancellation/parent loss ends
// forward work. Retained-original absence deliberately does NOT call it.
func (a *residentEndpointAdmission) beginForwardPieceSource(ctx context.Context) (context.Context, func(error) error, error) {
	if !kernelExecutionReleased {
		return nil, nil, ErrExecutorUnreleased
	}
	if a == nil || a.ns == nil || ctx == nil {
		return nil, nil, afPhase
	}
	if e := a.checkMembershipSource(ctx, a.ns, a.start.binding()); e != nil {
		return nil, nil, e
	}
	piece, cancel := context.WithTimeout(ctx, endpointObservationPieceBudget)
	finish := func(e error) error {
		defer cancel()
		// Check before cancel and after all phase defers/retained Control intervals.
		// A completed syscall may have happened before a late refusal; no rollback
		// or hard syscall/scheduling ceiling is asserted here.
		return errors.Join(e, piece.Err(), a.checkMembershipSource(ctx, a.ns, a.start.binding()))
	}
	return piece, finish, nil
}
func (s *operation1ResidentSource) beginForwardPieceSource(ctx context.Context) (context.Context, func(error) error, error) {
	if !kernelExecutionReleased {
		return nil, nil, ErrExecutorUnreleased
	}
	if s == nil || !s.active || s.owner == nil || s.owner.tx == nil || s.ns == nil || !s.ownerThreadCurrent() || ctx == nil {
		return nil, nil, afPhase
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	if s.channel == nil {
		return ctx, func(e error) error { return e }, nil
	}
	if s.channel.ns != s.ns || s.channel.start.binding() != s.owner.tx.plan.Binding {
		return nil, nil, afPhase
	}
	piece, finish, e := s.channel.beginForwardPieceSource(ctx)
	if e != nil {
		return nil, nil, e
	}
	return piece, func(err error) error {
		err = finish(err)
		if err != nil {
			// The source boundary is fatal, including a refusal detected after inner
			// phase defers ran. Do not retain a usable AF pass/forward ancestry.
			if s.disableLife != nil {
				s.disableLife.revoke()
			}
			if s.afOwner != nil {
				s.afOwner.last = nil
			}
		}
		return err
	}, nil
}
func (s *operation1ResidentSource) checkForwardSource(ctx context.Context, a *originalAttempt) error {
	if e := s.check(ctx, a); e != nil {
		return e
	}
	if s.channel != nil {
		return s.channel.checkMembershipSource(ctx, s.ns, s.owner.tx.plan.Binding)
	}
	return nil
}
