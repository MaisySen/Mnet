//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

func (s *operation1ResidentSource) observePrecreation(ctx context.Context) (*precreationObservation, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || s == nil || !s.active || s.owner == nil || s.owner.attempt != nil || s.owner.tx.stopStarted {
		return nil, afPhase
	}
	if e := s.ns.checkCurrent(); e != nil {
		return nil, e
	}
	c := s.owner
	if c.precreationSequence == ^uint64(0) {
		return nil, ErrConflict
	}
	c.precreationSequence++
	if c.precreationGeneration == 0 {
		c.precreationGeneration = 1
	}
	if s.afOwner == nil {
		s.afOwner = &afSourcePassOwner{controller: c, active: true}
	}
	// Fresh real socket, actual bound local port. Client reads exact request and
	// complete kernel-origin reply through this recording wire, not fake reader.
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
	raw, e := ReadStableInventory(ctx, residentNetlink{ns: s.ns}, s.ns.checkCurrent)
	if e != nil {
		s.afOwner.last = nil
		return nil, e
	}
	if !exactSetupPostMatches(RawInventory{Links: pass.links}, RawInventory{Links: raw.Links}) {
		s.afOwner.last = nil
		return nil, ErrConflict
	}
	sys, e := readPolicySysctlsSource(ctx, s.ns, raw)
	if e != nil {
		s.afOwner.last = nil
		return nil, e
	}
	effective, e := parseEffectiveV4(sys)
	if e != nil {
		s.afOwner.last = nil
		return nil, e
	}
	if e = s.ns.checkCurrent(); e != nil {
		s.afOwner.last = nil
		return nil, e
	}
	pass.sys = cloneSysExact(sys)
	pass.effective = effective
	return &precreationObservation{owner: c, baseline: s.baseline, pass: pass, raw: cloneRawExact(raw), sys: cloneSysExact(sys), generation: c.precreationGeneration, sequence: c.precreationSequence, phase: "precreation", identity: c.adapter.session.identity, binding: c.tx.plan.Binding}, nil
}
