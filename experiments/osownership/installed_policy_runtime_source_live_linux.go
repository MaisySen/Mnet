//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Called inside the same original Control/controller/file ownership interval,
// before and after each packet syscall. This is NOT cached authority or a
// periodic poll. The current supported rows depend on owned link/AF values,
// exact addresses/routes/rules and the byte gate, not unchanged global ALL
// operands. Additional unresolved rows must add their missing hook/XFRM/mark
// operands before completion can succeed. No throughput result is claimed.
func (r *originalEndpointReactor) checkInstalledOperandsSource(ctx context.Context) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if r == nil || r.evidence == nil || r.evidence.resources == nil || r.evidence.routed == nil || r.owner == nil || r.owner.adapter == nil || !r.owner.adapter.authorized(ctx) {
		return afPhase
	}
	o := r.evidence.resources
	if o.namespace == nil || o.session == nil || o.session.canonical == nil {
		return afPhase
	}
	raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
	if e != nil {
		return e
	}
	if !exactSetupPostMatches(raw, r.evidence.diagnostic.raw) {
		return ErrConflict
	}
	return ctx.Err()
}
