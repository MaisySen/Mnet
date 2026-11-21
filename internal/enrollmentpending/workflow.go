package enrollmentpending

// Journal-owned workflow-v1 channel/intent, not a key-use permit or State writer.
import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"time"
	"unicode/utf8"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
)

type workflowData struct {
	ReserveID      string `json:"reserve_id"`
	StartedMS      int64  `json:"started_ms"`
	TimeoutMS      int64  `json:"timeout_ms"`
	MaxChallengeMS int64  `json:"max_challenge_ms"`
	MaxReserveMS   int64  `json:"max_reserve_ms"`
	MaxAgeMS       int64  `json:"max_age_ms"`
	MaxLeaseMS     int64  `json:"max_lease_ms"`
	UncertaintyMS  int64  `json:"uncertainty_ms"`
	ChallengeJSON  string `json:"challenge_json"`
	ResponseDigest string `json:"response_digest"`
	ReceiptJSON    string `json:"receipt_json"`
}

func (workflowData) String() string   { return "[private journal workflow DATA]" }
func (workflowData) GoString() string { return "[private journal workflow DATA]" }
func (w workflowData) limits() registrar.WorkflowLimits {
	return registrar.WorkflowLimits{Timeout: time.Duration(w.TimeoutMS) * time.Millisecond, MaxChallengeLifetime: time.Duration(w.MaxChallengeMS) * time.Millisecond, MaxReserveLifetime: time.Duration(w.MaxReserveMS) * time.Millisecond, Timing: contract.FixtureTiming{MaxObservationAgeMS: w.MaxAgeMS, MaxLeaseMS: w.MaxLeaseMS, ClockUncertaintyMS: w.UncertaintyMS}}
}
func decodeWorkflow(r record) (workflowData, bool) {
	var w workflowData
	raw := []byte(r.WorkflowJSON)
	if len(raw) == 0 || len(raw) > 32768 || !utf8.Valid(raw) {
		return w, false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !shape(d, reflect.TypeOf(w), 0) {
		return w, false
	}
	if _, err := d.Token(); err != io.EOF {
		return w, false
	}
	return w, json.Unmarshal(raw, &w) == nil
}
func validWorkflowRecord(r record, p *config.LocalPolicyV2) bool {
	w, ok := decodeWorkflow(r)
	if w.TimeoutMS < 1 || w.TimeoutMS > 60000 || w.MaxChallengeMS < 1 || w.MaxChallengeMS > 60000 || w.MaxReserveMS < 1 || w.MaxReserveMS > 120000 || w.MaxAgeMS < 1 || w.MaxAgeMS > 60000 || w.MaxLeaseMS < 1 || w.MaxLeaseMS > 900000 || w.UncertaintyMS < 0 {
		return false
	}
	if !ok || !token(r.Code) || !token(w.ReserveID) || w.StartedMS < r.CreatedMS || w.StartedMS >= r.ExpiresMS || !registrar.ValidWorkflowLimits(w.limits()) {
		return false
	}
	own, err := contract.DecodeSourceExchange([]byte(r.ExchangeJSON), r.Launch, r.Authority, r.Allowed, r.Allowed, time.UnixMilli(r.CreatedMS))
	if err != nil || !policyAllowsExchange(r, p, own) {
		return false
	}
	if r.Phase == "workflow-begin-may-have-started" {
		return w.ChallengeJSON == "" && w.ResponseDigest == "" && w.ReceiptJSON == ""
	}
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	if err != nil || !bindingMatchesOwn(r, c.Binding) || c.Binding.ReserveID != w.ReserveID {
		return false
	}
	// Historical validity uses original observed checkedAt, not a fresh lease or
	// current eligibility. Lifetime cannot escape the saved journal boundary.
	if c.Binding.AccessUntilMS > r.ExpiresMS || c.ExpiresAtMS <= w.StartedMS || c.Binding.CheckedAtMS < w.StartedMS-w.MaxAgeMS || c.Binding.CheckedAtMS > w.StartedMS+w.TimeoutMS || registrar.CheckWorkflowChallenge(c, own, r.Policy.ServiceID, w.ReserveID, w.limits(), time.UnixMilli(c.Binding.CheckedAtMS)) != nil {
		return false
	}
	switch r.Phase {
	case "workflow-challenge-data":
		return w.ResponseDigest == "" && w.ReceiptJSON == ""
	case "workflow-submit-may-have-started":
		return token(w.ResponseDigest) && w.ReceiptJSON == ""
	case "workflow-receipt-data":
		if !token(w.ResponseDigest) {
			return false
		}
		_, err := contract.DecodeWorkflowReceiptWire([]byte(w.ReceiptJSON), c)
		return err == nil
	}
	return false
}
func workflowEncode(w workflowData) string { raw, _ := json.Marshal(w); return string(raw) }

// BeginWorkflowDATA creates SAME durable record4 marker before sending the saved
// private ticket/launch/mapping. It independently checks known own-PKCE fields,
// pins registrar-selected bounded challenge durably, then returns DATA only.
func (j *Journal) BeginWorkflowDATA(ctx context.Context, current func() SourceConfig, l registrar.WorkflowLimits) (contract.Challenge, error) {
	var empty contract.Challenge
	if ctx == nil || ctx.Err() != nil || current == nil || !registrar.ValidWorkflowLimits(l) {
		return empty, ErrInvalid
	}
	cfg := copyConfig(current())
	now := time.Now()
	j.mu.Lock()
	if err := j.health(); err != nil {
		j.mu.Unlock()
		return empty, err
	}
	if j.data == nil || j.data.Version != 2 || j.data.Phase != "exchanged-data" || j.reopened || j.cancel != nil || j.workflow != nil || !live(*j.data, now) || !bootstrapCurrent(*j.data, cfg, now) {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	own, err := contract.DecodeSourceExchange([]byte(j.data.ExchangeJSON), j.data.Launch, j.data.Authority, j.data.Allowed, cfg.Allowed, now)
	if err != nil {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	reserve, err := opaque()
	if err != nil {
		j.mu.Unlock()
		return empty, err
	}
	client, err := registrar.NewWorkflow(own, j.data.Policy.ServiceID, reserve, j.data.Credential, l)
	if err != nil {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	w := workflowData{ReserveID: reserve, StartedMS: now.UnixMilli(), TimeoutMS: l.Timeout.Milliseconds(), MaxChallengeMS: l.MaxChallengeLifetime.Milliseconds(), MaxReserveMS: l.MaxReserveLifetime.Milliseconds(), MaxAgeMS: l.Timing.MaxObservationAgeMS, MaxLeaseMS: l.Timing.MaxLeaseMS, UncertaintyMS: l.Timing.ClockUncertaintyMS}
	r := *j.data
	r.Version = 4
	r.Phase = "workflow-begin-may-have-started"
	r.WorkflowJSON = workflowEncode(w)
	if err := j.commit(r); err != nil {
		client.Close()
		j.mu.Unlock()
		return empty, err
	}
	child, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	j.workflow = client
	j.mu.Unlock()
	challenge, sendErr := client.Begin(child)
	afterConfig := copyConfig(current())
	after, backward := workflowBoundedNow(now)
	j.mu.Lock()
	defer j.mu.Unlock()
	cancel()
	j.cancel = nil
	if err := j.health(); err != nil {
		client.Close()
		return empty, errors.Join(ErrRemoteUnknown, err)
	}
	if sendErr != nil || backward || ctx.Err() != nil || !live(r, after) || !bootstrapCurrent(r, afterConfig, after) || j.data == nil || j.data.Phase != r.Phase || registrar.CheckWorkflowChallenge(challenge, own, r.Policy.ServiceID, reserve, l, after) != nil {
		client.Close()
		return empty, ErrRemoteUnknown
	}
	raw, _ := json.Marshal(contract.ChallengeWire{Version: 4, Challenge: challenge})
	w.ChallengeJSON = string(raw)
	r.Phase = "workflow-challenge-data"
	r.WorkflowJSON = workflowEncode(w)
	if err := j.commit(r); err != nil {
		client.Close()
		return empty, errors.Join(ErrRemoteUnknown, err)
	}
	j.mu.Unlock()
	finalConfig := copyConfig(current())
	final, finalBackward := workflowBoundedNow(now)
	j.mu.Lock()
	if err := j.health(); err != nil {
		client.Close()
		return empty, errors.Join(ErrNoLongerCurrent, err)
	}
	if finalBackward || !live(r, final) || !bootstrapCurrent(r, finalConfig, final) || registrar.CheckWorkflowChallenge(challenge, own, r.Policy.ServiceID, reserve, l, final) != nil {
		client.Close()
		return empty, ErrNoLongerCurrent
	}
	return challenge, nil
}

// SubmitWorkflowDATA takes only SAME challenge ID + caller MAC. It invokes NO
// private key. Submit intent is durable before send, receipt is durable before
// return. Receipt/confirmation still need actual owned native finalization; this
// method never installs credentials or treats crypto confirmation as a commit.
func (j *Journal) SubmitWorkflowDATA(ctx context.Context, id string, response [32]byte, current func() SourceConfig) (contract.WorkflowReceiptWire, error) {
	var empty contract.WorkflowReceiptWire
	if ctx == nil || ctx.Err() != nil || current == nil {
		return empty, ErrInvalid
	}
	cfg := copyConfig(current())
	now := time.Now()
	j.mu.Lock()
	if err := j.health(); err != nil {
		j.mu.Unlock()
		return empty, err
	}
	if j.data == nil || j.data.Version != 4 || j.data.Phase != "workflow-challenge-data" || j.reopened || j.cancel != nil || j.workflow == nil || !live(*j.data, now) || !bootstrapCurrent(*j.data, cfg, now) {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	r := *j.data
	w, ok := decodeWorkflow(r)
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	own, ownErr := contract.DecodeSourceExchange([]byte(r.ExchangeJSON), r.Launch, r.Authority, r.Allowed, cfg.Allowed, now)
	if !ok || err != nil || ownErr != nil || c.ID != id || registrar.CheckWorkflowChallenge(c, own, r.Policy.ServiceID, w.ReserveID, w.limits(), now) != nil {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	// Strict MAC grammar before persisting a spent submission marker.
	payload := contract.WorkflowSubmitWire{Version: 1, AttemptID: r.Launch.AttemptID, ChallengeID: id, ResponseMAC: base64.RawURLEncoding.EncodeToString(response[:])}
	raw, _ := json.Marshal(payload)
	if _, err := contract.DecodeWorkflowSubmitWire(raw); err != nil {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	w.ResponseDigest = registrar.WorkflowResponseDigest(response)
	r.Phase = "workflow-submit-may-have-started"
	r.WorkflowJSON = workflowEncode(w)
	if err := j.commit(r); err != nil {
		j.mu.Unlock()
		return empty, err
	}
	client := j.workflow
	child, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	j.mu.Unlock()
	result, sendErr := client.SubmitWorkflow(child, id, response)
	afterConfig := copyConfig(current())
	after, backward := workflowBoundedNow(now)
	j.mu.Lock()
	defer j.mu.Unlock()
	defer client.Close()
	cancel()
	j.cancel = nil
	if err := j.health(); err != nil {
		return empty, errors.Join(ErrRemoteUnknown, err)
	}
	if sendErr != nil || backward || ctx.Err() != nil || !live(r, after) || !bootstrapCurrent(r, afterConfig, after) || j.data == nil || j.data.Phase != r.Phase {
		return empty, ErrRemoteUnknown
	}
	raw, _ = json.Marshal(result)
	if _, err := contract.DecodeWorkflowReceiptWire(raw, c); err != nil {
		return empty, ErrRemoteUnknown
	}
	w.ReceiptJSON = string(raw)
	r.Phase = "workflow-receipt-data"
	r.WorkflowJSON = workflowEncode(w)
	if err := j.commit(r); err != nil {
		return empty, errors.Join(ErrRemoteUnknown, err)
	}
	j.mu.Unlock()
	finalConfig := copyConfig(current())
	final, finalBackward := workflowBoundedNow(now)
	j.mu.Lock()
	if err := j.health(); err != nil {
		return empty, errors.Join(ErrNoLongerCurrent, err)
	}
	if finalBackward || !live(r, final) || !bootstrapCurrent(r, finalConfig, final) {
		return empty, ErrNoLongerCurrent
	}
	return result, nil
}

// Cross-host clock trust is explicit policy; local elapsed time additionally
// prevents a paused request from gaining time when the wall clock regresses.
func workflowBoundedNow(start time.Time) (time.Time, bool) {
	now := time.Now()
	backward := now.UnixMilli() < start.UnixMilli()
	elapsed := start.Add(time.Since(start))
	if now.UnixMilli() < elapsed.UnixMilli() {
		now = elapsed
	}
	return now, backward
}
