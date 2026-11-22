//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func upQdiscEmitter(index int) []byte {
	b := make([]byte, 20)
	binary.NativeEndian.PutUint32(b[4:], uint32(index))
	binary.NativeEndian.PutUint32(b[12:], 0xffffffff)
	binary.NativeEndian.PutUint32(b[16:], 2)
	options := append(u32(3), []byte{1, 2, 2, 2, 1, 2, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1}...)
	stats := append(attr(1, make([]byte, 16)), attr(3, make([]byte, 20))...)
	for _, a := range [][]byte{attr(1, []byte("pfifo_fast\x00")), attr(2, options), attr(12, []byte{0}), attr(7, stats), attr(3, make([]byte, 40))} {
		b = append(b, a...)
	}
	return b
}

type upRunFake struct {
	*addressRunFake
	upFault  string
	upSends  int
	upPermit *upSendPermit
	qdisc    upQdiscObservation
}

func (f *upRunFake) bindCreatedOriginal(context.Context, *originalAttempt) (operation1DiagnosticSource, error) {
	return f, nil
}
func (f *upRunFake) observeSetupPhase(ctx context.Context, t *setupPhaseTicket) (*setupPhaseObservation, error) {
	if t.step != 5 {
		return f.addressRunFake.observeSetupPhase(ctx, t)
	}
	f.events = append(f.events, "snapshot5")
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	raw, e := f.captureSetupOuter()
	if e != nil {
		return nil, e
	}
	q := cloneUpQdisc(f.qdisc)
	if f.upFault == "phase5-qdisc" {
		q.bodies[0] = append(q.bodies[0], attr(99, u32(0))...)
	}
	return &setupPhaseObservation{qdisc: q, ticket: t, diagnostic: ownedSourceDiagnostic{raw: raw, sys: cloneSysExact(f.sys), held: held, lineageUsable: true}}, nil
}
func (f *upRunFake) applyOwnedUp(ctx context.Context, r *postAddressDownResult) (*postUpResult, error) {
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
	qbefore := cloneUpQdisc(f.qdisc)
	return finishOwnedUp(ctx, r, ownedSourceDiagnostic{raw: raw, sys: cloneSysExact(f.sys), held: held, lineageUsable: true}, qbefore, 66, 77, ownedUpDependencies{
		check: func() error { return ctx.Err() },
		send: func(_ context.Context, p *upSendPermit) (Datagram, error) {
			f.upPermit = p
			request := append([]byte(nil), p.request...)
			if f.upFault == "wrong-request" {
				request[8] ^= 1
			}
			e := p.send(request, 77, func() error {
				f.upSends++
				f.events = append(f.events, "up-effect")
				if r.current() {
					return errors.New("address result stayed current")
				}
				l := f.owned()
				l.Flags = 0x110d1
				l.Raw[16] = []byte{6}
				l.Raw[6] = []byte("pfifo_fast\x00")
				f.qdisc = upQdiscObservation{bodies: [][]byte{upQdiscEmitter(f.h.Link.Index)}}
				switch f.upFault {
				case "unsupported-kind":
					b := f.qdisc.bodies[0]
					copy(b[24:], []byte("fq_codel\x00\x00\x00"))
				case "send-error":
					return errors.New("ambiguous UP")
				case "carrier":
					l.Raw[33] = []byte{0}
				case "oper":
					l.Raw[16] = []byte{2}
				case "disable":
					f.word(26, 0)
				case "route":
					f.raw.Routes = nil
				case "qdisc-extra":
					f.qdisc.bodies = append(f.qdisc.bodies, upQdiscEmitter(f.h.Link.Index))
				case "qdisc-bpf":
					f.qdisc.bodies[0] = append(f.qdisc.bodies[0], attr(13, u32(5))...)
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
			if f.upFault == "ack" {
				ack[16] = 1
			}
			return Datagram{Bytes: ack}, nil
		},
		collect: func(context.Context) (RawInventory, SysctlSnapshot, upQdiscObservation, error) {
			f.events = append(f.events, "up-readback")
			if f.upFault == "stop" {
				f.life.revoke()
			}
			raw, e := f.captureSetupOuter()
			return raw, cloneSysExact(f.sys), cloneUpQdisc(f.qdisc), e
		},
	})
}
func upRunRig(t *testing.T, fault string) (*operation1Controller, *upRunFake) {
	t.Helper()
	c, base := addressRunRig(t, "")
	base.full = true
	fullSetupAttrsTest(base.owned())
	f := &upRunFake{addressRunFake: base, upFault: fault, qdisc: upQdiscObservation{}}
	c.adapter.creation = f
	return c, f
}
func TestActualRunUPThenPeerRouteRefusal(t *testing.T) {
	c, f := upRunRig(t, "")
	e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.tx.Run(ctx) })
	if e == nil || c.tx.Ready || c.tx.PendingCleanup || f.upSends != 1 || c.adapter.session.forwardStep != 5 || c.adapter.upped == nil || c.adapter.upped.current() || f.life.current() {
		t.Fatal("UP graph", e, f.events)
	}
	if c.adapter.addressed.current() {
		t.Fatal("stale address ancestry")
	}
	if !strings.Contains(strings.Join(f.events, ","), "up-effect,up-readback,snapshot5,snapshot5,original-close,absence") {
		t.Fatal("connected phase5 missing", f.events)
	}
	last := c.tx.Evidence[len(c.tx.Evidence)-1]
	if last.Step != "route-add-exclusive" || !last.Error {
		t.Fatal("peer route not refused", c.tx.Evidence)
	}
	calls := 0
	if f.upPermit.send(f.upPermit.request, 77, func() error { calls++; return nil }) == nil || calls != 0 {
		t.Fatal("UP replay")
	}
	if _, e := c.adapter.complete(context.Background(), c.tx); e == nil {
		t.Fatal("construction became Ready")
	}
}
func TestActualRunUPFaultsCleanWithoutRetry(t *testing.T) {
	for _, fault := range []string{"unsupported-kind", "wrong-request", "send-error", "ack", "carrier", "oper", "disable", "route", "qdisc-extra", "qdisc-bpf", "stop", "phase5-qdisc"} {
		t.Run(fault, func(t *testing.T) {
			c, f := upRunRig(t, fault)
			e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.tx.Run(ctx) })
			if e == nil || c.tx.Ready || c.tx.PendingCleanup || f.upSends > 1 || f.life.current() {
				t.Fatal("fault escaped", e, f.events)
			}
			if len(f.events) < 2 || f.events[len(f.events)-2] != "original-close" || f.events[len(f.events)-1] != "absence" {
				t.Fatal("cleanup", f.events)
			}
		})
	}
}
func TestUPQdiscProfileRejectsUnsupportedGraph(t *testing.T) {
	good := upQdiscObservation{bodies: [][]byte{upQdiscEmitter(7)}}
	if _, e := parseUpQdisc(good, 7); e != nil {
		t.Fatal(e)
	}
	for _, k := range []uint16{4, 5, 8, 11, 13, 14, 99} {
		q := cloneUpQdisc(good)
		q.bodies[0] = append(q.bodies[0], attr(k, u32(0))...)
		if _, e := parseUpQdisc(q, 7); e == nil {
			t.Fatal("graph extra", k)
		}
	}
}
