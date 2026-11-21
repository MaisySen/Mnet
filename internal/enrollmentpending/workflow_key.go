package enrollmentpending

import (
	"context"
	"errors"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"time"
)

// WorkflowKeyAdmission is runtime-only SAME Journal channel admission. It cannot
// be reconstructed from Challenge DATA, a receipt, an ID or a reopened file.
// Its callback must independently hold/check the actual native key/policy owner.
// No private key or usable credential enters/leaves this type.
type WorkflowKeyAdmission struct{ *workflowKeyAdmission }
type workflowKeyAdmission struct {
	owner  *Journal
	image  *record
	ctx    context.Context
	config SourceConfig
	start  time.Time
	used   bool
}

func (WorkflowKeyAdmission) String() string   { return "[private journal-owned key admission]" }
func (WorkflowKeyAdmission) GoString() string { return "[private journal-owned key admission]" }

// Respond executes at most once under the SAME journal lock. Native caller must
// first acquire its native owner lock; lock order is native -> journal. No remote
// I/O or current() callback occurs under either lock. Close cannot interleave
// with key derivation. The supplied function must be finite local computation.
func (a *WorkflowKeyAdmission) Respond(use func(contract.Challenge, contract.Binding) ([32]byte, error)) ([32]byte, error) {
	var empty [32]byte
	if a == nil || a.workflowKeyAdmission == nil || a.owner == nil || a.owner.journal == nil || use == nil {
		return empty, ErrInvalid
	}
	j := a.owner
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return empty, err
	}
	if a.used || a.ctx == nil || a.ctx.Err() != nil || j.reopened || j.data != a.image || j.workflow == nil || j.cancel == nil || j.data.Phase != "workflow-key-use-may-have-started" {
		return empty, ErrInvalid
	}
	// Spend even a callback failure. Durable marker already precedes this point.
	a.used = true
	now, backward := workflowBoundedNow(a.start)
	if backward || !live(*j.data, now) || !bootstrapCurrent(*j.data, a.config, now) {
		return empty, ErrInvalid
	}
	w, ok := decodeWorkflow(*j.data)
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	own, ownErr := contract.DecodeSourceExchange([]byte(j.data.ExchangeJSON), j.data.Launch, j.data.Authority, j.data.Allowed, a.config.Allowed, now)
	if !ok || err != nil || ownErr != nil || registrar.CheckWorkflowChallenge(c, own, j.data.Policy.ServiceID, w.ReserveID, w.limits(), now) != nil {
		return empty, ErrInvalid
	}
	expected, err := registrar.OwnWorkflowBindingDATA(own, j.data.Policy.ServiceID, w.ReserveID, c.Binding.CheckedAtMS, c.Binding.ReserveDeadlineMS, c.Binding.AccessUntilMS)
	if err != nil {
		return empty, ErrInvalid
	}
	return use(c, expected)
}

// ProveWorkflowDATA admits only a live journal-owned verified Begin result. It
// durably marks one-shot key use BEFORE offering the opaque runtime admission.
// It never supplies raw Challenge DATA as permission. Native code checks its
// owner then consumes admission under native -> journal locks. Any post-marker
// failure preserves the spent phase and never calls the key again automatically.
// A receipt remains DATA until the native owner confirms its outstanding MAC
// and separately finalizes the sole native State. No State writer lives here.
func (j *Journal) ProveWorkflowDATA(ctx context.Context, current func() SourceConfig, respond func(*WorkflowKeyAdmission) ([32]byte, error)) (contract.WorkflowReceiptWire, error) {
	var empty contract.WorkflowReceiptWire
	if j == nil || j.journal == nil || ctx == nil || ctx.Err() != nil || current == nil || respond == nil {
		return empty, ErrInvalid
	}
	cfg := copyConfig(current())
	start := time.Now()
	j.mu.Lock()
	if err := j.health(); err != nil {
		j.mu.Unlock()
		return empty, err
	}
	if j.data == nil || j.data.Version != 4 || j.data.Phase != "workflow-challenge-data" || j.reopened || j.workflow == nil || j.cancel != nil || !live(*j.data, start) || !bootstrapCurrent(*j.data, cfg, start) {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	r := *j.data
	w, ok := decodeWorkflow(r)
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	own, ownErr := contract.DecodeSourceExchange([]byte(r.ExchangeJSON), r.Launch, r.Authority, r.Allowed, cfg.Allowed, start)
	if !ok || err != nil || ownErr != nil || registrar.CheckWorkflowChallenge(c, own, r.Policy.ServiceID, w.ReserveID, w.limits(), start) != nil {
		j.mu.Unlock()
		return empty, ErrInvalid
	}
	r.Phase = "workflow-key-use-may-have-started"
	if err := j.commit(r); err != nil {
		j.mu.Unlock()
		return empty, err
	}
	child, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	a := &WorkflowKeyAdmission{&workflowKeyAdmission{owner: j, image: j.data, ctx: child, config: cfg, start: start}}
	j.mu.Unlock()
	// Revalidate after the durable marker as publication can take time or
	// observe policy removal. This callback never runs under the journal lock.
	readyConfig := copyConfig(current())
	readyNow, readyBackward := workflowBoundedNow(start)
	j.mu.Lock()
	readyHealth := j.health()
	if readyHealth != nil || j.data != a.image || child.Err() != nil || readyBackward || !live(r, readyNow) || !bootstrapCurrent(r, readyConfig, readyNow) {
		a.used = true
		cancel()
		j.cancel = nil
		j.mu.Unlock()
		return empty, errors.Join(ErrNoLongerCurrent, readyHealth)
	}
	j.mu.Unlock()
	// Actual native owner checks repeat inside responder before Respond.
	response, useErr := respond(a)
	defer clear(response[:])
	afterConfig := copyConfig(current())
	after, backward := workflowBoundedNow(start)
	j.mu.Lock()
	used := a.used
	a.used = true
	cancel()
	j.cancel = nil
	healthErr := j.health()
	bad := healthErr != nil || j.data != a.image || useErr != nil || !used || ctx.Err() != nil || backward || !live(r, after) || !bootstrapCurrent(r, afterConfig, after)
	j.mu.Unlock()
	if bad {
		return empty, errors.Join(ErrNoLongerCurrent, useErr, healthErr)
	}
	return j.submitWorkflowDATA(ctx, c.ID, response, current, "workflow-key-use-may-have-started")
}
