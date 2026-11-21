//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Source-connected diagnostic collector. Unused and execution-unreleased. It
// reads raw inventory and ALL through existing bounded namespace readers, then
// applies the same scoped TUN schema and effective formulas as fake passes.
// This is NOT original TUN FD/netns proof, an atomic pass, or active readiness.
func (o *originalHeldPacketResources) collectSourceDiagnostic(ctx context.Context) (ownedSourceDiagnostic, error) {
	var out ownedSourceDiagnostic
	if !kernelExecutionReleased {
		return out, ErrExecutorUnreleased
	}
	if o == nil || o.creation == nil || o.namespace == nil || o.session == nil || o.session.canonical == nil || o.ledger == nil {
		return out, ErrConflict
	}
	if e := o.namespace.checkCurrent(); e != nil {
		return out, e
	}
	held, e := heldSourceSnapshot(o.ledger, o.owner, o.session)
	if e != nil {
		return out, e
	}
	raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
	if e != nil {
		return out, e
	}
	sys, e := readPolicySysctlsSource(ctx, o.namespace, raw)
	if e != nil {
		return out, e
	}
	return diagnosticForHeldRecords(raw, sys, held)
}
