package enrollmentpending

import (
	"context"
	"encoding/base64"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
	"time"
)

type workflowFinalizationReady struct {
	image   *record
	receipt contract.WorkflowReceiptWire
}

// RespondProof is the crypto-owner form of SAME opaque key admission. Native
// callback must hold/check its real retained owner before consuming. The proof
// engine enforces exact independently reconstructed binding and key equality.
// No parser/equality/verification boolean can construct an engine confirmation.
func (a *WorkflowKeyAdmission) RespondProof(use func(contract.Challenge, contract.Binding) (*proof.Native, error)) (*proof.Native, error) {
	if use == nil {
		return nil, ErrInvalid
	}
	var n *proof.Native
	_, err := a.Respond(func(c contract.Challenge, b contract.Binding) ([32]byte, error) {
		var err error
		n, err = use(c, b)
		if err != nil || n == nil {
			return [32]byte{}, ErrInvalid
		}
		return n.Response(time.Now())
	})
	if err != nil {
		if n != nil {
			n.Close()
		}
		return nil, err
	}
	return n, nil
}

// ProveWorkflowOwned is separate from DATA-only ProveWorkflowDATA. It owns the
// actual outstanding proof engine returned by admitted native code, checks the
// authenticated receipt confirmation itself, then retains private SAME-runtime
// finalization readiness. DATA-only submit, equality checks, arbitrary receipt
// parsing or a caller saying verification succeeded cannot create readiness.
// Ownership of the returned engine transfers here, and it is always closed.
func (j *Journal) ProveWorkflowOwned(ctx context.Context, current func() SourceConfig, respond func(*WorkflowKeyAdmission) (*proof.Native, error)) (contract.WorkflowReceiptWire, error) {
	var empty contract.WorkflowReceiptWire
	if respond == nil {
		return empty, ErrInvalid
	}
	var native *proof.Native
	defer func() {
		if native != nil {
			native.Close()
		}
	}()
	result, err := j.ProveWorkflowDATA(ctx, current, func(a *WorkflowKeyAdmission) ([32]byte, error) {
		var err error
		native, err = respond(a)
		if err != nil || native == nil {
			return [32]byte{}, ErrInvalid
		}
		return native.Response(time.Now())
	})
	if err != nil {
		return empty, err
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(result.ConfirmationMAC)
	if err != nil || len(raw) != 32 {
		return empty, ErrInvalid
	}
	var confirmation [32]byte
	copy(confirmation[:], raw)
	defer clear(confirmation[:])
	if err := native.Confirm(confirmation, time.Now()); err != nil {
		return empty, err
	}
	cfg := copyConfig(current())
	now := time.Now()
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return empty, err
	}
	if ctx.Err() != nil || j.reopened || j.data == nil || j.data.Version != 4 || j.data.Phase != "workflow-receipt-data" || !live(*j.data, now) || !bootstrapCurrent(*j.data, cfg, now) {
		return empty, ErrNoLongerCurrent
	}
	w, ok := decodeWorkflow(*j.data)
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	if !ok || err != nil {
		return empty, ErrInvalid
	}
	pinned, err := contract.DecodeWorkflowReceiptWire([]byte(w.ReceiptJSON), c)
	if err != nil || pinned != result {
		return empty, ErrInvalid
	}
	j.finalizationReady = &workflowFinalizationReady{image: j.data, receipt: result}
	return result, nil
}
