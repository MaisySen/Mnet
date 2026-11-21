//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

// Pure marker implementation exists ONLY in this test. Embedded fakeKernel
// mutates Go values, no FDs, callbacks, subprocesses, network or syscalls.
type sessionFake struct {
	*fakeKernel
	badHandle  bool
	closeCount int
	closeError bool
}

func (*sessionFake) inertProtocolOnly() {}

// This fake chooses a fresh index from the captured inventory. The old
// fakeKernel is kept untouched: it deliberately uses index 7 for its own rig.
func (k *sessionFake) Create(_ context.Context, p Plan) (Handle, error) {
	index := 1
	for _, l := range k.i.Links {
		if l.Index >= index {
			index = l.Index + 1
		}
	}
	h, e := AttachCreated(p, index, 1500)
	if e != nil {
		return h, e
	}
	k.h = h
	k.calls = append(k.calls, "create")
	k.i.Links = append(k.i.Links, h.CreationLink)
	if k.badHandle {
		h.Address.Prefix = netip.MustParsePrefix("100.64.9.1/32")
	}
	return h, nil
}
func (k *sessionFake) Apply(_ context.Context, h Handle, o Operation) error {
	k.calls = append(k.calls, o.Kind)
	bad := k.fault == o.Kind
	if (!bad || k.successOnError) && !k.missingACK {
		switch o.Kind {
		case "set-alias-mtu-exact":
			for n, l := range k.i.Links {
				if l == h.CreationLink {
					k.i.Links[n] = h.Link
				}
			}
		case "disable-owned-link-ipv6":
			k.i.IPv6Disabled[h.Link.Index] = true
		case "address-add-exclusive":
			k.i.Addresses = append(k.i.Addresses, h.Address)
		case "link-up-exact":
			k.i.LinkUp[h.Link.Index] = true
		case "route-add-exclusive":
			k.i.Routes = append(k.i.Routes, h.Route)
		case "route-del-exact":
			var next []Route
			for _, r := range k.i.Routes {
				if r != o.Route {
					next = append(next, r)
				}
			}
			k.i.Routes = next
		case "address-del-exact":
			var next []Address
			for _, a := range k.i.Addresses {
				if a.Prefix != o.Address || a.LinkIndex != o.Index {
					next = append(next, a)
				}
			}
			k.i.Addresses = next
		default:
			return ErrConflict
		}
	}
	if bad {
		return errors.New("fake ambiguous operation")
	}
	return nil
}
func (k *sessionFake) CloseOriginalFD(h Handle) error {
	k.closeCount++
	e := k.fakeKernel.CloseOriginalFD(h)
	if k.closeError {
		return errors.New("fake close error after modeled removal")
	}
	return e
}
func protocolRig(t *testing.T) (*Transaction, *inertAdapterProtocol, *sessionFake) {
	t.Helper()
	base, _, _ := fixtureBaseline(t)
	b, _, now := rig()
	b.Namespace = base.ns
	i, e := base.Compose(base.raw, SysctlSnapshot(base.sys), nil, false)
	if e != nil {
		t.Fatal(e)
	}
	k := &sessionFake{fakeKernel: &fakeKernel{i: i}}
	a, e := newInertAdapterProtocol(k, base)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := NewTransaction(a, &fakeClock{Stamp{Wall: now, Monotonic: time.Second}}, b)
	if e != nil {
		t.Fatal(e)
	}
	return tx, a, k
}
func TestInertProtocolCleanTransaction(t *testing.T) {
	tx, a, k := protocolRig(t)
	if e := tx.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := tx.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	if tx.PendingCleanup || k.closeCount != 1 || !a.session.closeConfirmed {
		t.Fatal("incomplete", tx.PendingCleanup, k.closeCount)
	}
}
func TestInertProtocolDeleteBudget(t *testing.T) {
	tx, a, k := protocolRig(t)
	if e := tx.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	k.fault = "route-del-exact"
	for attempt := 1; attempt <= 2; attempt++ {
		if e := tx.Stop(context.Background()); e == nil {
			t.Fatal("failed delete declared clean")
		}
		if !tx.PendingCleanup {
			t.Fatal("lost custody")
		}
	}
	before := len(k.calls)
	if e := tx.Stop(context.Background()); e == nil {
		t.Fatal("budget not exhausted")
	}
	if len(k.calls) != before || k.closeCount != 0 {
		t.Fatal("effect after budget")
	}
	if len(a.session.cleanupAttempts) != 1 {
		t.Fatal(a.session.cleanupAttempts)
	}
	for op, n := range a.session.cleanupAttempts {
		if n != 2 {
			t.Fatal(n)
		}
		changed := op
		changed.Route.Priority++
		if e := a.session.beginCleanup(*a.session.canonical, k.i, changed); e == nil {
			t.Fatal("changed retry accepted")
		}
	}
	// Removing and readding the same modeled object cannot reset attempt budget.
	op := Operation{Kind: "route-del-exact", Route: a.session.canonical.Route}
	k.i.Routes = append(k.i.Routes, op.Route)
	if e := a.session.beginCleanup(*a.session.canonical, k.i, op); e == nil {
		t.Fatal("readd reset budget")
	}
}
func TestInertProtocolRejectedCustodyBlocksStop(t *testing.T) {
	tx, a, k := protocolRig(t)
	k.badHandle = true
	if e := tx.Start(context.Background()); e == nil {
		t.Fatal("bad creation accepted")
	}
	k.i.Links = nil
	if e := tx.Stop(context.Background()); e == nil {
		t.Fatal("rejected custody declared done")
	}
	if a.session.rejectedHandle == nil || !tx.PendingCleanup || k.closeCount != 0 {
		t.Fatal("rejected custody lost")
	}
}
func TestInertProtocolCloseContextConsumed(t *testing.T) {
	_, a, k := protocolRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.ownerContext = ctx
	if e := a.CloseOriginalFD(Handle{}); e == nil {
		t.Fatal("canceled close")
	}
	if a.ownerContext != nil || k.closeCount != 0 {
		t.Fatal("context reused")
	}
	a.ownerContext = context.Background()
	if e := a.CloseOriginalFD(Handle{}); e == nil {
		t.Fatal("unbounded close")
	}
	if a.ownerContext != nil {
		t.Fatal("unbounded context retained")
	}
}
func TestExactBaselineCloneShapeGuard(t *testing.T) {
	for value, n := range map[reflect.Type]int{reflect.TypeOf(RawInventory{}): 4, reflect.TypeOf(LinkRecord{}): 7, reflect.TypeOf(AddressRecord{}): 8, reflect.TypeOf(RouteRecord{}): 18} {
		if value.NumField() != n {
			t.Fatalf("clone schema changed %s fields=%d expected=%d", value, value.NumField(), n)
		}
	}
}
func TestExactBaselineCloneNilEmptyAndNetip(t *testing.T) {
	for _, raw := range []RawInventory{{}, {Links: []LinkRecord{}, Addresses: []AddressRecord{}, Routes: []RouteRecord{}, Rules: [][]byte{}}, {Links: []LinkRecord{{Raw: map[uint16][]byte{1: nil, 2: {}}}}, Addresses: []AddressRecord{{Local: netip.MustParseAddr("fe80::1%zone0"), Raw: map[uint16][]byte{3: {1, 2}}}}, Routes: []RouteRecord{{Dst: netip.MustParseAddr("192.0.2.1"), Raw: map[uint16][]byte{4: {3}}}}, Rules: [][]byte{nil, {}, {1, 2}}}} {
		cloned := cloneRawExact(raw)
		if !reflect.DeepEqual(raw, cloned) {
			t.Fatal("shape changed")
		}
		if len(cloned.Rules) > 2 {
			cloned.Rules[2][0] = 9
			if raw.Rules[2][0] != 1 {
				t.Fatal("rules alias")
			}
			cloned.Addresses[0].Raw[3][0] = 9
			if raw.Addresses[0].Raw[3][0] != 1 {
				t.Fatal("attrs alias")
			}
		}
	}
}
func TestCapabilityRequiresFinalCustodyAndBoundFreshWitnesses(t *testing.T) {
	now := time.Now()
	ns := Namespace{Device: 7, Inode: 8, Isolated: true}
	clean := adapterFinalState{}
	e := networkCapabilityEvidence{Final: &clean, Now: now, TargetID: "target", SessionID: "session", Namespace: ns}
	for _, kind := range []string{"deployment", "os-path", "transport", "authority"} {
		e.Witnesses = append(e.Witnesses, networkCapabilityWitness{Kind: kind, TargetID: e.TargetID, SessionID: e.SessionID, Namespace: ns, ObservedAt: now.Add(-time.Second), ValidUntil: now.Add(time.Second)})
	}
	if !projectNetworkCapability(e).NetworkingReady {
		t.Fatal("bound witness refused")
	}
	stripped := e
	stripped.Now = now.Round(0)
	if !reflect.DeepEqual(projectNetworkCapability(e), projectNetworkCapability(stripped)) {
		t.Fatal("hidden monotonic changes projection")
	}
	e.Now = now.Add(time.Second)
	if projectNetworkCapability(e).NetworkingReady {
		t.Fatal("expiry accepted")
	}
	e.Now = now
	for _, f := range []*adapterFinalState{nil, {CustodyUnresolved: true}, {CreateAttempted: true}, {CloseAmbiguous: true}, {RejectedHandle: &Handle{}}, {PossiblyApplied: &Operation{}}} {
		e.Final = f
		if projectNetworkCapability(e).NetworkingReady {
			t.Fatal("unresolved state omitted")
		}
	}
	e.Final = &clean
	e.Witnesses[0].TargetID = "other"
	if projectNetworkCapability(e).NetworkingReady {
		t.Fatal("target mismatch")
	}
}
func TestReadbackLagPoisonsAndCloseErrorReconciles(t *testing.T) {
	tx, a, k := protocolRig(t)
	k.missingACK = true
	if e := tx.Start(context.Background()); e == nil {
		t.Fatal("missing readback accepted")
	}
	if !a.session.poisoned || a.session.pending == nil {
		t.Fatal("lag lost")
	}
	// Drive actual Stop and fake close error rather than assigning ledger fields.
	k.missingACK = false
	k.closeError = true
	if e := tx.Stop(context.Background()); e == nil {
		t.Fatal("failed close reported clean")
	}
	if k.closeCount != 1 || !tx.PendingCleanup {
		t.Fatal("failed close custody lost")
	}
	// Independently decode original fixture, not session.baseline itself.
	raw, sys := recordedRaw(t)
	r, e := a.session.reconcile(raw, sys)
	if e != nil || !r.RawBaselineEqual || !r.CloseAmbiguous || !r.CustodyUnresolved {
		t.Fatal("close error upgraded", r, e)
	}
	changed := cloneRawExact(raw)
	changed.Links[0].MTU++
	if r, e = a.session.reconcile(changed, sys); e == nil || r.RawBaselineEqual {
		t.Fatal("changed external baseline accepted")
	}
	if e = a.session.begin(*a.session.canonical, Operation{}); !errors.Is(e, ErrConflict) {
		t.Fatal("forward resumed")
	}
}
func TestResolvedPoisonFinalCanProjectBoundPath(t *testing.T) {
	tx, a, k := protocolRig(t)
	k.fault = "route-add-exclusive"
	k.successOnError = true
	if e := tx.Start(context.Background()); e == nil {
		t.Fatal("ambiguous setup accepted")
	}
	k.fault = ""
	if e := tx.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	raw, sys := recordedRaw(t)
	final, e := a.session.reconcile(raw, sys)
	if e != nil || !final.Poisoned || final.PossiblyApplied == nil || final.requiresCleanup() {
		t.Fatal("resolved poisoned history mapping", final, e)
	}
	now := time.Now()
	ev := networkCapabilityEvidence{Final: &final, TargetID: "target", SessionID: "new-explicit-session", Namespace: a.session.baseline.ns, Now: now}
	for _, kind := range []string{"deployment", "os-path", "transport", "authority"} {
		ev.Witnesses = append(ev.Witnesses, networkCapabilityWitness{Kind: kind, TargetID: ev.TargetID, SessionID: ev.SessionID, Namespace: ev.Namespace, ObservedAt: now.Add(-time.Second), ValidUntil: now.Add(time.Second)})
	}
	// These are fresh independent projection witnesses, not expired old authority.
	if !projectNetworkCapability(ev).NetworkingReady {
		t.Fatal("resolved history wrongly blocks independent bound witnesses")
	}
}
func TestReaddedDeleteWithoutRetrySlotRefused(t *testing.T) {
	tx, a, k := protocolRig(t)
	if e := tx.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	h := *a.session.canonical
	op := Operation{Kind: "route-del-exact", Route: h.Route}
	if e := a.Apply(context.Background(), h, op); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Snapshot(context.Background()); e != nil {
		t.Fatal(e)
	}
	if a.session.cleanupAttempts[op] != 1 || a.session.cleanupRetry != nil || a.session.cleanupPending != nil {
		t.Fatal("not testing absent readback/no-retry-slot")
	}
	k.i.Routes = append(k.i.Routes, h.Route)
	before := len(k.calls)
	if e := a.Apply(context.Background(), h, op); !errors.Is(e, ErrConflict) {
		t.Fatal("readd should hit no-retry-slot not exhausted budget", e)
	}
	if len(k.calls) != before || a.session.cleanupAttempts[op] != 1 {
		t.Fatal("readd issued effect or consumed budget")
	}
}
