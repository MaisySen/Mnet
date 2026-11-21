//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"io"
	"reflect"
	"strconv"
	"testing"
)

// An independently mutable fake device, not predictor-output readback. Effects
// change these operands; subsequent captures read this state. No OS claims.
type setupRunFake struct {
	c           *operation1Controller
	l           *heldPacketLedger
	h           Handle
	raw         RawInventory
	sys         SysctlSnapshot
	events      []string
	life        *disableResidentLifetime
	ticket      *setupPhaseTicket
	observation *setupPhaseObservation
	mode        string
}

func (*setupRunFake) requiresSetupPhaseSnapshots() {}

func (f *setupRunFake) owned() *LinkRecord {
	for i := range f.raw.Links {
		if f.raw.Links[i].Index == f.h.Link.Index {
			return &f.raw.Links[i]
		}
	}
	panic("missing fake original")
}
func (f *setupRunFake) word(index int, value uint32) {
	l := f.owned()
	a, _ := decodeAttributes(l.Raw[26])
	b, _ := decodeAttributes(a[10])
	binary.NativeEndian.PutUint32(b[2][index*4:], value)
	a[10] = encodeIPv6PolicyTestAttributes(b)
	l.Raw[26] = encodeIPv6PolicyTestAttributes(a)
}
func (f *setupRunFake) observePrecreation(ctx context.Context) (*precreationObservation, error) {
	f.events = append(f.events, "precreation")
	c := f.c
	b := c.adapter.session.baseline
	c.precreationSequence++
	c.precreationGeneration = 1
	po := &afSourcePassOwner{controller: c, active: true, port: 77}
	pass, e := collectRouteAFPass(ctx, po, sourceAFWireTest(b.raw.Links, "ok"))
	if e != nil {
		return nil, e
	}
	pass.sys = cloneSysExact(SysctlSnapshot(b.sys))
	pass.effective, e = parseEffectiveV4(pass.sys)
	if e != nil {
		return nil, e
	}
	return &precreationObservation{owner: c, baseline: b, pass: pass, raw: cloneRawExact(b.raw), sys: cloneSysExact(pass.sys), generation: 1, sequence: c.precreationSequence, phase: "precreation", identity: c.adapter.session.identity, binding: c.tx.plan.Binding}, nil
}
func (f *setupRunFake) createOriginal(_ context.Context, p Plan, a *originalAttempt) (Handle, error) {
	f.events = append(f.events, "create")
	if a != f.c.attempt || a.owner != f.c || !reflect.DeepEqual(p, a.plan) {
		return Handle{}, ErrConflict
	}
	if e := a.afterOpen(&fileCustody{lineage: f.l, closer: f}, func() error { return nil }); e != nil {
		return Handle{}, e
	}
	h := f.h
	a.handle = &h
	return h, nil
}
func (f *setupRunFake) bindCreatedOriginal(context.Context, *originalAttempt) (operation1DiagnosticSource, error) {
	return f, nil
}
func (f *setupRunFake) Close() error { f.events = append(f.events, "original-close"); return nil }
func (f *setupRunFake) observeOriginalAbsence(_ context.Context, a *originalAttempt) (*originalAbsenceEvidence, error) {
	f.events = append(f.events, "absence")
	b := f.c.adapter.session.baseline
	return parseOriginalAbsence(a, a.absenceTicket, b, b.raw, SysctlSnapshot(b.sys))
}
func (f *setupRunFake) diagnoseOperation1(context.Context) (ownedSourceDiagnostic, error) {
	return ownedSourceDiagnostic{}, afPhase
}
func (f *setupRunFake) observeSetupPhase(_ context.Context, t *setupPhaseTicket) (*setupPhaseObservation, error) {
	f.events = append(f.events, "snapshot"+strconv.Itoa(t.step))
	f.ticket = t
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	o := &setupPhaseObservation{ticket: t, diagnostic: ownedSourceDiagnostic{raw: cloneRawExact(f.raw), sys: cloneSysExact(f.sys), held: held, lineageUsable: f.l.matches(held.identity)}}
	f.observation = o
	if t.step == 2 && f.mode == "alias-capture-drift" {
		o.diagnostic.raw.Links[len(o.diagnostic.raw.Links)-1].Flags |= 1
	}
	if t.step == 3 && f.mode == "disable-capture-drift" {
		o.diagnostic.sys["/proc/sys/net/ipv6/conf/"+f.h.Link.Name+"/disable_ipv6"] = "0"
	}
	if f.mode == "sequence" {
		t.sequence++
	}
	if f.mode == "attempt" {
		t.attempt = &originalAttempt{}
	}
	if f.mode == "generation" {
		o.diagnostic.held.identity.generation++
	}
	if f.mode == "owner" {
		o.diagnostic.held.owner = &Transaction{}
	}
	if f.mode == "unsupported-phase" {
		t.step = 4
	}
	if f.mode == "outer-attr" {
		for i := range o.diagnostic.raw.Links {
			if o.diagnostic.raw.Links[i].Index == f.h.Link.Index {
				o.diagnostic.raw.Links[i].Raw[999] = []byte{1}
			}
		}
	}
	return o, nil
}
func (f *setupRunFake) applyOperation1(ctx context.Context, p Plan) (*firstStepCompletion, error) {
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	d, e := diagnosticForHeldRecords(f.raw, f.sys, held)
	if e != nil {
		return nil, e
	}
	return orchestrateFirstStepResult(ctx, f.l, p, d, 33, 77, firstStepDependencies{
		send: func(_ context.Context, t *packetSetupTransition, permit *firstLinkSendPermit) (Datagram, error) {
			e := permit.send(t.request, 77, func() error {
				f.events = append(f.events, "alias-effect")
				l := f.owned()
				l.Alias = f.h.Link.Alias
				l.MTU = 1280
				l.Raw[20] = append([]byte(l.Alias), 0)
				l.Raw[4] = u32(1280)
				f.word(2, 1280)
				return nil
			})
			return setupACKTest(t), e
		},
		collect: func(context.Context, *packetSetupTransition) (setupReadbackRecord, error) {
			f.events = append(f.events, "alias-readback")
			id := f.l.snapshot()
			raw := cloneRawExact(f.raw)
			if f.mode == "alias-effect-drift" {
				raw.Links[len(raw.Links)-1].MTU++
			}
			return setupReadbackRecord{before: id, after: id, raw: raw, sys: cloneSysExact(f.sys), names: &verifiedFeatureNames{values: pinnedFeatureNamesV617}, features: ethtoolActiveFeatures{active: 1<<0 | 1<<6 | 1<<7 | 1<<11 | 1<<14 | 1<<45}}, nil
		},
	})
}

