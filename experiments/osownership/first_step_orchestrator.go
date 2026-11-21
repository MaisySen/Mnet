//go:build mnet_enrollment_development

package osownership

import "context"

// Private dependency seam shared by the source orchestrator and fake-only tests.
// The caller owns original file custody for this entire invocation. send must
// use this exact permit at its final send boundary, not a parallel invalidator.
// No arbitrary callback is installed in a production adapter or public API.
type firstStepDependencies struct {
	send    func(context.Context, *packetSetupTransition, *firstLinkSendPermit) (Datagram, error)
	collect func(context.Context, *packetSetupTransition) (setupReadbackRecord, error)
}

// One token, one send, one sequential collection, one parse/finish. Success
// refreshes ONLY packet lineage for the first alias/MTU operation. It does not
// advance adapterSession, transaction phase, strict Compose or Ready.
type firstStepCompletion struct {
	token    *packetSetupTransition
	ack      *setupACKEvidence
	post     *setupPostEvidence
	consumed bool
}

func orchestrateFirstStep(ctx context.Context, l *heldPacketLedger, p Plan, d ownedSourceDiagnostic, sequence, port uint32, deps firstStepDependencies) error {
	_, e := orchestrateFirstStepResult(ctx, l, p, d, sequence, port, deps)
	return e
}
func orchestrateFirstStepResult(ctx context.Context, l *heldPacketLedger, p Plan, d ownedSourceDiagnostic, sequence, port uint32, deps firstStepDependencies) (result *firstStepCompletion, err error) {
	if ctx == nil || ctx.Err() != nil || l == nil || deps.send == nil || deps.collect == nil || len(p.Operations) != 6 {
		return nil, ErrConflict
	}
	token, e := l.beginExactSetup(p, p.Operations[1], d, sequence, port)
	if e != nil {
		return nil, e
	}
	defer func() {
		if err != nil {
			token.abandon()
		}
	}()
	permit := &firstLinkSendPermit{token: token}
	reply, e := deps.send(ctx, token, permit)
	if e != nil {
		return nil, e
	}
	if !permit.used {
		return nil, ErrConflict
	}
	ack, e := token.parseACK(reply)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	// A canceled token must not start new expensive reads. parsePost and finish
	// also recheck generation, including invalidation DURING collection.
	if e = token.currentForCollection(); e != nil {
		return nil, e
	}
	record, e := deps.collect(ctx, token)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if e = token.currentForCollection(); e != nil {
		return nil, e
	}
	post, e := token.parsePost(record)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if e = token.finish(ack, post); e != nil {
		return nil, e
	}
	return &firstStepCompletion{token: token, ack: ack, post: post}, nil
}
func (t *packetSetupTransition) currentForCollection() error {
	if t == nil || t.ledger == nil {
		return ErrConflict
	}
	l := t.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.setup != t || t.done || l.current.closed || l.current.generation != t.generation || t.owner == nil || t.owner.stopStarted || t.owner.fdCloseAttempted || t.owner.clock == nil || t.owner.deadline.expired(t.owner.clock.Now()) {
		return ErrConflict
	}
	return nil
}
