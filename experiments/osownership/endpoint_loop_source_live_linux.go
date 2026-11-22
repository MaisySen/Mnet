//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Source adapter for the bounded resident loop. No worker is allowed to hold
// this object. ctx and cleanup are actual caller-owned contexts; no background
// context/renewed authority is manufactured here. Cleanup must be bounded by
// still-held original resident lifetime and remain on its original TID.
type endpointOriginalLane struct {
	reactor      *originalEndpointReactor
	inbox        *endpointBridgeInbox
	ctx, cleanup context.Context
}

func (l *endpointOriginalLane) checkStop() error {
	if l == nil || l.reactor == nil || l.inbox == nil || l.ctx == nil || l.cleanup == nil {
		return afPhase
	}
	if e := l.inbox.halted(); e != nil {
		return e
	}
	return l.ctx.Err()
}
func (l *endpointOriginalLane) drainOne() error {
	if e := l.checkStop(); e != nil {
		return e
	}
	return l.reactor.drainOneSource(l.ctx)
}
func (l *endpointOriginalLane) receiveOne(f endpointFrame) error {
	if e := l.checkStop(); e != nil {
		return e
	}
	return l.reactor.receiveSource(l.ctx, f)
}
func (l *endpointOriginalLane) stopOwned() error {
	if l == nil || l.reactor == nil || l.cleanup == nil {
		return afPhase
	}
	return l.reactor.stopSource(l.cleanup)
}
