//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Called inside the same original Control/controller/file ownership interval,
// before and after each packet syscall. This is NOT cached authority or a
// periodic poll. It compares common original link/AF/graph prerequisites. It
// does not prove their full kernel consumer/lifecycle/emission relevance, a
// supported running-kernel semantic profile or current ALL/hooks/classifiers.
// XFRM metadata has its own fresh original-bound evidence check below.
// Those missing named operands still refuse completion. No throughput result.
func (r *originalEndpointReactor) checkInstalledOperandsSource(ctx context.Context, fd int) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if r == nil || r.receipt == nil || r.evidence == nil || r.evidence.resources == nil || r.evidence.routed == nil || r.owner == nil || r.owner.adapter == nil || !r.owner.adapter.authorized(ctx) {
		return afPhase
	}
	a, ok := r.admission.(*residentEndpointAdmission)
	if !ok || a != r.evidence.admission {
		return afPhase
	}
	o := r.evidence.resources
	if o.namespace == nil || o.session == nil || o.session.canonical == nil {
		return afPhase
	}
	if e := r.checkParentBoundarySource(ctx); e != nil {
		return e
	}
	piece, cancel := context.WithTimeout(ctx, endpointObservationPieceBudget)
	raw, e := ReadStableInventory(piece, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
	if e == nil {
		e = piece.Err()
	}
	cancel()
	if e != nil {
		return e
	}
	if e = r.checkParentBoundarySource(ctx); e != nil {
		return e
	}
	if !endpointIPv6AddressRouteAbsent(raw) {
		return afPolicy
	}
	if !exactSetupPostMatches(raw, r.evidence.diagnostic.raw) {
		return ErrConflict
	}
	fresh, e := o.collectOriginalXfrmControlSource(ctx, fd, r.receipt.identity, a)
	if e != nil {
		return e
	}
	if !r.evidence.xfrm.matches(o, r.receipt.identity, a) || !fresh.matches(o, r.receipt.identity, a) || fresh.data != r.evidence.xfrm.data {
		return errXfrmEnvironment
	}
	return r.checkParentBoundarySource(ctx)
}
