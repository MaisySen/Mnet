//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"reflect"
	"strings"
)

// Private transition-only read. Not a packet supplier or general AF waiver.
// The adapter mints one ticket per Snapshot while the controller lane is held.
type setupPhaseCreationSource interface{ requiresSetupPhaseSnapshots() }

type setupPhaseSource interface {
	observeSetupPhase(context.Context, *setupPhaseTicket) (*setupPhaseObservation, error)
}
type setupPhaseTicket struct {
	adapter      *operation1Adapter
	attempt      *originalAttempt
	ledger       *heldPacketLedger
	identity     heldPacketIdentity
	sequence     uint64
	step         int
	active, used bool
}
type setupPhaseObservation struct {
	ticket     *setupPhaseTicket
	diagnostic ownedSourceDiagnostic
}

func (a *operation1Adapter) setupTicketCurrent(t *setupPhaseTicket) bool {
	if a == nil || !a.phaseCreation || t == nil || !t.active || t.used || t.adapter != a || t.sequence == 0 || t.sequence != a.phaseSequence || t.step < 1 || t.step > 3 || a.controller == nil || a.controller.tx == nil || a.session == nil || a.session.canonical == nil || a.session.plan == nil || a.controller.attempt == nil || t.attempt != a.controller.attempt || t.attempt.owner != a.controller || t.attempt.custody == nil || t.attempt.handle == nil || !sameSessionHandle(*t.attempt.handle, *a.session.canonical) || t.attempt.closeAttempted || t.attempt.stage != "session-bound" || t.attempt.custody.lineage != t.ledger || a.session.owner != a.controller.tx || a.session.forwardStep != t.step || a.session.pending != nil || a.session.poisoned || a.session.cleanupMode || a.session.closeAttempted || a.controller.tx.stopStarted || a.controller.tx.fdCloseAttempted || a.controller.tx.Ready || t.ledger == nil || !t.ledger.matches(t.identity) || !sameSessionHandle(t.identity.handle, *a.session.canonical) || t.identity.identity != a.session.identity || a.controller.tx.clock == nil || a.controller.tx.deadline.expired(a.controller.tx.clock.Now()) {
		return false
	}
	if !reflect.DeepEqual(*a.session.plan, t.attempt.plan) || !reflect.DeepEqual(a.controller.tx.plan, t.attempt.plan) {
		return false
	}
	switch t.step {
	case 1:
		return t.identity.generation == 1 && !a.session.canonical.TagConfirmed && a.aliasPost == nil && !a.disableAttempted && a.disabled == nil
	case 2:
		if a.aliasPost == nil || a.aliasPost.post == nil {
			return false
		}
		expected := a.aliasPost.post.identity
		expected.generation++
		expected.uncertain = false
		expected.handle.TagConfirmed = true
		return t.identity == expected && a.session.canonical.TagConfirmed && a.aliasPost != nil && a.aliasPost.consumed && a.aliasPost.token != nil && a.aliasPost.token.done && a.aliasPost.token.owner == a.controller.tx && a.aliasPost.post != nil && !a.disableAttempted && a.disabled == nil
	case 3:
		if a.aliasPost == nil || a.aliasPost.post == nil || a.aliasPost.token == nil || !a.aliasPost.token.done || a.aliasPost.token.owner != a.controller.tx || a.disabled == nil || a.disabled.ledger != t.ledger {
			return false
		}
		aliasIdentity := a.aliasPost.post.identity
		aliasIdentity.handle.TagConfirmed = true
		if !sameDisableOriginalIdentity(aliasIdentity, t.identity) {
			return false
		}
		return a.session.canonical.TagConfirmed && a.aliasPost != nil && a.aliasPost.consumed && a.disableAttempted && a.disabled != nil && a.disabled.consumed && a.disabled.owner == a.controller.tx && a.disabled.session == a.session && a.disabled.current() && a.disabled.identity == t.identity
	}
	return false
}
func (a *operation1Adapter) snapshotSetupPhase(ctx context.Context, source setupPhaseSource) (out Inventory, err error) {
	defer func() {
		if err != nil && a.session != nil {
			a.session.poisoned = true
		}
	}()
	if !a.authorized(ctx) || a.controller.attempt == nil || a.controller.attempt.custody == nil || a.phaseSequence == ^uint64(0) {
		return Inventory{}, afPhase
	}
	l := a.controller.attempt.custody.lineage
	if l == nil {
		return Inventory{}, afPhase
	}
	a.phaseSequence++
	ticket := &setupPhaseTicket{adapter: a, attempt: a.controller.attempt, ledger: l, identity: l.snapshot(), sequence: a.phaseSequence, step: a.session.forwardStep, active: true}
	defer func() { ticket.active = false }()
	if !a.setupTicketCurrent(ticket) {
		return Inventory{}, afPhase
	}
	o, e := source.observeSetupPhase(ctx, ticket)
	if e != nil {
		return Inventory{}, e
	}
	if ctx.Err() != nil {
		return Inventory{}, ctx.Err()
	}
	return a.consumeSetupPhase(ticket, o)
}
func (a *operation1Adapter) consumeSetupPhase(t *setupPhaseTicket, o *setupPhaseObservation) (Inventory, error) {
	if !a.setupTicketCurrent(t) || o == nil || o.ticket != t {
		return Inventory{}, afPhase
	}
	d := o.diagnostic
	h := d.held
	if h == nil || h.owner != a.controller.tx || h.session != a.session || h.ledger != t.ledger || h.identity != t.identity || !d.lineageUsable {
		return Inventory{}, afPhase
	}
	t.used = true
	i, e := a.composeSetupOnly(t.step, d)
	if e != nil {
		return Inventory{}, e
	}
	if !t.ledger.matches(t.identity) || a.session.forwardStep != t.step || a.controller.tx.stopStarted || a.controller.tx.fdCloseAttempted || a.controller.tx.deadline.expired(a.controller.tx.clock.Now()) {
		return Inventory{}, afPhase
	}
	// No capability escapes: Inventory is consumed by Transaction's synchronous
	// step checks. Later Apply revalidates this exact admitted ledger identity.
	a.phaseAdmitted = t.step
	a.phaseIdentity = t.identity
	return i, nil
}
func (a *operation1Adapter) setupAdmissionCurrent(step int) bool {
	return a.phaseAdmitted == step && a.controller.attempt != nil && a.controller.attempt.custody != nil && a.controller.attempt.custody.lineage != nil && a.controller.attempt.custody.lineage.matches(a.phaseIdentity)
}

