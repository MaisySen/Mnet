//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"time"
)

// No production issuer. This test-only proof preserves inert transaction tests
// without adding an opt-out constructor or synthetic success to source builds.
type testOnlyCompletion struct{ owner *Transaction }

func (p *testOnlyCompletion) validateCompletion(ctx context.Context, t *Transaction) error {
	if p == nil || p.owner != t {
		return ErrConflict
	}
	return ctx.Err()
}
func (k *fakeKernel) complete(ctx context.Context, t *Transaction) (*completionReceipt, error) {
	if t == nil || t.handle == nil {
		return nil, ErrConflict
	}
	id := heldPacketIdentity{generation: 1, fdObject: &packetOriginalObject{serial: 1}, handle: *t.handle}
	l := &heldPacketLedger{current: id}
	return &completionReceipt{owner: t, authority: t.completionAuthority, evidence: &testOnlyCompletion{owner: t}, ledger: l, identity: id}, nil
}

func (k *fakeKernel) stop(ctx context.Context, t *Transaction) error { return t.stopLegacyInert(ctx) }

// Test-marker inert protocol only. Source Kernel.stop never calls this path.
func (t *Transaction) stopLegacyInert(ctx context.Context) error {
	t.stopStarted = true // invalidate display authority before the first cleanup read
	bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	ctx = bounded
	t.Ready = false
	if t.fdCloseAttempted && t.fdCloseError != nil {
		t.PendingCleanup = true
		return t.fdCloseError
	}
	if t.handle == nil {
		if t.PendingCleanup {
			return errors.New("ambiguous creation without FD custody; cleanup pending, never adopt")
		}
		return nil
	}
	h := *t.handle
	for steps := 0; steps < 4; steps++ {
		i, e := t.k.Snapshot(ctx)
		if e != nil {
			return e
		}
		ops, e := Rollback(h, i)
		if e != nil {
			return e
		}
		if len(ops) == 0 {
			e = t.closeFDOnce(h)
			if e != nil {
				return e
			}
			if e = verifyAbsence(h, i, t.baseline); e != nil {
				return e
			}
			t.PendingCleanup = false
			return nil
		}
		op := ops[0]
		t.Evidence = append(t.Evidence, Evidence{Step: op.Kind + "-intent"})
		var applyErr error
		if op.Kind == "close-owned-tun-fd" {
			applyErr = t.closeFDOnce(h)
		} else {
			applyErr = t.k.Apply(ctx, h, op)
		}
		after, readErr := t.k.Snapshot(ctx)
		if readErr != nil {
			return errors.Join(applyErr, readErr)
		}
		if op.Kind == "close-owned-tun-fd" && applyErr != nil {
			t.PendingCleanup = true
			return errors.Join(applyErr, readErr)
		}
		remaining, conflict := Rollback(h, after)
		if conflict != nil {
			return errors.Join(applyErr, conflict)
		}
		if operationPresent(op, remaining) {
			return errors.Join(applyErr, errors.New("cleanup not confirmed, remains pending"))
		}
		t.Evidence = append(t.Evidence, Evidence{Step: op.Kind, ACK: applyErr == nil, Readback: "absent", Error: applyErr != nil})
	}
	return errors.New("cleanup step bound reached; pending")
}
