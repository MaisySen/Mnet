//go:build mnet_enrollment_development

package osownership

// adapterSession is an inert bookkeeping component, NOT Kernel and not a live
// executor. It owns canonical handle/phase state so caller-supplied stale or
// promoted Handle values cannot silently widen a future adapter's authority.
// No namespace, device, FD, sysctl, netlink or process operations here.
import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"time"
)

type adapterSession struct {
	baseline         *CapturedBaseline
	plan             *Plan
	forwardStep      int
	cleanupMode      bool
	cleanupPending   *Operation
	closeRecorded    bool
	canonical        *Handle
	pending          *Operation
	pendingACK       bool
	poisoned         bool
	addressConfirmed bool
	closeAttempted   bool
	closeConfirmed   bool
}

func newAdapterSession(b *CapturedBaseline) (*adapterSession, error) {
	if b == nil || !sameNamespace(b.ns, b.ns) {
		return nil, ErrConflict
	}
	data, e := json.Marshal(b.raw)
	if e != nil {
		return nil, e
	}
	var raw RawInventory
	if e = json.Unmarshal(data, &raw); e != nil {
		return nil, e
	}
	frozen := *b
	frozen.raw = raw
	frozen.sys = maps.Clone(b.sys)
	return &adapterSession{baseline: &frozen}, nil
}
func (s *adapterSession) bindCreated(p Plan, h Handle) error {
	if s == nil || s.baseline == nil {
		return ErrConflict
	}
	i, e := s.baseline.Compose(s.baseline.raw, SysctlSnapshot(s.baseline.sys), nil, false)
	if e != nil {
		return e
	}
	expectedPlan, e := Prepare(p.Binding, i, p.EffectiveExpiry.Add(-time.Nanosecond))
	if e != nil || !reflect.DeepEqual(expectedPlan, p) {
		return ErrConflict
	}
	expected, e := AttachCreated(p, h.Link.Index, h.CreationLink.MTU)
	if e != nil || expected != h {
		return ErrConflict
	}
	if s == nil || s.baseline == nil || s.canonical != nil || s.poisoned || s.closeAttempted || h.TagConfirmed || !sameNamespace(h.Namespace, s.baseline.ns) || h.Link.Index <= 0 || h.CreationLink.Index != h.Link.Index || h.CreationLink.Name != h.Link.Name || h.CreationLink.Kind != "tun" || h.CreationLink.Alias != "" || h.CreationLink.MTU == 0 {
		return ErrConflict
	}
	copy := h
	s.canonical = &copy
	pp := p
	pp.Operations = append([]Operation(nil), p.Operations...)
	s.plan = &pp
	s.forwardStep = 1
	return nil
}
func sameSessionHandle(a, b Handle) bool {
	// Caller may lag successful tag confirmation, never lead it or change any
	// identity/creation field. Canonical state, not caller flag, drives compose.
	if a.TagConfirmed && !b.TagConfirmed {
		return false
	}
	a.TagConfirmed = b.TagConfirmed
	return a == b
}
func (s *adapterSession) begin(h Handle, op Operation) error {
	if s == nil || s.cleanupMode || s.poisoned || s.closeAttempted || s.canonical == nil || s.pending != nil || !sameSessionHandle(h, *s.canonical) {
		return ErrConflict
	}
	if s.plan == nil || s.forwardStep >= len(forwardTransitions) || op.Kind != forwardTransitions[s.forwardStep] || op != s.plan.Operations[s.forwardStep] {
		return ErrConflict
	}
	// Validate exact opaque encoder even for plans containing zero route ifindex.
	// Disable IPv6 uses separately scoped writer, never a netlink payload.
	if op.Kind == "disable-owned-link-ipv6" {
		if !s.canonical.TagConfirmed {
			return ErrConflict
		}
	} else {
		if _, e := BuildExactMutation(*s.canonical, op); e != nil {
			return e
		}
		if op.Kind != "set-alias-mtu-exact" && !s.canonical.TagConfirmed {
			return ErrConflict
		}
	}
	copy := op
	s.pending = &copy
	s.pendingACK = false
	return nil
}
func (s *adapterSession) recordACK(err error) error {
	if s == nil || s.pending == nil || s.poisoned {
		return ErrConflict
	}
	if err != nil {
		// Ambiguous error may have changed state. Refuse all further forward setup;
		// leave canonical ownership for read-only inspection and exact cleanup review.
		s.poisoned = true
		return err
	}
	s.pendingACK = true
	return nil
}
func (s *adapterSession) observe(i Inventory) error {
	if s == nil || s.cleanupMode || s.poisoned || s.canonical == nil || s.closeAttempted || !sameNamespace(i.Namespace, s.baseline.ns) {
		return ErrConflict
	}
	if e := exactHandle(*s.canonical, *s.plan, i); e != nil {
		return e
	}
	if s.pending == nil {
		return nil
	}
	if !s.pendingACK {
		return errors.New("operation needs ACK and exact readback")
	}
	if !stepPresent(*s.pending, *s.canonical, i) {
		return errors.New("ACK lacks exact operation readback")
	}
	switch s.pending.Kind {
	case "set-alias-mtu-exact":
		s.canonical.TagConfirmed = true
	case "address-add-exclusive":
		s.addressConfirmed = true
	}
	s.forwardStep++
	s.pending = nil
	s.pendingACK = false
	return nil
}
func (s *adapterSession) compose(raw RawInventory, sys SysctlSnapshot) (Inventory, error) {
	if s == nil || s.baseline == nil {
		return Inventory{}, ErrConflict
	}
	// Original custody handle persists even after close: foreign replacement must
	// still fail schema/identity checks; successful close alone proves no absence.
	return s.baseline.Compose(raw, sys, s.canonical, s.addressConfirmed)
}
func (s *adapterSession) beginClose(h Handle, i Inventory) error {
	if s == nil || s.canonical == nil || s.closeAttempted || s.cleanupPending != nil || !sameSessionHandle(h, *s.canonical) {
		return ErrConflict
	}
	if _, e := s.cleanupChoice(h, i, Operation{Kind: "close-owned-tun-fd"}); e != nil {
		return e
	}
	s.closeAttempted = true
	return nil
}
func (s *adapterSession) recordClose(err error) error {
	if s == nil || !s.closeAttempted || s.closeRecorded {
		return ErrConflict
	}
	s.closeRecorded = true
	if err != nil {
		s.poisoned = true
		return err
	}
	s.closeConfirmed = true
	return nil
}
func (s *adapterSession) absenceConfirmed(raw RawInventory, sys SysctlSnapshot) error {
	if s == nil || s.baseline == nil || !s.closeConfirmed {
		return ErrConflict
	}
	// Exact complete original raw baseline, rules and sysctls. No volatility
	// normalization, no name-only cleanup, no cross-run equivalence.
	if !reflect.DeepEqual(raw, s.baseline.raw) || !reflect.DeepEqual(map[string]string(sys), s.baseline.sys) {
		return ErrConflict
	}
	return nil
}

