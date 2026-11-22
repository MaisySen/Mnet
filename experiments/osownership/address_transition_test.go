//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type addressRunFake struct {
	*setupRunFake
	fault   string
	sends   int
	permit  *addressSendPermit
	disable *postDisableResult
}

func (f *addressRunFake) createOriginal(ctx context.Context, p Plan, a *originalAttempt) (Handle, error) {
	h, e := f.setupRunFake.createOriginal(ctx, p, a)
	if e == nil {
		a.custody.closer = f
	}
	return h, e
}
func (f *addressRunFake) Close() error {
	f.events = append(f.events, "original-close")
	if f.life.current() {
		return errors.New("close before result revocation")
	}
	var links []LinkRecord
	for _, l := range f.raw.Links {
		if l.Index != f.h.Link.Index {
			links = append(links, l)
		}
	}
	var addresses []AddressRecord
	for _, a := range f.raw.Addresses {
		if a.Index != f.h.Link.Index {
			addresses = append(addresses, a)
		}
	}
	var routes []RouteRecord
	for _, r := range f.raw.Routes {
		if r.OIF != uint32(f.h.Link.Index) {
			routes = append(routes, r)
		}
	}
	f.raw.Links = links
	f.raw.Addresses = addresses
	f.raw.Routes = routes
	for k := range f.sys {
		if strings.HasPrefix(k, "/proc/sys/net/ipv4/conf/"+f.h.Link.Name+"/") || strings.HasPrefix(k, "/proc/sys/net/ipv6/conf/"+f.h.Link.Name+"/") || k == ipv6DirPrefix+f.h.Link.Name {
			delete(f.sys, k)
		}
	}
	return nil
}
func (f *addressRunFake) observeOriginalAbsence(_ context.Context, a *originalAttempt) (*originalAbsenceEvidence, error) {
	f.events = append(f.events, "absence")
	return parseOriginalAbsence(a, a.absenceTicket, f.c.adapter.session.baseline, cloneRawExact(f.raw), cloneSysExact(f.sys))
}
func (f *addressRunFake) bindCreatedOriginal(context.Context, *originalAttempt) (operation1DiagnosticSource, error) {
	return f, nil
}
func (f *addressRunFake) observeSetupPhase(ctx context.Context, t *setupPhaseTicket) (*setupPhaseObservation, error) {
	if t.step != 4 {
		return f.setupRunFake.observeSetupPhase(ctx, t)
	}
	f.events = append(f.events, "snapshot4")
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	raw, e := f.captureSetupOuter()
	if e != nil {
		return nil, e
	}
	if f.fault == "phase4-cache" {
		raw.Addresses[0].Raw[6][8] ^= 1
	}
	return &setupPhaseObservation{ticket: t, diagnostic: ownedSourceDiagnostic{raw: raw, sys: cloneSysExact(f.sys), held: held, lineageUsable: true}}, nil
}
func (f *addressRunFake) applyOwnedAddress(ctx context.Context, d *postDisableResult) (*postAddressDownResult, error) {
	f.disable = d
	custody := f.c.attempt.custody
	custody.mu.Lock()
	defer custody.mu.Unlock()
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	raw, e := f.captureSetupOuter()
	if e != nil {
		return nil, e
	}
	return finishOwnedAddressDown(ctx, d, ownedSourceDiagnostic{raw: raw, sys: cloneSysExact(f.sys), held: held, lineageUsable: true}, 51, 77, ownedAddressDependencies{
		check: func() error { return ctx.Err() },
		send: func(_ context.Context, p *addressSendPermit) (Datagram, error) {
			f.permit = p
			request := append([]byte(nil), p.request...)
			if f.fault == "wrong-sequence" {
				request[8] ^= 1
			}
			if f.fault == "wrong-body" {
				request[len(request)-1] ^= 1
			}
			if f.fault == "generation" {
				_ = f.l.invalidate("export-intent")
			}
			e := p.send(request, 77, func() error {
				f.sends++
				f.events = append(f.events, "address-effect")
				if d.current() {
					return errors.New("old disable result stayed current")
				}
				// Independent fake device operands, not a predicted inventory clone.
				address, e := DecodeAddress(addressEmitterFixture(&f.h, 123, 456))
				if e != nil {
					return e
				}
				route, e := DecodeRoute(addressLocalRouteEmitter(&f.h))
				if e != nil {
					return e
				}
				f.raw.Addresses = []AddressRecord{address}
				if f.fault != "missing-local-route" {
					f.raw.Routes = []RouteRecord{route}
				}
				if f.fault == "extra-route" {
					f.raw.Routes = append(f.raw.Routes, route)
				}
				if f.fault == "address-error" {
					return errors.New("ambiguous send")
				}
				return nil
			})
			if e != nil {
				return Datagram{}, e
			}
			ack := make([]byte, 36)
			binary.NativeEndian.PutUint32(ack, 36)
			binary.NativeEndian.PutUint16(ack[4:], 2)
			binary.NativeEndian.PutUint16(ack[6:], 0x100)
			copy(ack[8:16], p.request[8:16])
			copy(ack[20:], p.request[:16])
			if f.fault == "negative-ack" {
				binary.NativeEndian.PutUint32(ack[16:], 1)
			}
			return Datagram{Bytes: ack}, nil
		},
		collect: func(context.Context) (RawInventory, SysctlSnapshot, error) {
			f.events = append(f.events, "address-readback")
			if f.fault == "stop-readback" {
				f.life.revoke()
			}
			raw, e := f.captureSetupOuter()
			if e != nil {
				return RawInventory{}, nil, e
			}
			if f.fault == "conf-drift" {
				raw.Links[len(raw.Links)-1].Raw[4] = u32(1300)
			}
			if f.fault == "readback-error" {
				return RawInventory{}, nil, errors.New("readback")
			}
			return raw, cloneSysExact(f.sys), nil
		},
	})
}
func addressRunRig(t *testing.T, fault string) (*operation1Controller, *addressRunFake) {
	t.Helper()
	c, base := setupRunRig(t, "0")
	f := &addressRunFake{setupRunFake: base, fault: fault}
	c.adapter.creation = f
	c.absence = f
	return c, f
}
func TestActualRunAddressDownThenUPRefusal(t *testing.T) {
	c, f := addressRunRig(t, "")
	e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.tx.Run(ctx) })
	if e == nil || c.tx.Ready || c.tx.PendingCleanup || c.adapter.session.forwardStep != 4 || f.sends != 1 || c.adapter.addressed == nil || c.adapter.addressed.current() || f.life.current() {
		t.Fatal("graph or cleanup", e, f.events)
	}
	if f.disable == nil || f.disable.current() {
		t.Fatal("disable ancestry incorrectly current")
	}
	joined := strings.Join(f.events, ",")
	for _, required := range []string{"address-effect,address-readback,snapshot4,snapshot4", "original-close,absence"} {
		if !strings.Contains(joined, required) {
			t.Fatal("missing connected events", joined)
		}
	}
	last := c.tx.Evidence[len(c.tx.Evidence)-1]
	if last.Step != "link-up-exact" || !last.Error {
		t.Fatal("UP not refused", c.tx.Evidence)
	}
	calls := 0
	if f.permit.send(f.permit.request, 77, func() error { calls++; return nil }) == nil || calls != 0 {
		t.Fatal("replayed send")
	}
	if c.adapter.addressed.disabledAncestry != f.disable {
		t.Fatal("lost ancestry")
	}
	if !reflect.DeepEqual(c.adapter.addressed.address.cache, addressDumpCacheinfo{0xffffffff, 0xffffffff, 123, 456}) {
		t.Fatal("wrong captured cache")
	}
}
func TestActualRunAddressFaultsCleanWithoutRetry(t *testing.T) {
	for _, fault := range []string{"wrong-sequence", "wrong-body", "generation", "missing-local-route", "extra-route", "address-error", "negative-ack", "conf-drift", "readback-error", "stop-readback", "phase4-cache"} {
		t.Run(fault, func(t *testing.T) {
			c, f := addressRunRig(t, fault)
			e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.tx.Run(ctx) })
			if e == nil || c.tx.Ready || c.tx.PendingCleanup || !c.tx.stopStarted || f.sends > 1 || f.life.current() {
				t.Fatal("fault escaped", e, f.events)
			}
			if len(f.events) < 2 || f.events[len(f.events)-2] != "original-close" || f.events[len(f.events)-1] != "absence" {
				t.Fatal("cleanup missing", f.events)
			}
			if fault != "phase4-cache" && c.adapter.session.forwardStep != 3 {
				t.Fatal("failed transition advanced", fault, c.adapter.session.forwardStep)
			}
		})
	}
}
