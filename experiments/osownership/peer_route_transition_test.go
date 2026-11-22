//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func peerRouteEmitter(h Handle) []byte {
	b, _ := EncodeRoute(h.Route)
	return append(b, attr(15, u32(254))...)
}

type peerRouteRunFake struct {
	*upRunFake
	fault               string
	sends, runtimeCalls int
	permit              *peerRouteSendPermit
}

func (f *peerRouteRunFake) bindCreatedOriginal(context.Context, *originalAttempt) (operation1DiagnosticSource, error) {
	return f, nil
}
func (f *peerRouteRunFake) observeSetupPhase(ctx context.Context, t *setupPhaseTicket) (*setupPhaseObservation, error) {
	if t.step != 6 {
		return f.upRunFake.observeSetupPhase(ctx, t)
	}
	f.events = append(f.events, "snapshot6")
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	raw, e := f.captureSetupOuter()
	if e != nil {
		return nil, e
	}
	if f.fault == "phase6-table" {
		raw.Routes[1].Table = 253
	}
	q := cloneUpQdisc(f.qdisc)
	binary.NativeEndian.PutUint32(q.bodies[0][16:], 9)
	return &setupPhaseObservation{qdisc: q, ticket: t, diagnostic: ownedSourceDiagnostic{raw: raw, sys: cloneSysExact(f.sys), held: held, lineageUsable: true}}, nil
}
func (f *peerRouteRunFake) applyOwnedPeerRoute(ctx context.Context, r *postUpResult) (*postPeerRouteResult, error) {
	c := f.c.attempt.custody
	c.mu.Lock()
	defer c.mu.Unlock()
	held, e := heldSourceSnapshot(f.l, f.c.tx, f.c.adapter.session)
	if e != nil {
		return nil, e
	}
	raw, e := f.captureSetupOuter()
	if e != nil {
		return nil, e
	}
	return finishOwnedPeerRoute(ctx, r, ownedSourceDiagnostic{raw: raw, sys: cloneSysExact(f.sys), held: held, lineageUsable: true}, cloneUpQdisc(f.qdisc), 71, 77, ownedPeerRouteDependencies{
		check: func() error { return ctx.Err() },
		send: func(_ context.Context, p *peerRouteSendPermit) (Datagram, error) {
			f.permit = p
			request := append([]byte(nil), p.request...)
			if f.fault == "request" {
				request[6] ^= 0x80
			}
			e := p.send(request, 77, func() error {
				f.sends++
				f.events = append(f.events, "route-effect")
				if r.current() {
					return errors.New("UP ancestry stayed current")
				}
				body := peerRouteEmitter(f.h)
				switch f.fault {
				case "prefsrc":
					body = append(body, attr(7, ipBytes(f.h.Address.Prefix.Addr()))...)
				case "unknown":
					body = append(body, attr(99, u32(0))...)
				case "gateway":
					body = append(body, attr(5, ipBytes(f.h.Address.Prefix.Addr()))...)
				}
				route, e := DecodeRoute(body)
				if e != nil {
					return e
				}
				f.raw.Routes = append(f.raw.Routes, route)
				switch f.fault {
				case "ambiguous":
					return errors.New("send outcome unknown")
				case "residual":
					extra := route
					extra.Table = 200
					extra.TableID = 200
					extra.Raw = cloneRawAttrs(extra.Raw)
					extra.Raw[15] = u32(200)
					f.raw.Routes = append(f.raw.Routes, extra)
				case "missing-local":
					f.raw.Routes = f.raw.Routes[1:]
				case "duplicate-peer":
					f.raw.Routes = append(f.raw.Routes, route)
				case "disable":
					f.word(26, 0)
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
			if f.fault == "ack" {
				ack[16] = 1
			}
			return Datagram{Bytes: ack}, nil
		},
		collect: func(context.Context) (RawInventory, SysctlSnapshot, upQdiscObservation, error) {
			f.events = append(f.events, "route-readback")
			if f.fault == "revoke" {
				f.life.revoke()
			}
			q := cloneUpQdisc(f.qdisc)
			binary.NativeEndian.PutUint32(q.bodies[0][16:], 5)
			if f.fault == "qdisc-options" {
				q.bodies[0][40] ^= 1
			}
			raw, e := f.captureSetupOuter()
			return raw, cloneSysExact(f.sys), q, e
		},
	})
}

type pendingRuntimeFake struct{ owner *Transaction }

func (p *pendingRuntimeFake) validateCompletion(ctx context.Context, t *Transaction) error {
	if p.owner != t {
		return ErrConflict
	}
	return ownedAFPolicyIncomplete
}
func (f *peerRouteRunFake) collectRuntimeCompletion(ctx context.Context, r *postPeerRouteResult) (completionEvidence, error) {
	f.runtimeCalls++
	f.events = append(f.events, "runtime-original-binding")
	if r == nil || !r.current() || r != f.c.adapter.routed {
		return nil, afPhase
	}
	return &pendingRuntimeFake{owner: f.c.tx}, nil
}
func peerRouteRunRig(t *testing.T, fault string) (*operation1Controller, *peerRouteRunFake) {
	t.Helper()
	c, base := upRunRig(t, "")
	f := &peerRouteRunFake{upRunFake: base, fault: fault}
	c.adapter.creation = f
	return c, f
}

type forwardedCompletion struct {
	Kernel
	session *adapterSession
}

func (w *forwardedCompletion) ownershipSession() *adapterSession { return w.session }
func TestActualRunPeerRouteThenRuntimeProducerRefusal(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "wrapper"}[wrapped], func(t *testing.T) {
			c, f := peerRouteRunRig(t, "")
			if wrapped {
				c.tx.k = &forwardedCompletion{Kernel: c.adapter, session: c.adapter.session}
			}
			e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.tx.Run(ctx) })
			if e == nil || c.tx.Ready || c.tx.PendingCleanup || f.sends != 1 || f.runtimeCalls != 1 || c.adapter.session.forwardStep != 6 || c.adapter.routed == nil || c.adapter.routed.current() || f.life.current() {
				t.Fatal("route/runtime chain", e, f.events)
			}
			if !strings.Contains(strings.Join(f.events, ","), "route-effect,route-readback,snapshot6,runtime-original-binding,original-close,absence") {
				t.Fatal(f.events)
			}
			last := c.tx.Evidence[len(c.tx.Evidence)-1]
			if last.Step != "route-add-exclusive" || last.Error || last.Readback != "true" {
				t.Fatal(c.tx.Evidence)
			}
			calls := 0
			if f.permit.send(f.permit.request, 77, func() error { calls++; return nil }) == nil || calls != 0 {
				t.Fatal("retry")
			}
		})
	}
}
func TestActualRunPeerRouteFaultsCleanWithoutRetry(t *testing.T) {
	for _, fault := range []string{"request", "ambiguous", "ack", "prefsrc", "gateway", "unknown", "residual", "missing-local", "duplicate-peer", "disable", "revoke", "qdisc-options", "phase6-table"} {
		t.Run(fault, func(t *testing.T) {
			c, f := peerRouteRunRig(t, fault)
			e := c.withOwner(context.Background(), func(ctx context.Context) error { return c.tx.Run(ctx) })
			if e == nil || c.tx.Ready || c.tx.PendingCleanup || f.sends > 1 || f.runtimeCalls != 0 || f.life.current() {
				t.Fatal(e, f.events)
			}
			if len(f.events) < 2 || f.events[len(f.events)-2] != "original-close" || f.events[len(f.events)-1] != "absence" {
				t.Fatal(f.events)
			}
		})
	}
}
func TestConstructionQdiscStructuralVolatility(t *testing.T) {
	a := upQdiscObservation{bodies: [][]byte{upQdiscEmitter(7), upQdiscEmitter(8)}}
	b := cloneUpQdisc(a)
	binary.NativeEndian.PutUint32(b.bodies[0][16:], 12)
	binary.NativeEndian.PutUint32(b.bodies[1][16:], 13)
	if !sameConstructionQdisc(a, b) {
		t.Fatal("positive refcounts")
	}
	b.bodies[0], b.bodies[1] = b.bodies[1], b.bodies[0]
	if !sameConstructionQdisc(a, b) {
		t.Fatal("order")
	}
	for _, fault := range []string{"zero-ref", "foreign-options", "foreign-traffic", "foreign-kind", "duplicate"} {
		t.Run(fault, func(t *testing.T) {
			b := cloneUpQdisc(a)
			switch fault {
			case "zero-ref":
				binary.NativeEndian.PutUint32(b.bodies[0][16:], 0)
			case "foreign-options":
				b.bodies[1][40] ^= 1
			case "foreign-kind":
				b.bodies[1][24] = 'x'
			case "foreign-traffic":
				b.bodies[1][len(b.bodies[1])-40] = 1
			case "duplicate":
				b.bodies = append(b.bodies, append([]byte(nil), b.bodies[1]...))
			}
			if sameConstructionQdisc(a, b) {
				t.Fatal("accepted", fault)
			}
		})
	}
}

