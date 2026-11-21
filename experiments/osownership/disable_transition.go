//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"reflect"
	"sync/atomic"
)

// Private resident lease. Revocation is terminal. Not namespace writer exclusion.
type disableResidentLifetime struct{ revoked atomic.Bool }

func (l *disableResidentLifetime) current() bool { return l != nil && !l.revoked.Load() }
func (l *disableResidentLifetime) revoke() {
	if l != nil {
		l.revoked.Store(true)
	}
}

type postDisableResult struct {
	lifetime *disableResidentLifetime
	ledger   *heldPacketLedger
	identity heldPacketIdentity
	raw      RawInventory
	sys      SysctlSnapshot
}

func (r *postDisableResult) current() bool {
	return r != nil && r.lifetime != nil && r.lifetime.current() && r.ledger != nil && r.ledger.matches(r.identity)
}

// No Ready method, no Inventory conversion, no actual packet supplier. Any later
// supplier constructor MUST accept this private result and resident lifetime.
type ownedDisableDependencies struct {
	check   func() error
	leaf    func() (ownedDisableLeaf, error)
	collect func(context.Context) (RawInventory, SysctlSnapshot, error)
}

// Caller holds the controller ownership lane and original fileCustody.mu for
// this COMPLETE call, including lineage restoration. The source wrapper checks
// its owner lease and holds that file mutex through return. Lifetime validity
// remains independently atomic for result.current readers outside that lane.
func finishOwnedDisable(ctx context.Context, life *disableResidentLifetime, l *heldPacketLedger, d ownedSourceDiagnostic, deps ownedDisableDependencies) (result *postDisableResult, err error) {
	if ctx == nil || ctx.Err() != nil || life == nil || !life.current() || l == nil || d.held == nil || d.held.ledger != l || !d.lineageUsable || deps.check == nil || deps.leaf == nil || deps.collect == nil {
		return nil, afPhase
	}
	h := d.held
	if h.session == nil || h.owner == nil || h.session.owner != h.owner || h.owner.stopStarted || h.owner.fdCloseAttempted || h.session.canonical == nil || h.session.forwardStep != 2 || !h.session.canonical.TagConfirmed || h.session.pending != nil || h.session.poisoned || h.session.cleanupMode || h.session.closeAttempted || !l.matches(h.identity) {
		return nil, afPhase
	}
	if h.owner.clock == nil || h.owner.deadline.expired(h.owner.clock.Now()) || !sameSessionHandle(h.identity.handle, *h.session.canonical) {
		return nil, afPhase
	}
	found := false
	for _, link := range d.raw.Links {
		if link.Index == h.session.canonical.Link.Index {
			if link.Name != h.session.canonical.Link.Name || link.Alias != h.session.canonical.Link.Alias || link.MTU != h.session.canonical.Link.MTU {
				return nil, afPhase
			}
			found = true
		}
	}
	if !found {
		return nil, afPhase
	}
	expected, sys, e := predictFreshDownDisable(d.raw, d.sys, h.session.canonical.Link.Index)
	if e != nil {
		return nil, e
	}
	before := d.sys["/proc/sys/net/ipv6/conf/"+h.session.canonical.Link.Name+"/disable_ipv6"]
	if e = deps.check(); e != nil {
		return nil, e
	}
	l.mu.Lock()
	if !l.matchesLocked(h.identity) || l.setup != nil || l.current.generation >= ^uint64(0)-1 {
		l.mu.Unlock()
		return nil, afPhase
	}
	original := l.current
	if e = l.invalidateLocked("config-intent"); e != nil {
		l.mu.Unlock()
		return nil, e
	}
	generation := l.current.generation
	l.mu.Unlock()
	// Any failure remains uncertain. There is no retry or recovery-by-name.
	check := func() error {
		if ctx.Err() != nil || !life.current() || h.owner.stopStarted || h.owner.fdCloseAttempted || h.owner.deadline.expired(h.owner.clock.Now()) {
			return afPhase
		}
		l.mu.Lock()
		ok := l.current.generation == generation && !l.current.closed && l.current.fdObject == original.fdObject && l.current.uncertain
		l.mu.Unlock()
		if !ok {
			return afPhase
		}
		return deps.check()
	}
	if e = check(); e != nil {
		return nil, e
	}
	leaf, e := deps.leaf()
	if e != nil {
		return nil, e
	}
	if e = disableOwnedLeafExpected(leaf, before, check); e != nil {
		return nil, e
	}
	if e = check(); e != nil {
		return nil, e
	}
	raw, observed, e := deps.collect(ctx)
	if e != nil {
		return nil, e
	}
	if e = check(); e != nil {
		return nil, e
	}
	if !exactSetupPostMatches(raw, expected) || !reflect.DeepEqual(observed, sys) {
		return nil, ErrConflict
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.current.generation != generation || l.current.closed || l.current.fdObject != original.fdObject || !life.current() {
		return nil, afPhase
	}
	l.current.uncertain = false
	l.current.generation++
	return &postDisableResult{lifetime: life, ledger: l, identity: l.current, raw: cloneRawExact(raw), sys: cloneSysExact(observed)}, nil
}
