//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"testing"
)

// Reuse records, not an already-begun token. Exercise the actual common
// begin/send/parse/collect/parse/finish orchestrator with fake effects only.
func firstStepRig(t *testing.T) (*heldPacketLedger, Plan, ownedSourceDiagnostic) {
	old, _, d := setupRig(t)
	l, e := newHeldPacketLedger(old.original)
	if e != nil {
		t.Fatal(e)
	}
	held, e := heldSourceSnapshot(l, old.owner, d.held.session)
	if e != nil {
		t.Fatal(e)
	}
	diagnostic, e := diagnosticForHeldRecords(d.raw, d.sys, held)
	if e != nil {
		t.Fatal(e)
	}
	return l, old.plan, diagnostic
}
func TestFirstStepOrchestratorActualSeam(t *testing.T) {
	for _, mode := range []string{"success", "send-error", "ack-error", "invalidate-after-ack", "invalidate-in-read", "close-after-ack", "close-in-read", "timeout-after-ack", "timeout-in-read", "owner-expired-after-ack", "owner-expired-in-read", "read-error", "wrong-readback", "permit-bypass"} {
		t.Run(mode, func(t *testing.T) {
			l, p, d := firstStepRig(t)
			before := l.snapshot()
			session := d.held.session
			step := session.forwardStep
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sends, reads := 0, 0
			deps := firstStepDependencies{
				send: func(ctx context.Context, token *packetSetupTransition, permit *firstLinkSendPermit) (Datagram, error) {
					if mode == "permit-bypass" {
						return setupACKTest(token), nil
					}
					e := permit.send(token.request, 77, func() error {
						sends++
						if mode == "send-error" {
							return ErrConflict
						}
						return nil
					})
					if e != nil {
						return Datagram{}, e
					}
					ack := setupACKTest(token)
					switch mode {
					case "ack-error":
						ack.Bytes[16] = 1
					case "invalidate-after-ack":
						_ = l.invalidate("config-intent")
					case "close-after-ack":
						_ = l.invalidate("close-intent")
					case "timeout-after-ack":
						cancel()
					case "owner-expired-after-ack":
						token.owner.clock.(*fakeClock).s.Monotonic = token.owner.deadline.Mono
					}
					return ack, nil
				},
				collect: func(ctx context.Context, token *packetSetupTransition) (setupReadbackRecord, error) {
					reads++
					w := &setupMemoryWire{token: token}
					record, e := w.setupReadback(ctx)
					if e != nil {
						return record, e
					}
					switch mode {
					case "invalidate-in-read":
						_ = l.invalidate("config-intent")
					case "close-in-read":
						_ = l.invalidate("close-intent")
					case "timeout-in-read":
						cancel()
					case "owner-expired-in-read":
						token.owner.clock.(*fakeClock).s.Monotonic = token.owner.deadline.Mono
					case "read-error":
						return record, ErrConflict
					case "wrong-readback":
						record.after.fdObject = &packetOriginalObject{serial: 1}
					}
					return record, nil
				},
			}
			e := orchestrateFirstStep(ctx, l, p, d, 33, 77, deps)
			after := l.snapshot()
			if mode == "success" {
				if e != nil || after.uncertain || after.closed || !after.handle.TagConfirmed || after.generation <= before.generation || sends != 1 || reads != 1 {
					t.Fatal("not exact once success", e, after, sends, reads)
				}
			} else {
				if e == nil || !after.uncertain || after.handle.TagConfirmed {
					t.Fatal("failure minted", e, after)
				}
				wantReads := 0
				switch mode {
				case "invalidate-in-read", "close-in-read", "timeout-in-read", "owner-expired-in-read", "read-error", "wrong-readback":
					wantReads = 1
				}
				if reads != wantReads {
					t.Fatal("unexpected read/retry", reads, wantReads)
				}
				if mode == "permit-bypass" {
					if sends != 0 {
						t.Fatal("bypass effect")
					}
				} else if sends != 1 {
					t.Fatal("effect retry", sends)
				}
			}
			if session.forwardStep != step || session.activeEligible() || d.held.owner.Ready {
				t.Fatal("advanced adapter or Ready")
			}
			if after.fdObject != before.fdObject {
				t.Fatal("replacement")
			}
			// Neither success nor failure can silently replay the same diagnostic.
			priorSends := sends
			if e = orchestrateFirstStep(ctx, l, p, d, 33, 77, deps); e == nil || sends != priorSends {
				t.Fatal("replayed", e, sends)
			}
		})
	}
}
