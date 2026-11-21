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
	var native *proof.Native
	defer func() {
		if native != nil {
			native.Close()
		}
	}()
	result, err := p.journal.ProveWorkflowDATA(ctx, current, func(a *pending.WorkflowKeyAdmission) ([32]byte, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		if ctx.Err() != nil || d.enrollmentPreparation != p || d.checkEnrollmentContextLocked(p.context) != nil {
			return [32]byte{}, errEnrollmentContextStale
		}
		s := d.stateOwner.saved
		if !d.enrollmentIdentityUnclaimedLocked() {
			return [32]byte{}, errEnrollmentContextStale
		}
		return a.Respond(func(c contract.Challenge, expected contract.Binding) ([32]byte, error) {
			// SAME retained native + journal locks held throughout local key derivation.
			// Expected identity is reconstructed from journal-owned own-PKCE, not echo.
			key, err := base64.StdEncoding.Strict().DecodeString(s.PrivateKey)
			if err != nil || len(key) != 32 {
				return [32]byte{}, errEnrollmentContextStale
			}
			defer clear(key)
			var private [32]byte
			copy(private[:], key)
			defer clear(private[:])
			n, err := proof.Respond(private, c, expected, time.Now(), l.MaxChallengeLifetime)
			if err != nil {
				return [32]byte{}, err
			}
			native = n
			return n.Response(time.Now())
		})
	})
	if err != nil {
		return empty, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if ctx.Err() != nil || native == nil || d.enrollmentPreparation != p || d.checkEnrollmentContextLocked(p.context) != nil {
		return empty, errEnrollmentContextStale
	}
	confirmation, err := base64.RawURLEncoding.Strict().DecodeString(result.ConfirmationMAC)
	if err != nil || len(confirmation) != 32 {
		return empty, errEnrollmentContextStale
	}
	var mac [32]byte
	copy(mac[:], confirmation)
	defer clear(mac[:])
	if err := native.Confirm(mac, time.Now()); err != nil {
		return empty, err
	}
	// This is exact authenticated receipt DATA + confirmed transcript only.
	// Do not clear pending, mark enrollment complete, install State or start I/O.
	return result, nil
}

// Requires healthy native owner and d.mu. No legacy authority or routing flags
// may silently pass into the independent enrollment capability.
func (d *Daemon) enrollmentIdentityUnclaimedLocked() bool {
	s := d.stateOwner.saved
	return s.NodeID == "" && s.NodeToken == "" && s.AuthKey == "" && !s.ExitNode && len(s.AdvertiseRoutes) == 0 && s.UseExitNode == ""
}
