//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
)

func (*operation1ResidentSource) requiresSetupPhaseSnapshots() {}

// Execution remains mechanically unreleased. This producer holds the original
// file mutex across before/after OS identity and independent AF/raw/sys reads.
func (s *operation1ScopedDiagnostic) observeSetupPhase(ctx context.Context, t *setupPhaseTicket) (result *setupPhaseObservation, err error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if s == nil || s.scope == nil || s.source == nil || s.source.resources == nil {
		return nil, afPhase
	}
	scope := s.scope
	o := s.source.resources
	if scope.owner == nil || !scope.owner.adapter.authorized(ctx) || !scope.owner.adapter.setupTicketCurrent(t) || o.owner != scope.owner.tx || o.session != scope.owner.adapter.session || o.namespace != scope.ns || o.ledger != t.ledger || o.creation == nil || o.creation.fd != t.attempt.custody {
		return nil, afPhase
	}
	if e := scope.check(ctx, t.attempt); e != nil {
		return nil, e
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	if before.identity != t.identity {
		return nil, afPhase
	}
	if scope.afOwner == nil {
		scope.afOwner = &afSourcePassOwner{controller: scope.owner, active: true}
	}
	socket, e := openRouteSocket(scope.ns)
	if e != nil {
		return nil, e
	}
	defer func() {
		err = errors.Join(err, socket.Close())
		if err != nil {
			result = nil
			scope.afOwner.last = nil
		}
	}()
	scope.afOwner.port = socket.pid
	scope.afOwner.ownedSetup = o.session.canonical
	defer func() { scope.afOwner.ownedSetup = nil }()
	pass, e := collectRouteAFPass(ctx, scope.afOwner, socket)
	if e != nil {
		return nil, e
	}
	good := false
	defer func() {
		if !good {
			scope.afOwner.last = nil
		}
	}()
	raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: scope.ns}, handle: *o.session.canonical}, scope.ns.checkCurrent)
	if e != nil {
		return nil, e
	}
	if !exactSetupPostMatches(RawInventory{Links: pass.links}, RawInventory{Links: raw.Links}) {
		return nil, afPolicy
	}
	sys, e := readPolicySysctlsSource(ctx, scope.ns, raw)
	if e != nil {
		return nil, e
	}
	pass.sys = cloneSysExact(sys)
	pass.effective, e = parseEffectiveV4(sys)
	if e != nil {
		return nil, e
	}
	if _, e = pass.requestClaim(); e != nil {
		return nil, e
	}
	after, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	if before.identity != after.identity || before.deviceNamespace != after.deviceNamespace || before.tunName != after.tunName {
		return nil, afPhase
	}
	held, e := heldSourceSnapshot(o.ledger, o.owner, o.session)
	if e != nil {
		return nil, e
	}
	if held.identity != t.identity || !scope.owner.adapter.setupTicketCurrent(t) {
		return nil, afPhase
	}
	if e = scope.check(ctx, t.attempt); e != nil {
		return nil, e
	}
	good = true
	// Full AF request provenance and original OS identity are checked here. The
	// pure consumer separately checks phase/baseline/transition-specific operands.
	return &setupPhaseObservation{ticket: t, diagnostic: ownedSourceDiagnostic{raw: cloneRawExact(raw), sys: cloneSysExact(sys), held: held, lineageUsable: true, afPass: pass}}, nil
}

type ownedSetupDump struct {
	residentNetlink
	handle Handle
}

func (d ownedSetupDump) decodeOwnedSetupLink(b []byte) (LinkRecord, error) {
	return decodeSetupLink(b, d.handle)
}

// Before the creation handle has an ifindex, the exclusive tracked creation
// reads the only candidate TUN by exact preauthorized name, never adopts it.
// Original OS verification/ledger retention still follow before phase admission.
type creationSetupDump struct {
	residentNetlink
	name string
}

func (d creationSetupDump) decodeOwnedSetupLink(b []byte) (LinkRecord, error) {
	if len(b) < 16 {
		return LinkRecord{}, afShape
	}
	if binary.NativeEndian.Uint16(b[2:]) != 65534 {
		return DecodeLink(b)
	}
	r, e := parseSetupOuter(b)
	if e != nil {
		return LinkRecord{}, e
	}
	if r.Name != d.name {
		return LinkRecord{}, afPhase
	}
	r.setupOuter = &setupOuterRecord{body: append([]byte(nil), b...)}
	return r, nil
}
