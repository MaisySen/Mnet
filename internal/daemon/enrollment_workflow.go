package daemon

import (
	"context"
	"encoding/base64"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	proof "fintex.au/mnet/internal/enrollmentproof"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"time"
)

// proveNativeEnrollment is private/inactive. No production caller or trigger.
// Unlike Challenge DATA, SAME journal runtime admission plus independent native
// owner checks gates use of the already-durable key. No key/secret getter or
// caller-supplied challenge/binding is accepted. This does not finalize State.
func (d *Daemon) proveNativeEnrollment(ctx context.Context, l registrar.WorkflowLimits) (contract.WorkflowReceiptWire, error) {
	var empty contract.WorkflowReceiptWire
	if ctx == nil || ctx.Err() != nil {
		return empty, errEnrollmentContextStale
	}
	d.mu.Lock()
	p := d.enrollmentPreparation
	if p == nil || d.checkEnrollmentContextLocked(p.context) != nil || !d.enrollmentIdentityUnclaimedLocked() {
		d.mu.Unlock()
		return empty, errEnrollmentContextStale
	}
	d.mu.Unlock()
	current := func() pending.SourceConfig {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.enrollmentPreparation != p || d.checkEnrollmentContextLocked(p.context) != nil {
			return pending.SourceConfig{}
		}
		cfg, ok := pending.SourceConfigFor(p.context.policy)
		if !ok {
			return pending.SourceConfig{}
		}
		return cfg
	}
	if _, err := p.journal.BeginWorkflowDATA(ctx, current, l); err != nil {
		return empty, err
	}
	result, err := p.journal.ProveWorkflowOwned(ctx, current, func(a *pending.WorkflowKeyAdmission) (*proof.Native, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		if ctx.Err() != nil || d.enrollmentPreparation != p || d.checkEnrollmentContextLocked(p.context) != nil {
			return nil, errEnrollmentContextStale
		}
		s := d.stateOwner.saved
		if !d.enrollmentIdentityUnclaimedLocked() {
			return nil, errEnrollmentContextStale
		}
		return a.RespondProof(func(c contract.Challenge, expected contract.Binding) (*proof.Native, error) {
			// SAME retained native + journal locks held throughout local key derivation.
			// Expected identity is reconstructed from journal-owned own-PKCE, not echo.
			key, err := base64.StdEncoding.Strict().DecodeString(s.PrivateKey)
			if err != nil || len(key) != 32 {
				return nil, errEnrollmentContextStale
			}
			defer clear(key)
			var private [32]byte
			copy(private[:], key)
			defer clear(private[:])
			n, err := proof.Respond(private, c, expected, time.Now(), l.MaxChallengeLifetime)
			if err != nil {
				return nil, err
			}
			return n, nil
		})
	})
	if err != nil {
		return empty, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if ctx.Err() != nil || d.enrollmentPreparation != p || d.checkEnrollmentContextLocked(p.context) != nil {
		return empty, errEnrollmentContextStale
	}
	// SAME Journal owns and has cryptographically confirmed the outstanding engine.
	p.confirmed = &result
	// This is exact authenticated receipt DATA + confirmed transcript only.
	// Do not clear pending, mark enrollment complete, install State or start I/O.
	return result, nil
}

// Requires healthy native owner and d.mu. No legacy authority or routing flags
// may silently pass into the independent enrollment capability.
func (d *Daemon) enrollmentIdentityUnclaimedLocked() bool {
	s := d.stateOwner.saved
	return s.native == nil && s.NodeID == "" && s.NodeToken == "" && s.AuthKey == "" && !s.ExitNode && len(s.AdvertiseRoutes) == 0 && s.UseExitNode == ""
}