type setupRunLeaf struct {
	*disableLeafMemory
	f *setupRunFake
}

func (l *setupRunLeaf) Write(b []byte) (int, error) {
	n, e := l.disableLeafMemory.Write(b)
	if e == nil && n == 2 {
		l.f.events = append(l.f.events, "disable-write")
		if l.f.mode != "disable-effect-drift" {
			l.f.word(26, 1)
		}
		l.f.sys["/proc/sys/net/ipv6/conf/"+l.f.h.Link.Name+"/disable_ipv6"] = "1"
	}
	return n, e
}

var _ io.ReadWriteSeeker = (*setupRunLeaf)(nil)

func (f *setupRunFake) applyOwnedDisable(ctx context.Context) (*postDisableResult, error) {
	f.events = append(f.events, "disable-finisher")
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	path := "/proc/sys/net/ipv6/conf/" + f.h.Link.Name + "/disable_ipv6"
	d := ownedSourceDiagnostic{raw: cloneRawExact(f.raw), sys: SysctlSnapshot{path: f.sys[path]}, held: held, lineageUsable: f.l.matches(held.identity)}
	return finishOwnedDisable(ctx, f.life, f.l, d, ownedDisableDependencies{
		check: func() error { return ctx.Err() },
		leaf: func() (ownedDisableLeaf, error) {
			f.events = append(f.events, "leaf-open")
			return &setupRunLeaf{disableLeafMemory: &disableLeafMemory{value: f.sys[path] + "\n"}, f: f}, nil
		},
		collect: func(context.Context) (RawInventory, SysctlSnapshot, error) {
			f.events = append(f.events, "disable-readback")
			return cloneRawExact(f.raw), SysctlSnapshot{path: f.sys[path]}, nil
		},
	})
}
func setupRunRig(t *testing.T, disabled string) (*operation1Controller, *setupRunFake) {
	t.Helper()
	c, old := operation1AdapterRig(t)
	s := c.adapter.session
	h := *s.canonical
	b := *s.baseline
	b.raw = cloneRawExact(b.raw)
	b.sys = map[string]string(cloneSysExact(SysctlSnapshot(b.sys)))
	for k, v := range old.d.sys {
		b.sys[k] = v
	}
	s.baseline = &b
	raw := cloneRawExact(b.raw)
	raw.Links = append(raw.Links, cloneRawExact(old.d.raw).Links[0])
	sys := cloneSysExact(SysctlSnapshot(b.sys))
	path := "/proc/sys/net/ipv6/conf/" + h.Link.Name + "/disable_ipv6"
	sys[path] = disabled
	sys[ipv6DirPrefix+h.Link.Name] = "present"
	f := &setupRunFake{c: c, l: old.l, h: h, raw: raw, sys: sys, life: &disableResidentLifetime{}}
	n, _ := strconv.Atoi(disabled)
	f.word(2, h.CreationLink.MTU)
	f.word(26, uint32(n))
	c.attempt = nil
	c.absence = f
	c.adapter.source = nil
	c.adapter.creation = f
	s.canonical = nil
	s.plan = nil
	s.forwardStep = 0
	s.createAttempted = false
	c.tx.plan = Plan{Binding: old.p.Binding}
	c.tx.handle = nil
	c.tx.PendingCleanup = false
	c.tx.Evidence = nil
	return c, f
}
func TestActualTransactionRunCreationAliasDisableThenAddressRefusal(t *testing.T) {
	for _, disabled := range []string{"0", "1"} {
		t.Run(disabled, func(t *testing.T) {
			c, f := setupRunRig(t, disabled)
			tx := c.tx
			e := c.withOwner(context.Background(), func(ctx context.Context) error { return tx.Run(ctx) })
			if e == nil || c.tx.Ready || c.tx.PendingCleanup || !c.tx.stopStarted || c.adapter.session.forwardStep != 3 || c.adapter.disabled == nil || c.adapter.disabled.current() {
				t.Fatal("Run escaped bounded graph or cleanup", e, c.tx.Ready, c.tx.PendingCleanup, c.adapter.session.forwardStep, f.events)
			}
			want := []string{"precreation", "create", "snapshot1", "alias-effect", "alias-readback", "snapshot2", "disable-finisher", "leaf-open"}
			if disabled == "0" {
				want = append(want, "disable-write")
			}
			want = append(want, "disable-readback", "snapshot3", "snapshot3", "original-close", "absence")
			if !reflect.DeepEqual(f.events, want) {
				t.Fatal("actual Run events", f.events, want)
			}
			last := c.tx.Evidence[len(c.tx.Evidence)-1]
			if last.Step != "address-add-exclusive" || !last.Error {
				t.Fatal("did not reach address refusal", c.tx.Evidence)
			}
			if f.ticket.active || !f.ticket.used {
				t.Fatal("ticket escaped")
			}
			if _, e := c.adapter.consumeSetupPhase(f.ticket, f.observation); e == nil {
				t.Fatal("replay consumed")
			}
		})
	}
}
func TestPhaseSnapshotsRefuseDriftBindingAndUnsupportedStates(t *testing.T) {
	for _, mode := range []string{"sequence", "attempt", "generation", "owner", "unsupported-phase", "outer-attr", "alias-effect-drift", "alias-capture-drift", "disable-effect-drift", "disable-capture-drift"} {
		t.Run(mode, func(t *testing.T) {
			c, f := setupRunRig(t, "0")
			f.mode = mode
			tx := c.tx
			e := c.withOwner(context.Background(), func(ctx context.Context) error { return tx.Run(ctx) })
			if e == nil || c.tx.Ready || c.tx.PendingCleanup || !c.tx.stopStarted {
				t.Fatal("bad phase escaped/unclean", e, f.events)
			}
			for _, e := range c.tx.Evidence {
				if e.Step == "address-add-exclusive-intent" {
					t.Fatal("drift reached address", mode, f.events)
				}
			}
			if len(f.events) < 2 || f.events[len(f.events)-2] != "original-close" || f.events[len(f.events)-1] != "absence" {
				t.Fatal("cleanup missing", f.events)
			}
		})
	}
}

