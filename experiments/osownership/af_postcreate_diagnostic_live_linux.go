//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Typed postcreate DIAGNOSTIC, no Inventory/Ready conversion. Original custody
// brackets the ACTUAL route AF producer, raw/sys collection and assessment.
type postcreateAFSourceDiagnostic struct {
	ipv6Operands  *ipv6PolicyOperands
	ipv6Rules     []ipv6ConsumerAssessment
	ipv4Rules     []ipv4ConsumerAssessment
	before, after *originalDeviceEvidence
	pass          *afReadPass
	diagnostic    ownedSourceDiagnostic
	policy        ownedTUNPolicyAssessment
}

func (s *operation1ResidentSource) observeCreatedOriginalAF(ctx context.Context, o *originalHeldPacketResources) (*postcreateAFSourceDiagnostic, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if s == nil || s.owner == nil {
		return nil, ErrConflict
	}
	if s.afOwner != nil {
		s.afOwner.last = nil
	}
	if e := s.check(ctx, s.owner.attempt); e != nil {
		return nil, e
	}
	if o == nil || o.owner != s.owner.tx || o.session != s.owner.adapter.session || o.namespace != s.ns || o.creation == nil || o.creation.fd == nil || o.creation.fd != s.owner.attempt.custody || o.session.forwardStep != 1 || o.session.canonical == nil || o.session.canonical.TagConfirmed || o.session.pending != nil {
		return nil, afPhase
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	held, e := heldSourceSnapshot(o.ledger, o.owner, o.session)
	if e != nil {
		return nil, e
	}
	if held.identity != before.identity {
		return nil, ErrConflict
	}
	if s.afOwner == nil {
		s.afOwner = &afSourcePassOwner{controller: s.owner, active: true}
	}
	socket, e := openRouteSocket(s.ns)
	if e != nil {
		return nil, e
	}
	defer socket.Close()
	s.afOwner.port = socket.pid
	pass, e := collectRouteAFPass(ctx, s.afOwner, socket)
	if e != nil {
		return nil, e
	}
	good := false
	defer func() {
		if !good {
			s.afOwner.last = nil
		}
	}()
	raw, e := ReadStableInventory(ctx, residentNetlink{ns: s.ns}, s.ns.checkCurrent)
	if e != nil {
		return nil, e
	}
	if !exactSetupPostMatches(RawInventory{Links: pass.links}, RawInventory{Links: raw.Links}) {
		return nil, ErrConflict
	}
	sys, e := readPolicySysctlsSource(ctx, s.ns, raw)
	if e != nil {
		return nil, e
	}
	pass.sys = cloneSysExact(sys)
	pass.effective, e = parseEffectiveV4(sys)
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
	diagnostic, e := diagnosticForHeldRecords(raw, sys, held)
	if e != nil {
		return nil, e
	}
	link, e := pass.link(o.creation.handle.Link.Index)
	if e != nil {
		return nil, e
	}
	operands, e := parseIPv6PolicyOperands(sys)
	if e != nil {
		return nil, e
	}
	v4rules, e := assessIPv4ConsumerOperands(link, pass.effective)
	if e != nil {
		return nil, e
	}
	rules, e := assessIPv6ConsumerOperands(link, operands)
	if e != nil {
		return nil, e
	}
	policy, e := assessCreationDownPolicy(diagnostic)
	if e != nil {
		return nil, e
	}
	diagnostic.afPass = pass
	if e = s.check(ctx, s.owner.attempt); e != nil {
		return nil, e
	}
	good = true
	// DO NOT bindCreationApplicability or convert. Creation-DOWN-only resolutions
	// do not apply to later phases. Missing
	// phase/profile semantics stay explicit. Snapshot caller still always refuses.
	return &postcreateAFSourceDiagnostic{before: before, after: after, pass: pass, diagnostic: diagnostic, policy: policy, ipv6Operands: operands, ipv6Rules: rules, ipv4Rules: v4rules}, nil
}
