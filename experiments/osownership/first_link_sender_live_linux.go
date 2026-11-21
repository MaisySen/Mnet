//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Private sender dependency of the source first-step orchestrator. Caller holds
// original file custody throughout send AND subsequent collection/finish. The
// fresh socket port is bound into the token. No adoption, CREATE or UP.
func (o *originalHeldPacketResources) sendFirstLinkUpdateLockedSource(ctx context.Context, s *routeSocket, t *packetSetupTransition, permit *firstLinkSendPermit) (Datagram, error) {
	if !kernelExecutionReleased {
		return Datagram{}, ErrExecutorUnreleased
	}
	if o == nil || o.creation == nil || o.creation.fd == nil || s == nil || t == nil || permit == nil || permit.token != t || t.ledger != o.ledger || s.ns != o.namespace {
		return Datagram{}, ErrConflict
	}
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		t.abandon()
		return Datagram{}, e
	}
	if before.identity.generation != t.generation {
		t.abandon()
		return Datagram{}, ErrConflict
	}
	ds, e := s.exchangeSource(ctx, t.request, permit)
	if e != nil {
		t.abandon()
		return Datagram{}, e
	}
	if len(ds) != 1 {
		t.abandon()
		return Datagram{}, ErrConflict
	}
	after, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil || after.identity.fdObject != before.identity.fdObject || after.deviceNamespace != before.deviceNamespace || after.tunName != before.tunName {
		t.abandon()
		return Datagram{}, ErrConflict
	}
	return ds[0], nil
}
