//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
)

func routeReadRequestTest(kind uint16) []byte {
	body, _ := DumpRequestHeader(kind)
	out := make([]byte, 16+len(body))
	binary.NativeEndian.PutUint32(out, uint32(len(out)))
	binary.NativeEndian.PutUint16(out[4:], kind)
	binary.NativeEndian.PutUint16(out[6:], 0x301)
	binary.NativeEndian.PutUint32(out[8:], 1)
	binary.NativeEndian.PutUint32(out[12:], 77)
	copy(out[16:], body)
	return out
}
func TestRouteSendAllowlistCannotBypassWithMutationKinds(t *testing.T) {
	for _, kind := range []uint16{16, 17, 19, 20, 21, 24, 25, 27, 28, 32, 33, 35, 999} {
		request := routeReadRequestTest(18)
		binary.NativeEndian.PutUint16(request[4:], kind)
		called := false
		if e := guardReadOnlyRouteSend(request, 77, func() error { called = true; return nil }); e == nil || called {
			t.Fatal("forbidden sent", kind)
		}
	}
	for _, kind := range []uint16{18, 22, 26, 34} {
		request := routeReadRequestTest(kind)
		called := false
		if e := guardReadOnlyRouteSend(request, 77, func() error { called = true; return nil }); e != nil || !called {
			t.Fatal("read refused", kind, e)
		}
		request[len(request)-1] = 1
		called = false
		if e := guardReadOnlyRouteSend(request, 77, func() error { called = true; return nil }); e == nil || called {
			t.Fatal("nonexact body sent", kind)
		}
	}
}
func setupRig(t *testing.T) (*packetSetupTransition, *heldPacketLedger, ownedSourceDiagnostic) {
	t.Helper()
	b, _, m := packetBindingRig(t, false)
	id := heldPacketIdentity{fdObject: m.sample.original, generation: 1, target: m.sample.namespace, device: m.sample.namespace, identity: m.sample.identity, handle: m.sample.handle}
	l, e := newHeldPacketLedger(id)
	if e != nil {
		t.Fatal(e)
	}
	link := m.sample.link
	link.Raw = cloneRawAttrs(link.Raw)
	link.Raw[18] = nestedTunTest()
	sys := SysctlSnapshot{}
	for _, p := range effectiveV4Paths() {
		sys[p] = "0"
	}
	held, e := heldSourceSnapshot(l, b.owner, b.session)
	if e != nil {
		t.Fatal(e)
	}
	d, e := diagnosticForHeldRecords(RawInventory{Links: []LinkRecord{link}}, sys, held)
	if e != nil {
		t.Fatal(e)
	}
	token, e := l.beginExactSetup(b.plan, b.plan.Operations[1], d, 33, 77)
	if e != nil {
		t.Fatal(e)
	}
	return token, l, d
}
func setupACKTest(token *packetSetupTransition) Datagram {
	b := make([]byte, 36)
	binary.NativeEndian.PutUint32(b, 36)
	binary.NativeEndian.PutUint16(b[4:], 2)
	binary.NativeEndian.PutUint16(b[6:], 0x100)
	copy(b[8:16], token.request[8:16])
	copy(b[20:], token.request[:16])
	return Datagram{Bytes: b}
}

type setupMemoryWire struct {
	token    *packetSetupTransition
	fail     string
	callback func()
	effects  int
}

