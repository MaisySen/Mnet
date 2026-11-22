//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
)

type peerRouteSendPermit struct {
	prior      *postUpResult
	original   heldPacketIdentity
	generation uint64
	request    []byte
	used       bool
}

func (p *peerRouteSendPermit) currentLocked() bool {
	if p == nil || p.prior == nil || p.prior.ancestry == nil || p.prior.ancestry.disabledAncestry == nil {
		return false
	}
	r := p.prior
	d := r.ancestry.disabledAncestry
	l := r.ledger
	return l != nil && r.lifetime.current() && d.owner != nil && !d.owner.Ready && !d.owner.stopStarted && !d.owner.fdCloseAttempted && d.owner.clock != nil && !d.owner.deadline.expired(d.owner.clock.Now()) && d.session != nil && d.session.owner == d.owner && d.session.forwardStep == 5 && !d.session.poisoned && !d.session.cleanupMode && !d.session.closeAttempted && l.current.uncertain && !l.current.closed && l.current.generation == p.generation && sameDisableOriginalIdentity(l.current, p.original) && l.setup == nil
}
func (p *peerRouteSendPermit) send(raw []byte, pid uint32, effect func() error) error {
	if p == nil || p.prior == nil || p.prior.ledger == nil || effect == nil {
		return afPhase
	}
	l := p.prior.ledger
	l.mu.Lock()
	if p.used || !p.currentLocked() || !bytes.Equal(raw, p.request) || len(raw) < 32 || binary.NativeEndian.Uint16(raw[4:]) != 24 || binary.NativeEndian.Uint16(raw[6:]) != 0x605 || binary.NativeEndian.Uint32(raw[12:]) != pid {
		l.mu.Unlock()
		return afPhase
	}
	p.used = true
	l.mu.Unlock()
	return effect()
}
func (p *peerRouteSendPermit) current() bool {
	if p == nil || p.prior == nil || p.prior.ledger == nil {
		return false
	}
	l := p.prior.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	return p.currentLocked()
}

type postPeerRouteResult struct {
	ancestry *postUpResult
	lifetime *disableResidentLifetime
	ledger   *heldPacketLedger
	identity heldPacketIdentity
	raw      RawInventory
	sys      SysctlSnapshot
	qdisc    upQdiscObservation
	consumed bool
}

func (r *postPeerRouteResult) current() bool {
	return r != nil && r.ancestry != nil && r.lifetime == r.ancestry.lifetime && r.lifetime.current() && r.ledger != nil && r.ledger.matches(r.identity)
}

type ownedPeerRouteDependencies struct {
	check   func() error
	send    func(context.Context, *peerRouteSendPermit) (Datagram, error)
	collect func(context.Context) (RawInventory, SysctlSnapshot, upQdiscObservation, error)
}

