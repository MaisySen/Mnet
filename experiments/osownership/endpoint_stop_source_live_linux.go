//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
)

// The controller entry owns memoization, including setup/Transaction failures
// that entered stopOwnedLocked before the loop. This wrapper adds no interval.
func (s *operation1ResidentSource) cleanupEndpointSource(ctx context.Context) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if s == nil || !s.active || s.owner == nil || s.channel == nil || !s.ownerThreadCurrent() {
		return afPhase
	}
	return s.owner.stop(ctx)
}

// Same resident TID, no controller/file mutex acquisition, data read or worker.
// The actual membership check polls HUP even if STOP/data remain buffered.
func (r *originalEndpointReactor) checkParentBoundarySource(ctx context.Context) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if ctx == nil {
		return afPhase
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if r == nil || r.owner == nil || r.owner.tx == nil || r.owner.lane == nil || !r.owner.lane.ownerThreadCurrent() || r.admission == nil || r.evidence == nil || r.evidence.resources == nil || r.receipt == nil {
		return afPhase
	}
	return r.admission.checkResidentSource(ctx, r.evidence.resources.namespace, r.owner.tx.plan.Binding, r.receipt.identity.generation)
}