func (*setupMemoryWire) inertSetupWireOnly() {}
func (w *setupMemoryWire) setupEffect(_ context.Context, request []byte) (Datagram, error) {
	w.effects++
	if !bytesEqualTest(request, w.token.request) || !w.token.ledger.snapshot().uncertain {
		return Datagram{}, ErrConflict
	}
	if w.fail == "effect" {
		return Datagram{}, errors.New("fake ambiguous effect")
	}
	d := setupACKTest(w.token)
	switch w.fail {
	case "ack-short":
		d.Bytes = d.Bytes[:35]
	case "ack-error":
		d.Bytes[16] = 1
	case "ack-sequence":
		d.Bytes[8]++
	case "ack-echo":
		d.Bytes[24]++
	case "ack-sender":
		d.SenderPID = 1
	}
	return d, nil
}
func bytesEqualTest(a, b []byte) bool { return reflect.DeepEqual(a, b) }
func (w *setupMemoryWire) setupReadback(context.Context) (setupReadbackRecord, error) {
	if w.fail == "read" {
		return setupReadbackRecord{}, errors.New("fake read unavailable")
	}
	id := w.token.ledger.snapshot()
	out := setupReadbackRecord{before: id, after: id, raw: cloneRawExact(w.token.expectedRaw), sys: cloneSysExact(w.token.expectedSys), names: &verifiedFeatureNames{values: pinnedFeatureNamesV617}, features: ethtoolActiveFeatures{active: 1<<0 | 1<<6 | 1<<7 | 1<<11 | 1<<14 | 1<<45}}
	switch w.fail {
	case "replacement":
		out.after.fdObject = &packetOriginalObject{serial: id.fdObject.serial}
	case "netns":
		out.after.device.Inode++
	case "drift":
		out.raw.Links[0].Raw[999] = []byte{1}
	case "all":
		out.sys["/proc/sys/net/ipv4/conf/all/send_redirects"] = "1"
	case "partial":
		out.raw.Links[0].MTU = 1500
		out.raw.Links[0].Raw[4] = u32(1500)
	case "vnet":
		info, _ := schemaAttrs(out.raw.Links[0].Raw[18], map[uint16]bool{1: false, 2: false})
		info[2][20] = 1
		out.raw.Links[0].Raw[18] = append(attr(1, info[1]), attr(2, info[2])...)
	case "offload":
		out.features.active = 1 << 3
	case "names":
		out.names.values[3] = "unknown"
	}
	if w.callback != nil {
		w.callback()
	}
	return out, nil
}
func TestExactSetupACKAndPoststateMintsOnceAfterDoubleGuard(t *testing.T) {
	token, l, _ := setupRig(t)
	before := l.snapshot()
	w := &setupMemoryWire{token: token}
	// Additional authorized guard beyond the wrapper guard. No delta1 assumption.
	if e := token.guardAgain(); e != nil {
		t.Fatal(e)
	}
	if e := token.runInert(context.Background(), w); e != nil {
		t.Fatal(e)
	}
	after := l.snapshot()
	if after.uncertain || after.closed || !after.handle.TagConfirmed || after.fdObject != before.fdObject || after.generation <= before.generation+1 || w.effects != 1 {
		t.Fatal("mint", after)
	}
	if e := token.runInert(context.Background(), w); e == nil || w.effects != 1 {
		t.Fatal("reused setup")
	}
}
func TestExactSetupFailuresAndConcurrentInvalidationNeverRevive(t *testing.T) {
	for _, mode := range []string{"effect", "ack-short", "ack-error", "ack-sequence", "ack-echo", "ack-sender", "read", "replacement", "netns", "drift", "all", "partial", "vnet", "offload", "names", "close", "unrelated-config", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			token, l, _ := setupRig(t)
			w := &setupMemoryWire{token: token, fail: mode}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "close" || mode == "unrelated-config" {
				w.callback = func() {
					done := make(chan struct{})
					go func() {
						kind := "close-intent"
						if mode == "unrelated-config" {
							kind = "config-intent"
						}
						_ = l.invalidate(kind)
						close(done)
					}()
					<-done
				}
			}
			if mode == "timeout" {
				w.callback = cancel
			}
			if e := token.runInert(ctx, w); e == nil {
				t.Fatal("accepted", mode)
			}
			if !l.snapshot().uncertain {
				t.Fatal("revived", mode)
			}
			if e := token.finish(&setupACKEvidence{transition: token, request: token.request}, nil); e == nil {
				t.Fatal("revived dead token")
			}
		})
	}
}
func TestExactSetupTypedACKPostCannotCrossTokensOrUnknownPlan(t *testing.T) {
	token, l, d := setupRig(t)
	if e := token.guardAgain(); e != nil {
		t.Fatal(e)
	}
	ack, e := token.parseACK(setupACKTest(token))
	if e != nil {
		t.Fatal(e)
	}
	w := &setupMemoryWire{token: token}
	raw, e := w.setupReadback(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	post, e := token.parsePost(raw)
	if e != nil {
		t.Fatal(e)
	}
	other, _, _ := setupRig(t)
	ack.transition = other
	if e = token.finish(ack, post); e == nil || !l.snapshot().uncertain {
		t.Fatal("cross token evidence")
	}
	if _, e = l.beginExactSetup(token.plan, token.plan.Operations[2], d, 34, 77); e == nil {
		t.Fatal("unsupported setup step revived")
	}
}

func TestRouteReadGateRejectsAlteredFlagsIdentityAndLength(t *testing.T) {
	for _, mode := range []string{"flags", "sequence", "pid", "size", "trailing", "body"} {
		raw := routeReadRequestTest(18)
		switch mode {
		case "flags":
			binary.NativeEndian.PutUint16(raw[6:], 1)
		case "sequence":
			binary.NativeEndian.PutUint32(raw[8:], 0)
		case "pid":
			binary.NativeEndian.PutUint32(raw[12:], 78)
		case "size":
			raw[0]--
		case "trailing":
			raw = append(raw, 0)
		case "body":
			raw[16] = 2
		}
		called := false
		if e := guardReadOnlyRouteSend(raw, 77, func() error { called = true; return nil }); e == nil || called {
			t.Fatal("nonexact read", mode)
		}
	}
}
