//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
)

// Private exact one-use address permit. Not interchangeable with alias permit.
// Original file mutex and controller lane span construction through finish.
type addressSendPermit struct {
	disabled   *postDisableResult
	original   heldPacketIdentity
	generation uint64
	request    []byte
	used       bool
}

func (p *addressSendPermit) currentLocked() bool {
	if p == nil || p.disabled == nil {
		return false
	}
	d := p.disabled
	l := d.ledger
	return l != nil && d.lifetime.current() && d.owner != nil && !d.owner.Ready && d.session != nil && d.session.owner == d.owner && !d.owner.stopStarted && !d.owner.fdCloseAttempted && d.owner.clock != nil && !d.owner.deadline.expired(d.owner.clock.Now()) && d.session != nil && d.session.forwardStep == 3 && !d.session.poisoned && !d.session.cleanupMode && !d.session.closeAttempted && l.current.generation == p.generation && l.current.uncertain && !l.current.closed && sameDisableOriginalIdentity(l.current, p.original) && l.setup == nil
}
func (p *addressSendPermit) send(raw []byte, pid uint32, effect func() error) error {
	if p == nil || p.disabled == nil || p.disabled.ledger == nil || effect == nil {
		return afPhase
	}
	l := p.disabled.ledger
	l.mu.Lock()
	if p.used || !p.currentLocked() || len(raw) < 24 || !bytes.Equal(raw, p.request) || binary.NativeEndian.Uint16(raw[4:]) != 20 || binary.NativeEndian.Uint32(raw[12:]) != pid {
		l.mu.Unlock()
		return afPhase
	}
	flags, e := mutationRequestFlags(20)
	if e != nil || binary.NativeEndian.Uint16(raw[6:]) != flags {
		l.mu.Unlock()
		return afPhase
	}
	p.used = true
	l.mu.Unlock()
	return effect()
}
func (p *addressSendPermit) current() bool {
	if p == nil || p.disabled == nil || p.disabled.ledger == nil {
		return false
	}
	l := p.disabled.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	return p.currentLocked()
}
func (p *addressSendPermit) ack(d Datagram) bool {
	b := d.Bytes
	return p != nil && len(p.request) >= 16 && d.SenderPID == 0 && !d.Truncated && len(b) == 36 && binary.NativeEndian.Uint32(b) == 36 && binary.NativeEndian.Uint16(b[4:]) == 2 && binary.NativeEndian.Uint16(b[6:]) == 0x100 && bytes.Equal(b[8:16], p.request[8:16]) && binary.NativeEndian.Uint32(b[16:]) == 0 && bytes.Equal(b[20:], p.request[:16])
}

type postAddressDownResult struct {
	disabledAncestry *postDisableResult
	lifetime         *disableResidentLifetime
	ledger           *heldPacketLedger
	identity         heldPacketIdentity
	address          *ownedAddressDumpCandidate
	raw              RawInventory
	sys              SysctlSnapshot
	consumed         bool
}

func (r *postAddressDownResult) current() bool {
	return r != nil && r.disabledAncestry != nil && r.lifetime == r.disabledAncestry.lifetime && r.lifetime.current() && r.ledger != nil && r.ledger.matches(r.identity) && r.address != nil
}

// Retained legacy refusal entry cannot mint a permit or advance a phase.
func refuseAddressDownSend(context.Context, *postDisableResult, *ownedAddressDumpCandidate) (*postAddressDownResult, error) {
	return nil, afPhase
}
func (*postAddressDownResult) admitStep4(context.Context) error { return afPhase }

type ownedAddressDependencies struct {
	check   func() error
	send    func(context.Context, *addressSendPermit) (Datagram, error)
	collect func(context.Context) (RawInventory, SysctlSnapshot, error)
}

// No network operation under ledger lock. Caller holds original custody across
// verifier/prestate/invalidation/one send/ACK/independent post capture/restore.
// Error after invalidation never retries or restores uncertain lineage.
func finishOwnedAddressDown(ctx context.Context, d *postDisableResult, pre ownedSourceDiagnostic, seq, pid uint32, deps ownedAddressDependencies) (result *postAddressDownResult, err error) {
	if ctx == nil || ctx.Err() != nil || d == nil || !d.current() || !d.consumed || deps.check == nil || deps.send == nil || deps.collect == nil || seq == 0 || pid == 0 || pre.held == nil || !pre.lineageUsable {
		return nil, afPhase
	}
	s := d.session
	o := d.owner
	l := d.ledger
	if s == nil || o == nil || s.owner != o || s.canonical == nil || s.plan == nil || len(s.plan.Operations) != 6 || s.forwardStep != 3 || !reflect.DeepEqual(*s.plan, o.plan) || !s.canonical.TagConfirmed || s.pending != nil || s.poisoned || s.cleanupMode || s.closeAttempted || pre.held.owner != o || pre.held.session != s || pre.held.ledger != l || pre.held.identity != d.identity || !exactSetupPostMatches(pre.raw, d.raw) {
		return nil, afPhase
	}
	h := *s.canonical
	if !sameSessionHandle(h, d.identity.handle) || requireAddressDownLink(&h, pre.raw, pre.sys) != nil || len(pre.raw.Addresses) != 0 || len(pre.raw.Routes) != 0 {
		return nil, afPhase
	}
	m, e := BuildExactMutation(h, s.plan.Operations[3])
	if e != nil || m.kind != 20 {
		return nil, afPhase
	}
	flags, e := mutationRequestFlags(20)
	if e != nil {
		return nil, e
	}
	request := make([]byte, 16+len(m.payload))
	binary.NativeEndian.PutUint32(request, uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:], 20)
	binary.NativeEndian.PutUint16(request[6:], flags)
	binary.NativeEndian.PutUint32(request[8:], seq)
	binary.NativeEndian.PutUint32(request[12:], pid)
	copy(request[16:], m.payload)
	if e = deps.check(); e != nil {
		return nil, e
	}
	l.mu.Lock()
	if !l.matchesLocked(d.identity) || l.setup != nil || l.current.generation >= ^uint64(0)-1 {
		l.mu.Unlock()
		return nil, afPhase
	}
	original := l.current
	if e = l.invalidateLocked("config-intent"); e != nil {
		l.mu.Unlock()
		return nil, e
	}
	permit := &addressSendPermit{disabled: d, original: original, generation: l.current.generation, request: request}
	l.mu.Unlock()
	check := func() error {
		if ctx.Err() != nil || !permit.current() {
			return afPhase
		}
		return deps.check()
	}
	if e = check(); e != nil {
		return nil, e
	}
	reply, e := deps.send(ctx, permit)
	if e != nil {
		return nil, e
	}
	if !permit.used || !permit.ack(reply) {
		return nil, afPhase
	}
	if e = check(); e != nil {
		return nil, e
	}
	raw, sys, e := deps.collect(ctx)
	if e != nil {
		return nil, e
	}
	if e = check(); e != nil {
		return nil, e
	}
	address, e := captureAddressDownDelta(&h, pre.raw, pre.sys, raw, sys)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(pre.sys, sys) {
		return nil, afPolicy
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !permit.currentLocked() {
		return nil, afPhase
	}
	l.current.generation++
	l.current.uncertain = false
	return &postAddressDownResult{disabledAncestry: d, lifetime: d.lifetime, ledger: l, identity: l.current, address: address, raw: cloneRawExact(raw), sys: cloneSysExact(sys)}, nil
}