// Entering cleanup permanently forbids forward work, even if no mutation error.
// Fresh inventory selects ONLY the exact next Rollback operation. Pending forward
// state isn't thrown away as evidence; it no longer authorizes a forward retry.
func (s *adapterSession) cleanupChoice(h Handle, i Inventory, op Operation) ([]Operation, error) {
	if s == nil || s.canonical == nil || !sameSessionHandle(h, *s.canonical) || !sameNamespace(i.Namespace, s.baseline.ns) || i.ForeignPolicy || i.Forwarding {
		return nil, ErrConflict
	}
	s.cleanupMode = true
	ops, e := Rollback(*s.canonical, i)
	if e != nil {
		return nil, e
	}
	if op.Kind == "close-owned-tun-fd" {
		if len(ops) == 0 {
			return ops, nil
		}
		if len(ops) == 1 && ops[0].Kind == "close-owned-tun-fd" {
			return ops, nil
		}
		return nil, ErrConflict
	}
	if len(ops) == 0 || ops[0] != op {
		return nil, ErrConflict
	}
	if op.Kind != "route-del-exact" && op.Kind != "address-del-exact" {
		return nil, ErrConflict
	}
	if _, e = BuildExactMutation(*s.canonical, op); e != nil {
		return nil, e
	}
	return ops, nil
}
func (s *adapterSession) beginCleanup(h Handle, i Inventory, op Operation) error {
	if s == nil || s.closeAttempted || s.cleanupPending != nil {
		return ErrConflict
	}
	if _, e := s.cleanupChoice(h, i, op); e != nil {
		return e
	}
	copy := op
	s.cleanupPending = &copy
	return nil
}
func (s *adapterSession) observeCleanup(i Inventory) error {
	if s == nil || s.canonical == nil || s.cleanupPending == nil {
		return ErrConflict
	}
	if !sameNamespace(i.Namespace, s.baseline.ns) || i.ForeignPolicy || i.Forwarding {
		return ErrConflict
	}
	ops, e := Rollback(*s.canonical, i)
	if e != nil {
		return e
	}
	if operationPresent(*s.cleanupPending, ops) {
		return errors.New("cleanup remains present; custody retained")
	}
	if s.cleanupPending.Kind == "address-del-exact" {
		s.addressConfirmed = false
	}
	s.cleanupPending = nil
	return nil
}

// Independent transition table. The copied Plan must also match Prepare, so
// neither a caller-supplied Kind nor a malformed Plan can define this ordering.
var forwardTransitions = [6]string{"create-nonpersistent-tun", "set-alias-mtu-exact", "disable-owned-link-ipv6", "address-add-exclusive", "link-up-exact", "route-add-exclusive"}

type adapterFinalState struct {
	Poisoned          bool
	CleanupMode       bool
	PossiblyApplied   *Operation
	CleanupPending    *Operation
	CloseAttempted    bool
	CloseRecorded     bool
	CloseConfirmed    bool
	CloseAmbiguous    bool
	RawBaselineEqual  bool
	CustodyUnresolved bool
}

// Reconcile reads caller-supplied whole raw observations only, never closes or
// silently upgrades a close error to success. Baseline equality after a close
// error establishes observed network absence, NOT FD custody closure.
func (s *adapterSession) reconcile(raw RawInventory, sys SysctlSnapshot) (adapterFinalState, error) {
	if s == nil || s.baseline == nil {
		return adapterFinalState{}, ErrConflict
	}
	r := adapterFinalState{Poisoned: s.poisoned, CleanupMode: s.cleanupMode, CloseAttempted: s.closeAttempted, CloseRecorded: s.closeRecorded, CloseConfirmed: s.closeConfirmed, CloseAmbiguous: s.closeAttempted && (!s.closeRecorded || !s.closeConfirmed)}
	if s.pending != nil {
		copy := *s.pending
		r.PossiblyApplied = &copy
	}
	if s.cleanupPending != nil {
		copy := *s.cleanupPending
		r.CleanupPending = &copy
	}
	r.RawBaselineEqual = reflect.DeepEqual(raw, s.baseline.raw) && reflect.DeepEqual(map[string]string(sys), s.baseline.sys)
	r.CustodyUnresolved = s.canonical != nil && (!s.closeConfirmed || !r.RawBaselineEqual)
	if !r.RawBaselineEqual {
		return r, ErrConflict
	}
	return r, nil
}
