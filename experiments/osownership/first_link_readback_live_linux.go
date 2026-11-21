//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Source proposal preserves actual kernel-derived device namespace/name evidence
// and parsed raw/ALL/TUN diagnostics. NOT setupPostEvidence: ethtool STRSET+ACTIVE
// actual socket supplier remains missing, so this cannot finish/mint a setup.
type originalReadbackProposal struct {
	before, after *originalDeviceEvidence
	diagnostic    ownedSourceDiagnostic
}

func (o *originalHeldPacketResources) readFirstLinkPostSource(ctx context.Context) (*originalReadbackProposal, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if o == nil || o.creation == nil || o.creation.fd == nil {
		return nil, ErrConflict
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	diagnostic, e := o.collectSourceDiagnostic(ctx)
	if e != nil {
		return nil, e
	}
	after, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	if before.identity != after.identity || before.deviceNamespace != after.deviceNamespace || before.tunName != after.tunName {
		return nil, ErrConflict
	}
	return &originalReadbackProposal{before: before, after: after, diagnostic: diagnostic}, nil
}
