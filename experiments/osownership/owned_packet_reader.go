//go:build mnet_enrollment_development

package osownership

// Raw source output stays diagnostic. No Inventory/Ready promotion method.
type ownedSourceDiagnostic struct {
	raw           RawInventory
	sys           SysctlSnapshot
	framing       tunNestedFraming
	effective     []effectiveV4Word
	lineageUsable bool
	held          *heldSourceRecord
}

func sourceDiagnosticFromRecords(raw RawInventory, sys SysctlSnapshot, h Handle, usable bool) (ownedSourceDiagnostic, error) {
	var zero ownedSourceDiagnostic
	observed, e := parseEffectiveV4(sys)
	if e != nil {
		return zero, e
	}
	want := h.Link
	if !h.TagConfirmed {
		want = h.CreationLink
	}
	var owned *LinkRecord
	for _, l := range raw.Links {
		if l.Index == want.Index || l.Name == want.Name || (want.Alias != "" && l.Alias == want.Alias) {
			if owned != nil || l.Index != want.Index || l.Name != want.Name || l.Alias != want.Alias || l.MTU != want.MTU || l.Hardware != 65534 || l.Flags & ^uint32(1|0x40|0x10000) != 0x1090 {
				return zero, ErrConflict
			}
			copy := l
			owned = &copy
		}
	}
	if owned == nil {
		return zero, ErrConflict
	}
	framing, e := parseTUNNested(*owned)
	if e != nil {
		return zero, e
	}
	values, e := effectiveV4Link(*owned, observed)
	if e != nil {
		return zero, e
	}
	clone := SysctlSnapshot{}
	for k, v := range sys {
		clone[k] = v
	}
	return ownedSourceDiagnostic{raw: cloneRawExact(raw), sys: clone, framing: framing, effective: values, lineageUsable: usable}, nil
}

// A typed diagnostic binding to original held lineage. Original kernel FD/netns
// identity proof is NOT implemented here. This snapshots correlation, not attests.
type heldSourceRecord struct {
	ledger   *heldPacketLedger
	identity heldPacketIdentity
	owner    *Transaction
	session  *adapterSession
}

func heldSourceSnapshot(l *heldPacketLedger, t *Transaction, s *adapterSession) (*heldSourceRecord, error) {
	if l == nil || t == nil || s == nil || s.canonical == nil {
		return nil, ErrConflict
	}
	id := l.snapshot()
	a, ok := t.k.(transactionLedgerAdapter)
	if !ok || a.ownershipSession() != s || id.identity != s.identity || !sameSessionHandle(id.handle, *s.canonical) {
		return nil, ErrConflict
	}
	return &heldSourceRecord{ledger: l, identity: id, owner: t, session: s}, nil
}
func diagnosticForHeldRecords(raw RawInventory, sys SysctlSnapshot, held *heldSourceRecord) (ownedSourceDiagnostic, error) {
	if held == nil || held.ledger == nil || held.session == nil || held.session.canonical == nil || held.identity.fdObject == nil || held.session.identity != held.identity.identity {
		return ownedSourceDiagnostic{}, ErrConflict
	}
	now := held.ledger.snapshot()
	if now != held.identity {
		return ownedSourceDiagnostic{}, ErrConflict
	}
	d, e := sourceDiagnosticFromRecords(raw, sys, *held.session.canonical, held.ledger.matches(held.identity))
	if e != nil {
		return d, e
	}
	d.held = held
	return d, nil
}
