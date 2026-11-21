package enrollmentpending

// Distinct bounded historical lookup protocol, never native authority. Receipt
// pinning cannot create finalizationReady, confirmation, current access or State.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"fintex.au/mnet/internal/stateowner"
	"io"
	"os"
	"reflect"
	"time"
)

type historyData struct {
	OriginalVersion int    `json:"original_version"`
	OriginalPhase   string `json:"original_phase"`
	MaxAttempts     int    `json:"max_attempts"`
	Attempts        int    `json:"attempts"`
	TimeoutMS       int64  `json:"timeout_ms"`
	StartedMS       int64  `json:"started_ms"`
	ReceiptJSON     string `json:"receipt_json"`
}

func decodeHistory(r record) (historyData, bool) {
	var h historyData
	d := json.NewDecoder(bytes.NewReader([]byte(r.HistoryJSON)))
	d.UseNumber()
	if !shape(d, reflect.TypeOf(h), 0) {
		return h, false
	}
	if _, err := d.Token(); err != io.EOF {
		return h, false
	}
	if json.Unmarshal([]byte(r.HistoryJSON), &h) != nil {
		return h, false
	}
	return h, h.MaxAttempts >= 1 && h.MaxAttempts <= 4 && h.Attempts >= 1 && h.Attempts <= h.MaxAttempts && h.TimeoutMS >= 1 && h.TimeoutMS <= 60000 && h.StartedMS >= r.CreatedMS
}
func historyOriginal(r record, h historyData) record {
	r.Version = h.OriginalVersion
	r.Phase = h.OriginalPhase
	r.HistoryJSON = ""
	return r
}
func historyEligible(r record) bool {
	return r.Version == 4 && (r.Phase == "workflow-submit-may-have-started" || r.Phase == "workflow-receipt-data")
}
func validHistoryRecord(r record) bool {
	if r.Version != 6 || (r.Phase != "history-lookup-may-have-started" && r.Phase != "history-receipt-data") {
		return false
	}
	h, ok := decodeHistory(r)
	if !ok {
		return false
	}
	old := historyOriginal(r, h)
	if !historyEligible(old) || !validRecord(old) {
		return false
	}
	w, ok := decodeWorkflow(old)
	if !ok || !token(w.ResponseDigest) {
		return false
	}
	// A later lookup marker retains the FIRST durable historical receipt. A
	// failed/missing response must not erase that immutable anchor on reopen.
	if r.Phase == "history-lookup-may-have-started" && h.ReceiptJSON == "" {
		return true
	}
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	if err != nil {
		return false
	}
	got, err := contract.DecodeHistoryReceiptWire([]byte(h.ReceiptJSON), c)
	return err == nil && historyMatchesExisting(w, got, c)
}
func historyMatchesExisting(w workflowData, h contract.HistoryReceiptWire, c contract.Challenge) bool {
	if w.ReceiptJSON == "" {
		return true
	}
	known, err := contract.DecodeWorkflowReceiptWire([]byte(w.ReceiptJSON), c)
	return err == nil && known.Receipt == h.Receipt && known.ReceiptDigest == h.ReceiptDigest
}
func historyMatchesPinned(h historyData, result contract.HistoryReceiptWire, c contract.Challenge) bool {
	if h.ReceiptJSON == "" {
		return true
	}
	pinned, err := contract.DecodeHistoryReceiptWire([]byte(h.ReceiptJSON), c)
	return err == nil && pinned == result
}
func historyPolicyCurrent(r record, c SourceConfig, device string) bool {
	expected, ok := c.Policy.Snapshot()
	if !ok || !reflect.DeepEqual(expected, r.Policy) || device != r.Launch.DevicePublicKey {
		return false
	}
	// Decode source at its ORIGINAL historical observation, not now. This is not
	// renewed source eligibility; only exact original identity/context disclosure.
	return bootstrapCurrent(r, c, time.UnixMilli(r.CreatedMS))
}
func (j *Journal) checkExactHistoryFileLocked() error {
	info, err := os.Lstat(j.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 || stateowner.SingleLink(j.path, info) != nil {
		return ErrInvalid
	}
	raw, err := os.ReadFile(j.path)
	expected, _ := json.Marshal(j.data)
	if err != nil || !bytes.Equal(raw, expected) {
		return ErrInvalid
	}
	return nil
}

// RecoverHistoryDATA does not invoke private key, source exchange/reserve/submit,
// State publication or finalization. Caller supplies actual current native-owned
// policy/device. Every explicit lookup spends one durable finite attempt BEFORE
// disclosure. Reopen may retry SAME immutable lookup within ORIGINAL budget.
// No automatic retry. Begin-unknown without full challenge is never admitted.
func (j *Journal) RecoverHistoryDATA(ctx context.Context, device string, current func() SourceConfig, timeout time.Duration, maxAttempts int) (contract.HistoryReceiptWire, error) {
	return j.recoverHistory(ctx, device, current, timeout, maxAttempts, false)
}

// RecoverHistoryOwned additionally requires fresh SAME-client authenticated
// delivery provenance and a still-live original access interval. It never
// constructs proof confirmation. Native owner must separately publish State.
func (j *Journal) RecoverHistoryOwned(ctx context.Context, device string, current func() SourceConfig, timeout time.Duration, maxAttempts int) error {
	_, err := j.recoverHistory(ctx, device, current, timeout, maxAttempts, true)
	return err
}
func (j *Journal) recoverHistory(ctx context.Context, device string, current func() SourceConfig, timeout time.Duration, maxAttempts int, owned bool) (contract.HistoryReceiptWire, error) {
	var empty contract.HistoryReceiptWire
	if j == nil || j.journal == nil || ctx == nil || ctx.Err() != nil || current == nil || timeout < time.Millisecond || timeout > time.Minute || timeout%time.Millisecond != 0 || maxAttempts < 1 || maxAttempts > 4 {
		return empty, ErrInvalid
	}
	cfg := copyConfig(current())
	start := time.Now()
	j.mu.Lock()
	if err := j.health(); err != nil {
		j.mu.Unlock()
		return empty, err
	}
	if j.data == nil || j.cancel != nil || j.historyClient != nil || !historyPolicyCurrent(*j.data, cfg, device) || j.checkExactHistoryFileLocked() != nil {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	r := *j.data
	var h historyData
	if r.Version == 6 {
		var ok bool
		h, ok = decodeHistory(r)
		if !ok || h.MaxAttempts != maxAttempts || h.TimeoutMS != timeout.Milliseconds() || h.Attempts >= h.MaxAttempts || start.UnixMilli() < h.StartedMS {
			j.mu.Unlock()
			return empty, ErrInvalid
		}
	} else {
		if !historyEligible(r) {
			j.mu.Unlock()
			return empty, ErrInvalid
		}
		h = historyData{OriginalVersion: r.Version, OriginalPhase: r.Phase, TimeoutMS: timeout.Milliseconds(), MaxAttempts: maxAttempts}
	}
	w, ok := decodeWorkflow(r)
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	if !ok || err != nil || !token(w.ResponseDigest) || !bindingMatchesOwn(r, c.Binding) {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	client, err := registrar.NewHistory(r.Policy.RegistrarOrigin, c, r.Credential, timeout)
	if err != nil {
		j.mu.Unlock()
		return empty, err
	}
	h.Attempts++
	h.StartedMS = start.UnixMilli()
	raw, _ := json.Marshal(h)
	r.Version = 6
	r.Phase = "history-lookup-may-have-started"
	r.HistoryJSON = string(raw)
	if err := j.commit(r); err != nil {
		client.Close()
		j.mu.Unlock()
		return empty, err
	}
	child, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	j.historyClient = client
	j.finalizationReady = nil
	j.historyReady = nil
	j.mu.Unlock()
	// Marker sync can outlive policy or cancellation. Check again BEFORE the
	// credential leaves the device, without taking native lock under journal.
	readyCfg := copyConfig(current())
	ready, readyBack := workflowBoundedNow(start)
	j.mu.Lock()
	readyHealth := j.health()
	if readyHealth != nil || child.Err() != nil || readyBack || ready.Sub(start) > timeout || !historyPolicyCurrent(r, readyCfg, device) || j.checkExactHistoryFileLocked() != nil {
		cancel()
		client.Close()
		j.cancel = nil
		j.historyClient = nil
		j.mu.Unlock()
		return empty, errors.Join(ErrNoLongerCurrent, readyHealth)
	}
	j.mu.Unlock()
	var result contract.HistoryReceiptWire
	var sendErr error
	if owned {
		var delivery *registrar.HistoryDelivery
		delivery, sendErr = client.ReceiptOwned(child)
		if sendErr == nil {
			result, sendErr = client.ConsumeDelivery(delivery)
		}
	} else {
		result, sendErr = client.Receipt(child)
	}
	cfg = copyConfig(current())
	after, backward := workflowBoundedNow(start)
	j.mu.Lock()
	defer j.mu.Unlock()
	defer client.Close()
	cancel()
	j.cancel = nil
	j.historyClient = nil
	if err := j.health(); err != nil {
		return empty, errors.Join(ErrRemoteUnknown, err)
	}
	if sendErr != nil || ctx.Err() != nil || backward || after.Sub(start) > timeout || !historyPolicyCurrent(r, cfg, device) || j.data == nil || j.data.Phase != r.Phase || j.checkExactHistoryFileLocked() != nil {
		return empty, ErrRemoteUnknown
	}
	raw, _ = json.Marshal(result)
	if _, err := contract.DecodeHistoryReceiptWire(raw, c); err != nil || !historyMatchesExisting(w, result, c) || !historyMatchesPinned(h, result, c) {
		return empty, ErrRemoteUnknown
	}
	// Keep exact original pinned bytes on a successful repeat, not a replacement
	// projection. Only the first successful pin can establish this anchor.
	if h.ReceiptJSON == "" {
		h.ReceiptJSON = string(raw)
	}
	raw, _ = json.Marshal(h)
	r.HistoryJSON = string(raw)
	r.Phase = "history-receipt-data"
	if err := j.commit(r); err != nil {
		return empty, errors.Join(ErrRemoteUnknown, err)
	}
	j.mu.Unlock()
	finalCfg := copyConfig(current())
	final, back := workflowBoundedNow(start)
	j.mu.Lock()
	if err := j.health(); err != nil {
		return empty, errors.Join(ErrNoLongerCurrent, err)
	}
	if ctx.Err() != nil || back || final.Sub(start) > timeout || !historyPolicyCurrent(r, finalCfg, device) || j.checkExactHistoryFileLocked() != nil {
		return empty, ErrNoLongerCurrent
	}
	if owned {
		if !historyAccessLive(r, result, start, final) {
			return empty, ErrNoLongerCurrent
		}
		j.historyReady = &historyFinalizationReady{image: j.data, receipt: result, start: start}
	}
	return result, nil
}

// CheckHistoryStateBoundaryDATA is a safety-only strict FILE validator for the
// native loader's new version6 namespace. It creates no receipt provenance or
// history/client/finalization readiness and exposes no private values.
func CheckHistoryStateBoundaryDATA(raw []byte) bool {
	var r record
	return strictRecord(raw, &r) && r.Version == 6 && validHistoryRecord(r)
}
