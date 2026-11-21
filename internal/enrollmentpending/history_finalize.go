package enrollmentpending

// Authenticated history delivery is NOT a reconstructed proof confirmation.
// This separate provenance path permits local publication only within the
// ORIGINAL receipt access interval and unchanged native ownership/policy.
import (
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"time"
)

type historyFinalizationReady struct {
	image   *record
	receipt contract.HistoryReceiptWire
	start   time.Time
}

func historyAccessLive(r record, receipt contract.HistoryReceiptWire, start, now time.Time) bool {
	elapsed := start.Add(time.Since(start))
	return live(r, now) && now.UnixMilli() >= start.UnixMilli() && now.UnixMilli() >= receipt.Receipt.CommittedAtMS && now.UnixMilli() < receipt.Receipt.AccessExpiresAtMS && elapsed.UnixMilli() < receipt.Receipt.AccessExpiresAtMS
}
func validHistoryFinalizationRecord(r record) bool {
	if r.Version != 7 || (r.Phase != "native-finalization-prepared" && r.Phase != "native-finalization-completed") {
		return false
	}
	old := r
	old.Version = 6
	old.Phase = "history-receipt-data"
	old.FinalizationJSON = ""
	if !validRecord(old) {
		return false
	}
	_, _, err := finalizationDocument(r)
	return err == nil
}

// FinalizeHistory accepts no receipt/confirmation DATA. Only a successful SAME
// Journal-owned authenticated delivery can set private readiness. Reopen alone,
// exact file bytes, parser results or public DATA methods never set it.
func (j *Journal) FinalizeHistory(originalRevision, policyRevision string, current SourceConfig, publish func(contract.NativeCredentialDocument) error) error {
	if j == nil || j.journal == nil || publish == nil || !token(originalRevision) || !token(policyRevision) {
		return ErrInvalid
	}
	cfg := copyConfig(current)
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return err
	}
	ready := j.historyReady
	if ready == nil || j.data == nil || ready.image != j.data || j.cancel != nil || j.historyClient != nil || !((j.data.Version == 6 && j.data.Phase == "history-receipt-data") || (j.data.Version == 7 && j.data.Phase == "native-finalization-prepared")) || !historyPolicyCurrent(*j.data, cfg, j.data.Launch.DevicePublicKey) || !historyAccessLive(*j.data, ready.receipt, ready.start, time.Now()) || j.checkExactHistoryFileLocked() != nil {
		return ErrInvalid
	}
	r := *j.data
	h, ok := decodeHistory(r)
	if !ok {
		return ErrInvalid
	}
	w, ok := decodeWorkflow(r)
	if !ok {
		return ErrInvalid
	}
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	if err != nil {
		return ErrInvalid
	}
	pinned, err := contract.DecodeHistoryReceiptWire([]byte(h.ReceiptJSON), c)
	if err != nil || pinned != ready.receipt {
		return ErrInvalid
	}
	if r.Version == 6 {
		id, err := opaque()
		if err != nil {
			return err
		}
		f := finalizationData{ID: id, OriginalStateRevision: originalRevision, PolicyRevision: policyRevision, ReceiptDigest: pinned.ReceiptDigest}
		raw, _ := json.Marshal(f)
		r.FinalizationJSON = string(raw)
		r.Version = 7
		r.Phase = "native-finalization-prepared"
		if err := j.commit(r); err != nil {
			return err
		}
		ready.image = j.data
	} else {
		f, ok := decodeFinalization(r)
		if !ok || f.OriginalStateRevision != originalRevision || f.PolicyRevision != policyRevision || f.ReceiptDigest != pinned.ReceiptDigest {
			return ErrInvalid
		}
	}
	if !historyAccessLive(r, ready.receipt, ready.start, time.Now()) || j.checkExactHistoryFileLocked() != nil {
		return ErrNoLongerCurrent
	}
	doc, _, err := finalizationDocument(r)
	if err != nil {
		return err
	}
	if err := publish(doc); err != nil {
		return err
	}
	// Native callback rechecks original policy/context/expiry pre/post State sync.
	// A post-publication timeout cannot erase State or pretend commit absence.
	if !historyAccessLive(r, ready.receipt, ready.start, time.Now()) {
		return ErrNoLongerCurrent
	}
	r.Phase = "native-finalization-completed"
	if err := j.commit(r); err != nil {
		return err
	}
	j.historyReady = nil
	return nil
}
