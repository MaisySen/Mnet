//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
)

type adapterDisableFake struct {
	*operation1FakeSource
	life   *disableResidentLifetime
	events *[]string
	opens  int
	mode   string
}

func (f *adapterDisableFake) applyOwnedDisable(ctx context.Context) (*postDisableResult, error) {
	*f.events = append(*f.events, "disable-finisher")
	held, e := heldSourceSnapshot(f.l, f.d.held.owner, f.d.held.session)
	if e != nil {
		return nil, e
	}
	name := held.session.canonical.Link.Name
	// Captured actual fake records, not a synthesized successful policy vector.
	d := ownedSourceDiagnostic{raw: cloneRawExact(f.d.raw), sys: SysctlSnapshot{"/proc/sys/net/ipv6/conf/" + name + "/disable_ipv6": "1"}, held: held, lineageUsable: f.l.matches(held.identity)}
	a, e := decodeAttributes(d.raw.Links[0].Raw[26])
	if e != nil {
		return nil, e
	}
	b, e := decodeAttributes(a[10])
	if e != nil {
		return nil, e
	}
	if len(b[2]) != 240 {
		return nil, afShape
	}
	// First-step fixture inherits disabled1; no-op still requires readback.
	value := "1"
	if f.mode == "write" {
		value = "0"
		binary.NativeEndian.PutUint32(b[2][26*4:], 0)
		a[10] = encodeIPv6PolicyTestAttributes(b)
		d.raw.Links[0].Raw[26] = encodeIPv6PolicyTestAttributes(a)
		for k := range d.sys {
			d.sys[k] = "0"
		}
	}
	expected, postSys, e := predictFreshDownDisable(d.raw, d.sys, d.raw.Links[0].Index)
	if e != nil {
		return nil, e
	}
	deps := ownedDisableDependencies{check: func() error { return nil }, leaf: func() (ownedDisableLeaf, error) {
		f.opens++
		*f.events = append(*f.events, "leaf-open")
		if f.mode == "open-error" {
			return nil, errors.New("fake leaf failure")
		}
		return &disableLeafMemory{value: value + "\n"}, nil
	}, collect: func(context.Context) (RawInventory, SysctlSnapshot, error) {
		*f.events = append(*f.events, "independent-af-raw")
		raw := cloneRawExact(expected)
		sys := cloneSysExact(postSys)
		if f.mode == "drift" {
			raw.Links[0].Flags |= 1
		}
		return raw, sys, nil
	}}
	return finishOwnedDisable(ctx, f.life, f.l, d, deps)
}

type recordedOriginalCloser struct{ events *[]string }

func (c *recordedOriginalCloser) Close() error {
	*c.events = append(*c.events, "original-close")
	return nil
}

type recordedDisableAbsence struct {
	rawAbsenceFake
	events *[]string
}

