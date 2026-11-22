//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
)

type upSendPermit struct {
	prior      *postAddressDownResult
	original   heldPacketIdentity
	generation uint64
	request    []byte
	used       bool
}

func (p *upSendPermit) currentLocked() bool {
	if p == nil || p.prior == nil || p.prior.disabledAncestry == nil {
		return false
	}
	r := p.prior
	d := r.disabledAncestry
	l := r.ledger
	return l != nil && r.lifetime.current() && d.owner != nil && !d.owner.Ready && !d.owner.stopStarted && !d.owner.fdCloseAttempted && d.owner.clock != nil && !d.owner.deadline.expired(d.owner.clock.Now()) && d.session != nil && d.session.owner == d.owner && d.session.forwardStep == 4 && !d.session.poisoned && !d.session.cleanupMode && !d.session.closeAttempted && l.current.uncertain && !l.current.closed && l.current.generation == p.generation && sameDisableOriginalIdentity(l.current, p.original) && l.setup == nil
}
func (p *upSendPermit) send(raw []byte, pid uint32, effect func() error) error {
	if p == nil || p.prior == nil || p.prior.ledger == nil || effect == nil {
		return afPhase
	}
	l := p.prior.ledger
	l.mu.Lock()
	if p.used || !p.currentLocked() || !bytes.Equal(raw, p.request) || len(raw) < 32 || binary.NativeEndian.Uint16(raw[4:]) != 16 || binary.NativeEndian.Uint16(raw[6:]) != 5 || binary.NativeEndian.Uint32(raw[12:]) != pid {
		l.mu.Unlock()
		return afPhase
	}
	p.used = true
	l.mu.Unlock()
	return effect()
}
func (p *upSendPermit) current() bool {
	if p == nil || p.prior == nil || p.prior.ledger == nil {
		return false
	}
	l := p.prior.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	return p.currentLocked()
}

type postUpResult struct {
	ancestry *postAddressDownResult
	lifetime *disableResidentLifetime
	ledger   *heldPacketLedger
	identity heldPacketIdentity
	raw      RawInventory
	sys      SysctlSnapshot
	qdisc    upQdiscObservation
	consumed bool
}

func (r *postUpResult) current() bool {
	return r != nil && r.ancestry != nil && r.lifetime == r.ancestry.lifetime && r.lifetime.current() && r.ledger != nil && r.ledger.matches(r.identity)
}

type ownedUpDependencies struct {
	check   func() error
	send    func(context.Context, *upSendPermit) (Datagram, error)
	collect func(context.Context) (RawInventory, SysctlSnapshot, upQdiscObservation, error)
}

// Original file mutex/controller lane span caller's full invocation. Only UP
// plus source-derived operational/qdisc changes may differ. Not runtime Ready.
func finishOwnedUp(ctx context.Context, r *postAddressDownResult, pre ownedSourceDiagnostic, qbefore upQdiscObservation, seq, pid uint32, deps ownedUpDependencies) (*postUpResult, error) {
	if ctx == nil || ctx.Err() != nil || r == nil || !r.current() || !r.consumed || r.disabledAncestry == nil || seq == 0 || pid == 0 || deps.check == nil || deps.send == nil || deps.collect == nil || pre.held == nil || !pre.lineageUsable {
		return nil, afPhase
	}
	d := r.disabledAncestry
	s := d.session
	o := d.owner
	l := r.ledger
	if s == nil || o == nil || s.canonical == nil || s.plan == nil || len(s.plan.Operations) != 6 || !reflect.DeepEqual(*s.plan, o.plan) || s.forwardStep != 4 || s.pending != nil || pre.held.identity != r.identity || pre.held.owner != o || pre.held.session != s || pre.held.ledger != l || !exactSetupPostMatches(pre.raw, r.raw) || !reflect.DeepEqual(pre.sys, r.sys) || requireAddressDownLink(s.canonical, pre.raw, pre.sys) != nil {
		return nil, afPhase
	}
	for _, b := range qbefore.bodies {
		if len(b) < 20 || int(int32(binary.NativeEndian.Uint32(b[4:]))) == s.canonical.Link.Index {
			return nil, afPhase
		}
	}
	m, e := BuildExactMutation(*s.canonical, s.plan.Operations[4])
	if e != nil || m.kind != 16 {
		return nil, afPhase
	}
	request := make([]byte, 16+len(m.payload))
	binary.NativeEndian.PutUint32(request, uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:], 16)
	binary.NativeEndian.PutUint16(request[6:], 5)
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
	p := &upSendPermit{prior: r, original: original, generation: l.current.generation, request: request}
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
	if e = validateUpDelta(*s.canonical, pre.raw, pre.sys, raw, sys, qbefore, q); e != nil {
		return nil, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !p.currentLocked() {
		return nil, afPhase
	}
	l.current.generation++
	l.current.uncertain = false
	return &postUpResult{ancestry: r, lifetime: r.lifetime, ledger: l, identity: l.current, raw: cloneRawExact(raw), sys: cloneSysExact(sys), qdisc: cloneUpQdisc(q)}, nil
}
func cloneUpQdisc(q upQdiscObservation) upQdiscObservation {
	out := upQdiscObservation{}
	for _, b := range q.bodies {
		out.bodies = append(out.bodies, append([]byte(nil), b...))
	}
	return out
}
func validateUpDelta(h Handle, before RawInventory, beforeSys SysctlSnapshot, after RawInventory, sys SysctlSnapshot, qbefore, qafter upQdiscObservation) error {
	if !reflect.DeepEqual(beforeSys, sys) || len(after.Addresses) != 1 || len(after.Routes) != 1 || validateAddressLocalRoute(&h, after.Routes[0]) != nil {
		return afPhase
	}
	if _, e := captureOwnedAddressRecordCandidate(&h, after.Addresses[0]); e != nil {
		return e
	}
	q, e := parseUpQdisc(qafter, h.Link.Index)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(q.foreign, qbefore.bodies) {
		return afPhase
	}
	normalized := cloneRawExact(after)
	found := false
	for i := range normalized.Links {
		l := &normalized.Links[i]
		if l.Index != h.Link.Index {
			continue
		}
		if found || l.Flags != 0x110d1 || l.Name != h.Link.Name || l.Alias != h.Link.Alias || l.MTU != h.Link.MTU || l.Hardware != 65534 {
			return afPhase
		}
		found = true
		// Require complete actual full-emitter evidence at this activation boundary.
		if validateSetupOuter(*l) != nil {
			return afShape
		}
		af, e := freshDisableAF(l.Raw[26])
		if e != nil || binary.NativeEndian.Uint32(af[1]) != 0 || binary.NativeEndian.Uint32(af[2][26*4:]) != 1 || sys["/proc/sys/net/ipv6/conf/"+l.Name+"/disable_ipv6"] != "1" {
			return afPhase
		}
		var old *LinkRecord
		for j := range before.Links {
			if before.Links[j].Index == l.Index {
				old = &before.Links[j]
			}
		}
		if old == nil || old.Flags != 0x1090 {
			return afPhase
		}
		// Only explicitly reviewed flags/OPER/qdisc delta, not generic attr masking.
		l.Flags = old.Flags
		l.Raw[16] = append([]byte(nil), old.Raw[16]...)
		if value, ok := old.Raw[6]; ok {
			l.Raw[6] = append([]byte(nil), value...)
		} else {
			delete(l.Raw, 6)
		}
	}
	if !found || !exactSetupPostMatches(normalized, before) {
		return afPhase
	}
	return nil
}