// Only C, post-alias C, and verified D. No address/UP/route/traffic readiness.
// All93 diagnostic words remain unknown as runtime policy; only the bounded
// no-supplier DOWN graph permits their consumers to be out of scope here.
func (a *operation1Adapter) composeSetupOnly(step int, d ownedSourceDiagnostic) (Inventory, error) {
	s := a.session
	b := s.baseline
	h := *s.canonical
	if step < 1 || step > 3 || b == nil || len(d.raw.Addresses) != 0 || len(d.raw.Routes) != 0 {
		return Inventory{}, afPhase
	}
	want := h.Link
	if step == 1 {
		want = h.CreationLink
	}
	var link *LinkRecord
	rest := cloneRawExact(d.raw)
	rest.Links = nil
	for _, l := range d.raw.Links {
		if l.Index == want.Index || l.Name == want.Name || l.Alias == h.Link.Alias {
			if link != nil || l.Index != want.Index || l.Name != want.Name || l.Alias != want.Alias || l.MTU != want.MTU || l.Hardware != 65534 || l.Flags != 0x1090 {
				return Inventory{}, afPhase
			}
			copy := l
			link = &copy
		} else {
			rest.Links = append(rest.Links, l)
		}
	}
	if link == nil {
		return Inventory{}, afPhase
	}
	// Deliberately narrow full shape. Unknown outer attrs are not erased to get
	// through baseline Compose. Future emitter profiles need their own review.
	if link.setupOuter != nil {
		if e := validateSetupOuter(*link); e != nil {
			return Inventory{}, e
		}
	} else {
		for k := range link.Raw {
			if k != 3 && k != 4 && k != 18 && k != 20 && k != 26 {
				return Inventory{}, afShape
			}
		}
	}
	name, e := attrText(link.Raw, 3, true)
	if e != nil || name != want.Name {
		return Inventory{}, afShape
	}
	mtu, e := attrUint(link.Raw, 4, true)
	if e != nil || mtu != want.MTU {
		return Inventory{}, afShape
	}
	alias, e := attrText(link.Raw, 20, false)
	if e != nil || alias != want.Alias {
		return Inventory{}, afShape
	}
	if _, e = parseTUNNested(*link); e != nil {
		return Inventory{}, e
	}
	af, e := freshDisableAF(link.Raw[26])
	if e != nil {
		return Inventory{}, e
	}
	if binary.NativeEndian.Uint32(af[1]) != 0 || binary.NativeEndian.Uint32(af[2][8:]) != want.MTU {
		return Inventory{}, afPhase
	}
	disabled := binary.NativeEndian.Uint32(af[2][26*4:])
	path := "/proc/sys/net/ipv6/conf/" + want.Name + "/disable_ipv6"
	if d.sys[ipv6DirPrefix+want.Name] != "present" {
		return Inventory{}, afDirectory
	}
	if disabled > 1 || (disabled == 0 && d.sys[path] != "0") || (disabled == 1 && d.sys[path] != "1") {
		return Inventory{}, afPolicy
	}
	if step == 3 && disabled != 1 {
		return Inventory{}, afPhase
	}
	// Local observations are never substituted for ALL/default or baseline state.
	// Strip only the owned directory when comparing the unowned source baseline.
	sys := cloneSysExact(d.sys)
	for k := range sys {
		if strings.HasPrefix(k, "/proc/sys/net/ipv4/conf/"+want.Name+"/") || strings.HasPrefix(k, "/proc/sys/net/ipv6/conf/"+want.Name+"/") || k == ipv6DirPrefix+want.Name {
			delete(sys, k)
		}
	}
	if !originalBaselineMatches(*s.plan, b, rest, sys) {
		return Inventory{}, ErrConflict
	}
	switch step {
	case 2:
		if a.aliasPost == nil || a.aliasPost.post == nil || !exactSetupPostMatches(d.raw, a.aliasPost.post.raw) || !reflect.DeepEqual(d.sys, a.aliasPost.post.sys) {
			return Inventory{}, afPhase
		}
	case 3:
		if a.disabled == nil || !exactSetupPostMatches(d.raw, a.disabled.raw) {
			return Inventory{}, afPhase
		}
		expected := cloneSysExact(a.aliasPost.post.sys)
		expected[path] = "1"
		if !reflect.DeepEqual(d.sys, expected) {
			return Inventory{}, afPolicy
		}
	}
	normalized, e := comparableSetupRaw(rest)
	if e != nil {
		return Inventory{}, e
	}
	baseRaw, e := comparableSetupRaw(b.raw)
	if e != nil {
		return Inventory{}, e
	}
	baseline := *b
	baseline.raw = baseRaw
	i, e := baseline.Compose(normalized, sys, nil, false)
	if e != nil {
		return Inventory{}, e
	}
	// Exact observed DOWN link, not an AF safety or packet-readiness assertion.
	i.Links = append(i.Links, Link{Index: link.Index, Name: link.Name, Alias: link.Alias, Kind: "tun", MTU: link.MTU})
	i.LinkUp[link.Index] = false
	i.IPv6Disabled[link.Index] = disabled == 1
	return i, nil
}

// Called only while Transaction.Run retains the active controller lease. No
// generic rollback can reopen/read a closed original or bypass absence tickets.
func (a *operation1Adapter) stopOwnedTransaction(ctx context.Context) error {
	if !a.authorized(ctx) {
		return ErrConflict
	}
	return a.controller.stopOwnedLocked(ctx)
}
