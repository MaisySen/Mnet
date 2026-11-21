//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"reflect"
	"sync"
	"testing"
)

func freshDisableFixture(t *testing.T) (ownedSourceDiagnostic, *disableResidentLifetime) {
	t.Helper()
	d := creationDownFixture(t)
	h := d.held
	// Synthetic phase2 fixture, not proof that real alias/source was executed.
	h.session.canonical.TagConfirmed = true
	h.session.forwardStep = 2
	h.identity.handle.TagConfirmed = true
	h.ledger.current = h.identity
	l := &d.raw.Links[0]
	l.Alias = h.session.canonical.Link.Alias
	l.MTU = 1280
	l.Raw[20] = append([]byte(l.Alias), 0)
	l.Raw[4] = u32(1280)
	setCreationWord(t, &d, 10, 2, 1280)
	setCreationWord(t, &d, 10, 26, 0)
	p := "/proc/sys/net/ipv6/conf/" + l.Name + "/disable_ipv6"
	d.sys = SysctlSnapshot{p: "0"}
	return d, &disableResidentLifetime{}
}
func disableFakeDeps(t *testing.T, d ownedSourceDiagnostic, f *disableLeafMemory, mode string) ownedDisableDependencies {
	t.Helper()
	expected, sys, e := predictFreshDownDisable(d.raw, d.sys, d.raw.Links[0].Index)
	if e != nil {
		t.Fatal(e)
	}
	return ownedDisableDependencies{check: func() error { return nil }, leaf: func() (ownedDisableLeaf, error) { return f, nil }, collect: func(context.Context) (RawInventory, SysctlSnapshot, error) {
		if mode == "af" {
			expected.Links[0].Flags |= 1
		}
		if mode == "address" {
			expected.Addresses = []AddressRecord{{Index: expected.Links[0].Index}}
		}
		if mode == "route" {
			expected.Routes = []RouteRecord{{OIF: uint32(expected.Links[0].Index)}}
		}
		if mode == "proc" {
			for k := range sys {
				sys[k] = "0"
			}
		}
		if mode == "invalidate" {
			_ = d.held.ledger.invalidate("export-intent")
		}
		return expected, sys, nil
	}}
}
func TestFreshDisablePredictionChangesOnlyNamedWord(t *testing.T) {
	d, _ := freshDisableFixture(t)
	before := cloneRawExact(d.raw)
	want := cloneRawExact(d.raw)
	l := &want.Links[0]
	a, _ := decodeAttributes(l.Raw[26])
	b, _ := decodeAttributes(a[10])
	binary.NativeEndian.PutUint32(b[2][26*4:], 1)
	a[10] = encodeIPv6PolicyTestAttributes(b)
	l.Raw[26] = encodeIPv6PolicyTestAttributes(a)
	got, sys, e := predictFreshDownDisable(d.raw, d.sys, l.Index)
	if e != nil || !exactSetupPostMatches(got, want) || !reflect.DeepEqual(before, d.raw) {
		t.Fatal("wrong predicted delta", e)
	}
	for _, v := range sys {
		if v != "1" {
			t.Fatal("wrong proc")
		}
	}
}
func TestFreshDisableUnsupportedPrestatesRefuseBeforeLeaf(t *testing.T) {
	for _, mode := range []string{"up", "ready", "address", "route", "mtu1279", "mtu1281", "ramtu", "missing-proc", "malformed-af"} {
		t.Run(mode, func(t *testing.T) {
			d, life := freshDisableFixture(t)
			switch mode {
			case "up":
				d.raw.Links[0].Flags |= 1
			case "ready":
				a, _ := decodeAttributes(d.raw.Links[0].Raw[26])
				b, _ := decodeAttributes(a[10])
				b[1] = u32(0x80000000)
				a[10] = encodeIPv6PolicyTestAttributes(b)
				d.raw.Links[0].Raw[26] = encodeIPv6PolicyTestAttributes(a)
			case "address":
				d.raw.Addresses = []AddressRecord{{Index: d.raw.Links[0].Index}}
			case "route":
				d.raw.Routes = []RouteRecord{{OIF: 1}}
			case "mtu1279":
				d.raw.Links[0].MTU = 1279
			case "mtu1281":
				d.raw.Links[0].MTU = 1281
			case "ramtu":
				a, _ := decodeAttributes(d.raw.Links[0].Raw[26])
				b, _ := decodeAttributes(a[10])
				b[9] = u32(1400)
				a[10] = encodeIPv6PolicyTestAttributes(b)
				d.raw.Links[0].Raw[26] = encodeIPv6PolicyTestAttributes(a)
			case "missing-proc":
				d.sys = nil
			case "malformed-af":
				d.raw.Links[0].Raw[26] = []byte{1}
			}
			opened := false
			_, e := finishOwnedDisable(context.Background(), life, d.held.ledger, d, ownedDisableDependencies{check: func() error { return nil }, leaf: func() (ownedDisableLeaf, error) { opened = true; return nil, ErrConflict }, collect: func(context.Context) (RawInventory, SysctlSnapshot, error) { return RawInventory{}, nil, ErrConflict }})
			if e == nil || opened {
				t.Fatal("unsupported effect", mode, e)
			}
		})
	}
}
func TestDisableTypedResultRequiresIndependentReadbackAndLifetime(t *testing.T) {
	d, life := freshDisableFixture(t)
	f := &disableLeafMemory{value: "0\n"}
	result, e := finishOwnedDisable(context.Background(), life, d.held.ledger, d, disableFakeDeps(t, d, f, ""))
	if e != nil || result == nil || !result.current() || f.writes != 1 {
		t.Fatal("typed finish", e)
	}
	if d.held.session.forwardStep != 2 {
		t.Fatal("transition advanced session")
	}
	life.revoke()
	if result.current() {
		t.Fatal("resident lifetime escape")
	}
}
func TestDisableReadbackDriftNeverMintsResult(t *testing.T) {
	for _, mode := range []string{"af", "address", "route", "proc", "invalidate"} {
		d, life := freshDisableFixture(t)
		f := &disableLeafMemory{value: "0\n"}
		r, e := finishOwnedDisable(context.Background(), life, d.held.ledger, d, disableFakeDeps(t, d, f, mode))
		if e == nil || r != nil || !d.held.ledger.snapshot().uncertain {
			t.Fatal(mode, e)
		}
	}
}
func TestDisableCloseOrPreReadMismatchRefusesNoRetry(t *testing.T) {
	for _, mode := range []string{"close", "mismatch"} {
		d, life := freshDisableFixture(t)
		f := &disableLeafMemory{value: "0\n", failClose: mode == "close"}
		if mode == "mismatch" {
			f.value = "1\n"
		}
		r, e := finishOwnedDisable(context.Background(), life, d.held.ledger, d, disableFakeDeps(t, d, f, ""))
		if e == nil || r != nil || f.writes > 1 || !d.held.ledger.snapshot().uncertain {
			t.Fatal(mode, e)
		}
	}
}
func TestAliasMTU1280PredictsNestedIPv6Word(t *testing.T) {
	d, _ := freshDisableFixture(t)
	l := d.raw.Links[0]
	l.MTU = 1500
	setCreationWord(t, &d, 10, 2, 1500)
	l.Raw = d.raw.Links[0].Raw
	if e := predictOwnedMTU6(&l, RawInventory{Links: []LinkRecord{l}}); e != nil {
		t.Fatal(e)
	}
	a, _ := decodeAttributes(l.Raw[26])
	b, _ := decodeAttributes(a[10])
	if binary.NativeEndian.Uint32(b[2][8:]) != 1280 {
		t.Fatal("missing nested mtu6 delta")
	}
	l.Flags |= 1
	if e := predictOwnedMTU6(&l, RawInventory{}); e == nil {
		t.Fatal("up profile accepted")
	}
}

