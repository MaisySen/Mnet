//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// original FD/device-netns checks and actual read-only name/feature transport
// form a proposal used by the first-step source orchestrator. Caller holds
// original file custody. Real reads remain mechanically unreleased.
type originalSetupPostProposal struct {
	before, after *originalDeviceEvidence
	record        setupReadbackRecord
}

func (o *originalHeldPacketResources) collectSetupPostLockedSource(ctx context.Context, t *packetSetupTransition) (*originalSetupPostProposal, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if o == nil || o.creation == nil || o.creation.fd == nil || t == nil || t.ledger != o.ledger {
		return nil, ErrConflict
	}
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
	if e != nil {
		return nil, e
	}
	sys, e := readPolicySysctlsSource(ctx, o.namespace, raw)
	if e != nil {
		return nil, e
	}
	// No name-only selection: exact original handle index/name/alias/MTU and the
	// whole typed token expected-poststate are checked before minting evidence.
	socket, e := openOwnedEthtoolSource(o.namespace)
	if e != nil {
		return nil, e
	}
	defer socket.closeSource()
	names, features, e := socket.collectFeaturesSource(ctx, uint32(t.original.handle.Link.Index), t.original.handle.Link.Name)
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
	// The orchestrator checks
	// the live token/generation, exact raw/sys/framing and observed names/ACTIVE
	// through parsePost, then pairs it with the matching ACK before finish.
	record := setupReadbackRecord{before: before.identity, after: after.identity, raw: raw, sys: sys, names: names, features: features}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return &originalSetupPostProposal{before: before, after: after, record: record}, nil
}
