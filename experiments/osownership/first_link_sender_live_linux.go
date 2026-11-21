//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// SOURCE PROPOSAL, no caller/release. A fresh socket's exact port must already
// have been used to construct the token. Does not create/adopt names or set UP.
// No finish/mint here: actual same-pass feature-name readback is still missing.
func (o *originalHeldPacketResources) sendFirstLinkUpdateSource(ctx context.Context, s *routeSocket, t *packetSetupTransition) (*setupACKEvidence, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if o == nil || o.creation == nil || o.creation.fd == nil || s == nil || t == nil || t.ledger != o.ledger || s.ns != o.namespace {
		return nil, ErrConflict
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		t.abandon()
		return nil, e
	}
	if before.identity.generation != t.generation {
		t.abandon()
		return nil, ErrConflict
	}
	ds, e := s.exchangeSource(ctx, t.request, &firstLinkSendPermit{token: t})
	if e != nil {
		t.abandon()
		return nil, e
	}
	if len(ds) != 1 {
		t.abandon()
		return nil, ErrConflict
	}
	ack, e := t.parseACK(ds[0])
	if e != nil {
		t.abandon()
		return nil, e
	}
	after, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil || after.identity.fdObject != before.identity.fdObject || after.deviceNamespace != before.deviceNamespace || after.tunName != before.tunName {
		t.abandon()
		return nil, ErrConflict
	}
	return ack, nil
}
