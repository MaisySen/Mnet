//go:build mnet_enrollment_development

package osownership

import "context"

// Mandatory Kernel contract. The only non-source proof implementations live in
// _test.go. No production inert issuer or opt-out constructor exists.
type completionEvidence interface {
	validateCompletion(context.Context, *Transaction) error
}
type completionAuthority struct{ owner *Transaction }
type completionReceipt struct {
	ledger    *heldPacketLedger
	identity  heldPacketIdentity
	owner     *Transaction
	authority *completionAuthority
	evidence  completionEvidence
	used      bool
}

func (t *Transaction) consumeCompletion(ctx context.Context, r *completionReceipt) error {
	if ctx == nil || t == nil || r == nil || r.used || r.owner != t || t.completionAuthority == nil || t.completionAuthority.owner != t || r.authority != t.completionAuthority || r.evidence == nil || t.handle == nil || r.ledger == nil || r.identity.generation == 0 || r.identity.fdObject == nil || !sameSessionHandle(r.identity.handle, *t.handle) || !r.ledger.matches(r.identity) || t.Ready || t.stopStarted || t.fdCloseAttempted {
		return ErrConflict
	}
	r.used = true
	if e := t.live(ctx); e != nil {
		return e
	}
	if e := r.evidence.validateCompletion(ctx, t); e != nil {
		return e
	}
	// Evidence may have been invalidated while the completion producer returned.
	if t.stopStarted || t.fdCloseAttempted || !r.ledger.matches(r.identity) {
		return ErrConflict
	}
	return t.live(ctx)
}

type runtimeCompletionSource interface {
	collectRuntimeCompletion(context.Context, *postPeerRouteResult) (completionEvidence, error)
}

func (a *operation1Adapter) complete(ctx context.Context, t *Transaction) (*completionReceipt, error) {
	if !a.authorized(ctx) || a.controller.tx != t || a.session == nil || a.session.forwardStep != 6 || a.routed == nil || !a.routed.current() || !a.routed.consumed || !a.setupAdmissionCurrent(6) {
		return nil, afPhase
	}
	source, ok := a.source.(runtimeCompletionSource)
	if !ok {
		return nil, ownedAFPolicyIncomplete
	}
	evidence, e := source.collectRuntimeCompletion(ctx, a.routed)
	if e != nil {
		return nil, e
	}
	if !a.routed.current() || !a.authorized(ctx) || t.stopStarted || t.fdCloseAttempted {
		return nil, afPhase
	}
	return &completionReceipt{owner: t, authority: t.completionAuthority, evidence: evidence, ledger: a.routed.ledger, identity: a.routed.identity}, nil
}
