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
}

func (*sessionFake) inertProtocolOnly() {}
func (k *sessionFake) Create(ctx context.Context, p Plan) (Handle, error) {
	h, e := k.fakeKernel.Create(ctx, p)
	if k.badHandle {
		h.Address.Prefix = netip.MustParsePrefix("100.64.9.1/32")
	}
	return h, e
}
func (k *sessionFake) CloseOriginalFD(h Handle) error {
	k.closeCount++
	return k.fakeKernel.CloseOriginalFD(h)
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
	s := a.session
	s.closeAttempted = true
	s.closeRecorded = true
	s.closeConfirmed = false
	r, e := s.reconcile(s.baseline.raw, SysctlSnapshot(s.baseline.sys))
	if e != nil || !r.RawBaselineEqual || !r.CloseAmbiguous || !r.CustodyUnresolved {
		t.Fatal("close error upgraded", r, e)
	}
	if !errors.Is(s.begin(*s.canonical, Operation{}), ErrConflict) {
		t.Fatal("forward resumed")
	}
}