// Original file mutex/controller lane span the entire exclusive peer-route transition.
// Exact one new route is construction only, not runtime Ready.
func finishOwnedPeerRoute(ctx context.Context, r *postUpResult, pre ownedSourceDiagnostic, qbefore upQdiscObservation, seq, pid uint32, deps ownedPeerRouteDependencies) (*postPeerRouteResult, error) {
	if ctx == nil || ctx.Err() != nil || r == nil || !r.current() || !r.consumed || r.ancestry == nil || r.ancestry.disabledAncestry == nil || seq == 0 || pid == 0 || deps.check == nil || deps.send == nil || deps.collect == nil || pre.held == nil || !pre.lineageUsable {
		return nil, afPhase
	}
	d := r.ancestry.disabledAncestry
	s := d.session
	o := d.owner
	l := r.ledger
	if s == nil || o == nil || s.canonical == nil || s.plan == nil || len(s.plan.Operations) != 6 || !reflect.DeepEqual(*s.plan, o.plan) || s.forwardStep != 5 || s.pending != nil || pre.held.identity != r.identity || pre.held.owner != o || pre.held.session != s || pre.held.ledger != l || !exactSetupPostMatches(pre.raw, r.raw) || !reflect.DeepEqual(pre.sys, r.sys) || !sameConstructionQdisc(qbefore, r.qdisc) {
		return nil, afPhase
	}
	m, e := BuildExactMutation(*s.canonical, s.plan.Operations[5])
	if e != nil || m.kind != 24 {
		return nil, afPhase
	}
	request := make([]byte, 16+len(m.payload))
	binary.NativeEndian.PutUint32(request, uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:], 24)
	binary.NativeEndian.PutUint16(request[6:], 0x605)
	binary.NativeEndian.PutUint32(request[8:], seq)
	binary.NativeEndian.PutUint32(request[12:], pid)
	copy(request[16:], m.payload)
	if e = deps.check(); e != nil {
		return nil, e
	}
	l.mu.Lock()
	if !l.matchesLocked(r.identity) || l.setup != nil || l.current.generation >= ^uint64(0)-1 {
		l.mu.Unlock()
		return nil, afPhase
	}
	original := l.current
	if e = l.invalidateLocked("config-intent"); e != nil {
		l.mu.Unlock()
		return nil, e
	}
	p := &peerRouteSendPermit{prior: r, original: original, generation: l.current.generation, request: request}
	l.mu.Unlock()
	check := func() error {
		if ctx.Err() != nil || !p.current() {
			return afPhase
		}
		return deps.check()
	}
	if e = check(); e != nil {
		return nil, e
	}
	ack, e := deps.send(ctx, p)
	if e != nil {
		return nil, e
	}
	// Same exact fresh-socket capped-success contract grounded in S76 af_netlink.
	ackParser := &addressSendPermit{request: p.request}
	if !p.used || !ackParser.ack(ack) {
		return nil, afPhase
	}
	if e = check(); e != nil {
		return nil, e
	}
	raw, sys, q, e := deps.collect(ctx)
	if e != nil {
		return nil, e
	}
	if e = check(); e != nil {
		return nil, e
	}
	if e = validatePeerRouteDelta(*s.canonical, pre.raw, pre.sys, raw, sys, qbefore, q); e != nil {
		return nil, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !p.currentLocked() {
		return nil, afPhase
	}
	l.current.generation++
	l.current.uncertain = false
	return &postPeerRouteResult{ancestry: r, lifetime: r.lifetime, ledger: l, identity: l.current, raw: cloneRawExact(raw), sys: cloneSysExact(sys), qdisc: cloneUpQdisc(q)}, nil
}
func validatePeerRouteDelta(h Handle, before RawInventory, beforeSys SysctlSnapshot, after RawInventory, sys SysctlSnapshot, qbefore, qafter upQdiscObservation) error {
	if !reflect.DeepEqual(beforeSys, sys) || len(before.Routes) != 1 || len(after.Routes) != 2 || !sameConstructionQdisc(qbefore, qafter) {
		return afPhase
	}
	if validateAddressLocalRoute(&h, before.Routes[0]) != nil {
		return afPhase
	}
	normalized := cloneRawExact(after)
	normalized.Routes = nil
	local, peer := 0, 0
	for _, r := range after.Routes {
		if validateAddressLocalRoute(&h, r) == nil {
			local++
			normalized.Routes = append(normalized.Routes, r)
		} else if validatePeerRoute(&h, r) == nil {
			peer++
		} else {
			return afPhase
		}
	}
	if local != 1 || peer != 1 || !exactSetupPostMatches(normalized, before) {
		return afPhase
	}
	return nil
}
func validatePeerRoute(h *Handle, r RouteRecord) error {
	if h == nil || r.Family != 2 || r.DestBits != 32 || r.SourceBits != 0 || r.TOS != 0 || r.Table != 254 || r.TableID != 254 || r.Protocol != 242 || r.Scope != 253 || r.Type != 1 || r.Flags != 0 || r.Dst != h.Route.Destination.Addr() || r.PreferredSource.IsValid() || r.Source.IsValid() || r.Gateway.IsValid() || r.OIF != uint32(h.Link.Index) || r.IIF != 0 || r.Priority != h.Route.Priority || len(r.Raw) != 4 {
		return afPhase
	}
	if !bytes.Equal(r.Raw[1], ipBytes(r.Dst)) {
		return afShape
	}
	for k, w := range map[uint16]uint32{4: r.OIF, 6: r.Priority, 15: r.TableID} {
		v, e := attrUint(r.Raw, k, true)
		if e != nil || v != w {
			return afShape
		}
	}
	return nil
}
