//go:build linux && mnet_enrollment_development

package osownership

// Retained original resource references for future source adapter integration.
// No constructor accepts an fd number/path or manufactures one by lookup.
// This does NOT inspect kernel state and is NOT a live supplier implementation.
// It intentionally cannot implement inertHeldPacketWire's fake-only marker.
type originalHeldPacketResources struct {
	creation  *ownedCreation
	namespace *namespaceCustody
	ledger    *heldPacketLedger
	owner     *Transaction
	session   *adapterSession
}

func retainOriginalPacketResources(c *ownedCreation, ns *namespaceCustody, t *Transaction, s *adapterSession, v heldPacketIdentity) (*originalHeldPacketResources, error) {
	if c == nil || c.fd == nil || c.fd.file == nil || ns == nil || t == nil || s == nil || s.canonical == nil || s.plan == nil || s.forwardStep != 1 || s.closeAttempted || s.cleanupMode || s.poisoned || t.stopStarted || t.fdCloseAttempted || c.handle != *s.canonical || v.handle != c.handle || v.identity != s.identity || !sameNamespace(ns.identity, v.target) || !sameNamespace(c.handle.Namespace, ns.identity) {
		return nil, ErrConflict
	}
	a, ok := t.k.(transactionLedgerAdapter)
	if !ok || a.ownershipSession() != s {
		return nil, ErrConflict
	}
	// Mint correlation token from retained original ownership here, never accept
	// a caller's claimed token as binding it to this particular *fileCustody.
	v.fdObject = &packetOriginalObject{serial: 1}
	l, e := newHeldPacketLedger(v)
	if e != nil {
		return nil, e
	}
	if e = c.fd.attachLineage(l); e != nil {
		return nil, e
	}
	return &originalHeldPacketResources{creation: c, namespace: ns, ledger: l, owner: t, session: s}, nil
}

// Source adapter must invalidate BEFORE any control/dup/pass/close attempt and
// before delegating it to the effect helper. It cannot regain lineage by matching
// name/ifindex later. No effect helper call is made by this method.
func (o *originalHeldPacketResources) invalidateBefore(kind string) error {
	if o == nil || o.ledger == nil {
		return ErrConflict
	}
	return o.ledger.invalidate(kind)
}