func TestPhaseCreationCannotSkipPrecreationOrSnapshotAdmission(t *testing.T) {
	c, f := setupRunRig(t, "0")
	e := c.withOwner(context.Background(), func(ctx context.Context) error {
		_, e := c.adapter.Create(ctx, Plan{Binding: c.tx.plan.Binding})
		return e
	})
	if e == nil || len(f.events) != 0 || c.attempt != nil {
		t.Fatal("creation skipped precreation", e, f.events)
	}
	e = c.withOwner(context.Background(), func(ctx context.Context) error {
		before, e := c.adapter.Snapshot(ctx)
		if e != nil {
			return e
		}
		p, e := Prepare(c.tx.plan.Binding, before, c.tx.clock.Now().Wall)
		if e != nil {
			return e
		}
		c.tx.plan = p
		h, e := c.adapter.Create(ctx, p)
		if e != nil {
			return e
		}
		if e = c.adapter.Apply(ctx, h, p.Operations[1]); e == nil {
			t.Fatal("alias skipped creation Snapshot")
		}
		if _, e = c.adapter.Snapshot(ctx); e != nil {
			return e
		}
		ticket, observation := f.ticket, f.observation
		// Even reopening active is insufficient: used/sequence are terminal.
		ticket.active = true
		if _, e = c.adapter.consumeSetupPhase(ticket, observation); e == nil {
			t.Fatal("phase replay")
		}
		ticket.active = false
		if e = f.l.invalidate("export-intent"); e != nil {
			return e
		}
		if e = c.adapter.Apply(ctx, h, p.Operations[1]); e == nil {
			t.Fatal("stale admitted generation")
		}
		return c.tx.Stop(context.WithoutCancel(ctx))
	})
	if e != nil || c.tx.PendingCleanup || c.tx.Ready {
		t.Fatal("cleanup", e, f.events)
	}
}
