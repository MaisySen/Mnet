//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"reflect"
	"sort"
	"testing"
)

// Full-emitter-shaped synthetic records. Metadata examples intentionally are
// not claims about allocator literals, qdisc defaults, enum meanings or ABI.
func fullSetupAttrsTest(l *LinkRecord) {
	for k, v := range map[uint16]uint32{13: 500, 27: 0, 30: 0, 31: 1, 32: 1, 35: 3, 40: 123, 41: 456, 47: 2, 48: 1, 50: 68, 51: 65535, 58: 789, 59: 101, 60: 202, 61: 0, 63: 303, 64: 404} {
		l.Raw[k] = u32(v)
	}
	l.Raw[6] = []byte("noqueue\x00") // example scheduler label, not source constant proof
	l.Raw[7] = make([]byte, 96)
	l.Raw[23] = make([]byte, 200)
	l.Raw[14] = make([]byte, 32) // example allowed-form policy
	l.Raw[16] = []byte{199}
	l.Raw[17] = []byte{0}
	l.Raw[33] = []byte{1}
	l.Raw[39] = []byte{0}
	l.Raw[67] = []byte{0}
	l.Raw[43] = attr(2, []byte{0})
	l.Raw[62] = nil
	l.Raw[65] = nil
	l.Raw[66] = make([]byte, 8)
}
func fullSetupBodyTest(l LinkRecord, mode string) []byte {
	body := make([]byte, 16)
	binary.NativeEndian.PutUint16(body[2:], l.Hardware)
	binary.NativeEndian.PutUint32(body[4:], uint32(l.Index))
	binary.NativeEndian.PutUint32(body[8:], l.Flags)
	var keys []int
	for k := range l.Raw {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, key := range keys {
		k := uint16(key)
		// Exercise different supported PAD layouts after alias. This is an encoding
		// policy fixture, not the missing kernel alignment helper's output oracle.
		if k == 23 && l.Alias == "" || k == 14 && l.Alias != "" {
			body = append(body, attr(42, nil)...)
		}
		typ := k
		if k == 62 || k == 65 {
			typ |= 0x8000
		}
		body = append(body, attr(typ, l.Raw[k])...)
	}
	if mode == "pad-payload" {
		body = append(body, attr(42, []byte{0})...)
	}
	if mode == "pad-trailing" {
		body = append(body, attr(42, nil)...)
	}
	if mode == "duplicate" {
		body = append(body, attr(4, u32(l.MTU))...)
	}
	return body
}
func (f *setupRunFake) captureSetupOuter() (RawInventory, error) {
	raw := cloneRawExact(f.raw)
	if !f.full {
		return raw, nil
	}
	for i := range raw.Links {
		l := raw.Links[i]
		if l.Index == f.h.Link.Index {
			parsed, e := decodeSetupLink(fullSetupBodyTest(l, ""), f.h)
			if e != nil {
				return RawInventory{}, e
			}
			raw.Links[i] = parsed
		}
	}
	return raw, nil
}

type fullSetupDumpTest struct{ f *setupRunFake }

func (d fullSetupDumpTest) decodeOwnedSetupLink(b []byte) (LinkRecord, error) {
	return decodeSetupLink(b, d.f.h)
}
func (d fullSetupDumpTest) Request(_ context.Context, kind uint16, _ []byte, _ bool) ([][]byte, error) {
	raw, e := d.f.captureSetupOuter()
	if e != nil {
		return nil, e
	}
	var bodies [][]byte
	switch kind {
	case 18:
		for _, l := range raw.Links {
			bodies = append(bodies, afLinkBodyTest(l))
		}
	case 34:
		bodies = raw.Rules
	}
	return bodies, nil
}
func (f *setupRunFake) collectFullSetupPhase(ctx context.Context, first RawInventory) (RawInventory, error) {
	owner := &afSourcePassOwner{controller: f.c, active: true, port: 77, ownedSetup: &f.h}
	pass, e := collectRouteAFPass(ctx, owner, sourceAFWireTest(first.Links, "ok"))
	if e != nil {
		return RawInventory{}, e
	}
	raw, e := ReadStableInventory(ctx, fullSetupDumpTest{f: f}, func() error { return ctx.Err() })
	if e != nil {
		return RawInventory{}, e
	}
	if !exactSetupPostMatches(RawInventory{Links: pass.links}, RawInventory{Links: raw.Links}) {
		return RawInventory{}, afShape
	}
	return raw, nil
}
func (f *setupRunFake) mutateFullPhaseObservation(o *setupPhaseObservation) error {
	for i := range o.diagnostic.raw.Links {
		l := &o.diagnostic.raw.Links[i]
		if l.Index != f.h.Link.Index {
			continue
		}
		switch f.mode {
		case "metadata-value":
			l.Raw[40] = u32(999)
		case "metadata-absence":
			delete(l.Raw, 32)
		case "metadata-width":
			l.Raw[40] = []byte{1}
		case "oper-width":
			l.Raw[16] = []byte{1, 2}
		case "qdisc-change":
			l.Raw[6] = []byte("another\x00")
		case "topology":
			l.Raw[10] = u32(9)
		case "carrier-zero":
			l.Raw[33] = []byte{0}
		case "xdp-program":
			l.Raw[43] = append(attr(2, []byte{1}), attr(4, u32(7))...)
		case "devlink-content":
			l.Raw[62] = []byte{0}
		case "missing-required":
			delete(l.Raw, 13)
		case "outer-unknown":
			l.Raw[999] = []byte{1}
		}
		body := fullSetupBodyTest(*l, f.mode)
		parsed, e := decodeSetupLink(body, f.h)
		if e != nil {
			return e
		}
		*l = parsed
	}
	return nil
}
func fullSetupRunRig(t *testing.T, disabled string) (*operation1Controller, *setupRunFake) {
	c, f := setupRunRig(t, disabled)
	f.full = true
	fullSetupAttrsTest(f.owned())
	return c, f
}
func TestFullOuterProfileActualRunNamedRetentionAndPerPhasePAD(t *testing.T) {
	for _, disabled := range []string{"0", "1"} {
		t.Run(disabled, func(t *testing.T) {
			c, f := fullSetupRunRig(t, disabled)
			tx := c.tx
			e := c.withOwner(context.Background(), func(ctx context.Context) error { return tx.Run(ctx) })
			if e == nil || c.tx.Ready || c.tx.PendingCleanup || c.adapter.session.forwardStep != 3 || c.adapter.disabled == nil || c.adapter.disabled.current() {
				t.Fatal("full graph", e, f.events)
			}
			last := tx.Evidence[len(tx.Evidence)-1]
			if last.Step != "address-add-exclusive" || !last.Error {
				t.Fatal("address refusal not reached", tx.Evidence, f.events)
			}
			alias := c.adapter.aliasPost
			if alias == nil || alias.post.raw.Links[len(alias.post.raw.Links)-1].setupOuter == nil {
				t.Fatal("ordered original readback lost")
			}
			if f.ticket.active {
				t.Fatal("ticket escaped")
			}
			if f.events[len(f.events)-2] != "original-close" || f.events[len(f.events)-1] != "absence" {
				t.Fatal("missing cleanup", f.events)
			}
		})
	}
}
func TestFullOuterProfilePhaseTargetedBindingsAndMalformedCaptures(t *testing.T) {
	for _, phase := range []int{1, 2, 3} {
		for _, mode := range []string{"sequence", "attempt", "generation", "owner", "unsupported-phase", "metadata-width", "oper-width", "topology", "carrier-zero", "xdp-program", "devlink-content", "missing-required", "outer-unknown", "pad-payload", "pad-trailing", "duplicate"} {
			t.Run(string(rune('0'+phase))+"/"+mode, func(t *testing.T) {
				c, f := fullSetupRunRig(t, "0")
				f.mode = mode
				f.corruptPhase = phase
				tx := c.tx
				e := c.withOwner(context.Background(), func(ctx context.Context) error { return tx.Run(ctx) })
				if e == nil || tx.Ready || tx.PendingCleanup || !tx.stopStarted {
					t.Fatal("invalid capture/cleanup", e, f.events)
				}
				alias, disable := false, false
				for _, event := range f.events {
					if event == "alias-readback" {
						alias = true
					}
					if event == "disable-readback" {
						disable = true
					}
				}
				if alias != (phase >= 2) || disable != (phase >= 3) {
					t.Fatal("not phase targeted", phase, f.events)
				}
				for _, ev := range tx.Evidence {
					if ev.Step == "address-add-exclusive-intent" {
						t.Fatal("bad Snapshot reached address", phase, f.events)
					}
				}
			})
		}
	}
}
func TestFullOuterProfileLaterMetadataDriftRefusesNotRuntimeMeaning(t *testing.T) {
	for _, phase := range []int{2, 3} {
		for _, mode := range []string{"metadata-value", "metadata-absence", "qdisc-change"} {
			t.Run(string(rune('0'+phase))+"/"+mode, func(t *testing.T) {
				c, f := fullSetupRunRig(t, "0")
				f.mode = mode
				f.corruptPhase = phase
				tx := c.tx
				if e := c.withOwner(context.Background(), func(ctx context.Context) error { return tx.Run(ctx) }); e == nil || tx.Ready || tx.PendingCleanup {
					t.Fatal(e, f.events)
				}
				for _, ev := range tx.Evidence {
					if ev.Step == "address-add-exclusive-intent" {
						t.Fatal("metadata drift escaped", f.events)
					}
				}
			})
		}
	}
}
func TestFullOuterPADRecordsRetainedButNamedComparisonIgnoresLayout(t *testing.T) {
	_, f := fullSetupRunRig(t, "0")
	a, e := f.captureSetupOuter()
	if e != nil {
		t.Fatal(e)
	}
	l := a.Links[len(a.Links)-1]
	body := append([]byte(nil), l.setupOuter.body...)
	// Add second valid policy site without changing named attributes.
	var different []byte
	different = append(different, body[:16]...)
	for raw := body[16:]; len(raw) > 0; {
		n := int(binary.NativeEndian.Uint16(raw))
		aligned := (n + 3) &^ 3
		typ := binary.NativeEndian.Uint16(raw[2:])
		if typ == 14 {
			different = append(different, attr(42, nil)...)
		}
		different = append(different, raw[:aligned]...)
		raw = raw[aligned:]
	}
	other, e := decodeSetupLink(different, f.h)
	if e != nil {
		t.Fatal(e)
	}
	if reflect.DeepEqual(l.setupOuter.body, other.setupOuter.body) || !reflect.DeepEqual(l.Raw, other.Raw) {
		t.Fatal("wire record or named attrs lost")
	}
	b := cloneRawExact(a)
	b.Links[len(b.Links)-1] = other
	if !exactSetupPostMatches(a, b) {
		t.Fatal("PAD incorrectly retained as cross-phase metadata")
	}
	if e := validateSetupOuter(other); e != nil {
		t.Fatal(e)
	}
	// No generic duplicate relaxation, even for PAD.
	if _, e := DecodeLink(different); e == nil {
		t.Fatal("owned policy leaked into generic decoder")
	}
}

func TestFullOuterMetadataIsShapeOnlyAndCopiedWithoutMutableAlias(t *testing.T) {
	_, f := fullSetupRunRig(t, "0")
	for _, k := range []uint16{40, 41, 58, 59, 60, 63, 64} {
		f.owned().Raw[k] = u32(0)
	}
	f.owned().Raw[16] = []byte{255}
	raw, e := f.captureSetupOuter()
	if e != nil {
		t.Fatal("invented allocator or OPER limit", e)
	}
	copy := cloneRawExact(raw)
	l := &copy.Links[len(copy.Links)-1]
	l.setupOuter.body[0] = 1
	if raw.Links[len(raw.Links)-1].setupOuter.body[0] != 0 {
		t.Fatal("wire bytes aliased")
	}
	if e := validateSetupOuter(*l); e == nil {
		t.Fatal("mutated retained header accepted")
	}
	for _, mode := range []string{"pad-before-name", "flagged-pad", "pad-run", "plain-devlink", "plain-dpll", "flagged-linkinfo"} {
		t.Run(mode, func(t *testing.T) {
			body := fullSetupBodyTest(*f.owned(), "")
			switch mode {
			case "pad-before-name":
				body = append(append(append([]byte(nil), body[:16]...), attr(42, nil)...), body[16:]...)
			case "flagged-pad":
				body = append(body, attr(42|0x8000, nil)...)
			case "pad-run":
				body = append(append(append([]byte(nil), body[:16]...), append(attr(42, nil), attr(42, nil)...)...), body[16:]...)
			default:
				for pos := 16; pos < len(body); {
					n := int(binary.NativeEndian.Uint16(body[pos:]))
					typ := binary.NativeEndian.Uint16(body[pos+2:])
					if mode == "plain-devlink" && typ == 62|0x8000 {
						binary.NativeEndian.PutUint16(body[pos+2:], 62)
					}
					if mode == "plain-dpll" && typ == 65|0x8000 {
						binary.NativeEndian.PutUint16(body[pos+2:], 65)
					}
					if mode == "flagged-linkinfo" && typ == 18 {
						binary.NativeEndian.PutUint16(body[pos+2:], 18|0x8000)
					}
					pos += (n + 3) &^ 3
				}
			}
			if _, e := decodeSetupLink(body, f.h); e == nil {
				t.Fatal("unsupported encoding admitted", mode)
			}
		})
	}
}