func TestDisableAlreadyOneNoWriteStillRequiresFullCollection(t *testing.T) {
	d, life := freshDisableFixture(t)
	setCreationWord(t, &d, 10, 26, 1)
	for k := range d.sys {
		d.sys[k] = "1"
	}
	f := &disableLeafMemory{value: "1\n"}
	r, e := finishOwnedDisable(context.Background(), life, d.held.ledger, d, disableFakeDeps(t, d, f, ""))
	if e != nil || r == nil || !r.current() || f.writes != 0 || f.closes != 1 {
		t.Fatal(e, f)
	}
	_ = d.held.ledger.invalidate("close-intent")
	if r.current() {
		t.Fatal("postclose result usable")
	}
}

func TestFreshDisableRequiresExactTUNNestingAndKind(t *testing.T) {
	for _, mode := range []string{"nested-flag-info-data", "missing-info-data", "wrong-kind", "unknown-info-attr", "malformed-af"} {
		t.Run(mode, func(t *testing.T) {
			d, _ := freshDisableFixture(t)
			l := &d.raw.Links[0]
			switch mode {
			case "nested-flag-info-data":
				info, e := schemaAttrs(l.Raw[18], map[uint16]bool{1: false, 2: false})
				if e != nil {
					t.Fatal(e)
				}
				l.Raw[18] = append(attr(1, info[1]), attr(0x8002, info[2])...)
			case "missing-info-data":
				l.Raw[18] = attr(1, []byte("tun\x00"))
			case "wrong-kind":
				info, e := schemaAttrs(l.Raw[18], map[uint16]bool{1: false, 2: false})
				if e != nil {
					t.Fatal(e)
				}
				l.Raw[18] = append(attr(1, []byte("tap\x00")), attr(2, info[2])...)
			case "unknown-info-attr":
				l.Raw[18] = append(l.Raw[18], attr(3, []byte{0})...)
			case "malformed-af":
				l.Raw[26] = append(l.Raw[26], []byte{0}...)
			}
			if _, _, e := predictFreshDownDisable(d.raw, d.sys, l.Index); e == nil {
				t.Fatal("weakened shape", mode)
			}
		})
	}
	d, _ := freshDisableFixture(t)
	if _, _, e := predictFreshDownDisable(d.raw, d.sys, d.raw.Links[0].Index); e != nil {
		t.Fatal("correct nested kind rejected", e)
	}
}
func TestPostDisableLifetimeConcurrentRevocation(t *testing.T) {
	d, life := freshDisableFixture(t)
	f := &disableLeafMemory{value: "0\n"}
	r, e := finishOwnedDisable(context.Background(), life, d.held.ledger, d, disableFakeDeps(t, d, f, ""))
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 1000; n++ {
				_ = r.current()
			}
		}()
	}
	life.revoke()
	wg.Wait()
	if r.current() {
		t.Fatal("revoked lifetime current")
	}
	life.revoke()
	if life.current() {
		t.Fatal("revocation not terminal")
	}
}