type invalidatingCompletion struct {
	testOnlyCompletion
	ledger *heldPacketLedger
}

func (p *invalidatingCompletion) validateCompletion(ctx context.Context, t *Transaction) error {
	_ = p.ledger.invalidate("custody-loss")
	return nil
}
func TestCompletionReceiptOwnerGenerationAndReplay(t *testing.T) {
	for _, fault := range []string{"owner", "authority", "handle", "generation-before", "generation-during", "nil-proof", "replay"} {
		t.Run(fault, func(t *testing.T) {
			tx, k, _ := transactionRig(t)
			if e := tx.Start(context.Background()); e != nil {
				t.Fatal(e)
			}
			tx.Ready = false
			r, e := k.complete(context.Background(), tx)
			if e != nil {
				t.Fatal(e)
			}
			switch fault {
			case "owner":
				r.owner = &Transaction{}
			case "authority":
				r.authority = &completionAuthority{owner: tx}
			case "handle":
				r.identity.handle.Link.Index++
			case "generation-before":
				_ = r.ledger.invalidate("custody-loss")
			case "generation-during":
				r.evidence = &invalidatingCompletion{testOnlyCompletion: testOnlyCompletion{owner: tx}, ledger: r.ledger}
			case "nil-proof":
				r.evidence = nil
			case "replay":
				if e := tx.consumeCompletion(context.Background(), r); e != nil {
					t.Fatal(e)
				}
			}
			if tx.consumeCompletion(context.Background(), r) == nil {
				t.Fatal("invalid receipt accepted", fault)
			}
		})
	}
}