func (f *recordedDisableAbsence) observeOriginalAbsence(ctx context.Context, a *originalAttempt) (*originalAbsenceEvidence, error) {
	*f.events = append(*f.events, "absence")
	return f.rawAbsenceFake.observeOriginalAbsence(ctx, a)
}
func adapterDisableRig(t *testing.T) (*operation1Controller, *adapterDisableFake, *[]string) {
	t.Helper()
	c, base := operation1AdapterRig(t)
	events := []string{}
	f := &adapterDisableFake{operation1FakeSource: base, life: &disableResidentLifetime{}, events: &events}
	base.after = func() { events = append(events, "alias-finisher") }
	c.adapter.source = f
	baseline := c.adapter.session.baseline
	c.absence = &recordedDisableAbsence{events: &events, rawAbsenceFake: rawAbsenceFake{baseline: baseline, raw: cloneRawExact(baseline.raw), sys: cloneSysExact(SysctlSnapshot(baseline.sys))}}
	c.attempt = &originalAttempt{owner: c, plan: base.p, custody: &fileCustody{lineage: base.l, closer: &recordedOriginalCloser{events: &events}}}
	return c, f, &events
}
func TestAdapterActualAliasDisableStopAbsenceDispatch(t *testing.T) {
	c, f, events := adapterDisableRig(t)
	apply := func(i int) error {
		return c.withOwner(context.Background(), func(ctx context.Context) error {
			return c.adapter.Apply(ctx, *c.adapter.session.canonical, f.p.Operations[i])
		})
	}
	if e := apply(1); e != nil {
		t.Fatal("alias dispatch", e)
	}
	if e := apply(2); e != nil {
		t.Fatal("disable dispatch", e)
	}
	if c.adapter.session.forwardStep != 3 || c.adapter.disabled == nil || !c.adapter.disabled.current() || !c.adapter.disabled.consumed || c.tx.Ready {
		t.Fatal("typed join/readiness")
	}
	// Consumer is a test-only inspection seam, not an actual packet supplier.
	if !consumeDisableResultFake(c.adapter.disabled) {
		t.Fatal("fake consumer refused verified result")
	}
	for _, i := range []int{1, 2, 3, 4, 5} {
		if e := apply(i); e == nil || f.opens != 1 {
			t.Fatal("repeat/later dispatch", i, e, f.opens)
		}
	}
	if e := c.stop(context.Background()); e != nil {
		t.Fatal("stop", e)
	}
	if c.adapter.disabled.current() || c.tx.PendingCleanup || c.tx.Ready {
		t.Fatal("post-stop result or cleanup")
	}
	want := []string{"alias-finisher", "disable-finisher", "leaf-open", "independent-af-raw", "original-close", "absence"}
	if !reflect.DeepEqual(*events, want) {
		t.Fatal("actual dispatch order", *events, want)
	}
}
func consumeDisableResultFake(r *postDisableResult) bool {
	return r != nil && r.consumed && r.current()
}
func TestAdapterWrongPhaseUnleasedRepeatNeverOpensDisableLeaf(t *testing.T) {
	c, f, _ := adapterDisableRig(t)
	h := *c.adapter.session.canonical
	if e := c.adapter.Apply(context.Background(), h, f.p.Operations[2]); e == nil || f.opens != 0 {
		t.Fatal("unleased")
	}
	if e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.adapter.Apply(ctx, h, f.p.Operations[2]) }); e == nil || f.opens != 0 {
		t.Fatal("pre-alias")
	}
	if e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.adapter.Apply(ctx, h, f.p.Operations[1]) }); e != nil {
		t.Fatal(e)
	}
	wrong := f.p.Operations[2]
	wrong.Name = "wrong"
	if e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.adapter.Apply(ctx, *c.adapter.session.canonical, wrong) }); e == nil || f.opens != 0 {
		t.Fatal("wrong operation")
	}
	f.mode = "open-error"
	if e := c.withOwner(context.Background(), func(ctx context.Context) error {
		return c.adapter.Apply(ctx, *c.adapter.session.canonical, f.p.Operations[2])
	}); e == nil || f.opens != 1 {
		t.Fatal("fake failure", e, f.opens)
	}
	if e := c.withOwner(context.Background(), func(ctx context.Context) error {
		return c.adapter.Apply(ctx, *c.adapter.session.canonical, f.p.Operations[2])
	}); e == nil || f.opens != 1 {
		t.Fatal("failure retried")
	}
}
func TestAdapterDisableReadbackDriftPoisonsAndStopStillCloses(t *testing.T) {
	c, f, events := adapterDisableRig(t)
	f.mode = "drift"
	for _, i := range []int{1, 2} {
		e := c.withOwner(context.Background(), func(ctx context.Context) error {
			return c.adapter.Apply(ctx, *c.adapter.session.canonical, f.p.Operations[i])
		})
		if (i == 2) != (e != nil) {
			t.Fatal(i, e)
		}
	}
	if c.adapter.disabled != nil || !c.adapter.session.poisoned || !f.l.snapshot().uncertain {
		t.Fatal("drift token")
	}
	if e := c.stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(*events) < 2 || (*events)[len(*events)-2] != "original-close" || (*events)[len(*events)-1] != "absence" {
		t.Fatal("cleanup skipped", *events)
	}
}

func TestAdapterDisableZeroToOneUsesSameTypedDispatch(t *testing.T) {
	c, f, _ := adapterDisableRig(t)
	f.mode = "write"
	for _, i := range []int{1, 2} {
		if e := c.withOwner(context.Background(), func(ctx context.Context) error {
			return c.adapter.Apply(ctx, *c.adapter.session.canonical, f.p.Operations[i])
		}); e != nil {
			t.Fatal(i, e)
		}
	}
	if f.opens != 1 || c.adapter.disabled == nil || !c.adapter.disabled.current() || c.tx.Ready {
		t.Fatal("zero-to-one not verified")
	}
}

func TestDisableOriginalVerifierComparisonAllowsOnlyControlledLedgerChange(t *testing.T) {
	d, _ := freshDisableFixture(t)
	a := d.held.identity
	b := a
	b.generation++
	b.uncertain = true
	if !sameDisableOriginalIdentity(a, b) {
		t.Fatal("controlled invalidation rejected")
	}
	for _, mode := range []string{"closed", "target", "device", "fd-object", "handle", "adapter"} {
		c := b
		switch mode {
		case "closed":
			c.closed = true
		case "target":
			c.target.Inode++
		case "device":
			c.device.Inode++
		case "fd-object":
			c.fdObject = &packetOriginalObject{serial: 1}
		case "handle":
			c.handle.Link.Index++
		case "adapter":
			c.identity.AttemptID += "-other"
		}
		if sameDisableOriginalIdentity(a, c) {
			t.Fatal("identity drift ignored", mode)
		}
	}
}
