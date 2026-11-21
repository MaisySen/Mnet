package daemon

import (
	"context"
	"encoding/base64"
	pending "fintex.au/mnet/internal/enrollmentpending"
	"path/filepath"
	"time"
)

// Narrow private owner. Not a Daemon, public startup/Prepare/key-generation or
// State mutator. Only exact preexisting original State+policy+prepared record5
// (or its strict record8 retry wrapper) is admitted. Ordinary New/LoadState gates
// are untouched. This object never replaces keys or constructs missing State.
type nativeFinalizationRecovery struct{ daemon *Daemon }

func (nativeFinalizationRecovery) String() string {
	return "[private same-attempt native recovery owner]"
}
func (nativeFinalizationRecovery) GoString() string {
	return "[private same-attempt native recovery owner]"
}
func openNativeFinalizationRecovery(dir string) (*nativeFinalizationRecovery, error) {
	owner, err := openStateOwnerGuarded(dir, func(o *nativeStateOwner) error {
		d := newStoppedDaemon(Config{StateDir: dir}, o)
		if !o.saved.revisionExists || !d.enrollmentIdentityUnclaimedLocked() {
			return errEnrollmentContextStale
		}
		ctx, err := d.enrollmentContextLocked()
		if err != nil {
			return err
		}
		cfg, ok := pending.SourceConfigFor(ctx.policy)
		if !ok {
			return errEnrollmentContextStale
		}
		key, err := base64.StdEncoding.Strict().DecodeString(ctx.publicKey)
		if err != nil || len(key) != 32 {
			return errEnrollmentContextStale
		}
		raw, err := readOwnedEnrollmentFile(filepath.Join(o.lease.Parent(), "enrollment-pending", "enrollment-pending-v1.json"), 65536)
		if err != nil || !pending.CheckPreparedRecoveryStateDATA(raw, base64.RawURLEncoding.EncodeToString(ctx.stateRevision[:]), base64.RawURLEncoding.EncodeToString(ctx.policyRevision[:]), base64.RawURLEncoding.EncodeToString(key), cfg) {
			return errEnrollmentContextStale
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &nativeFinalizationRecovery{daemon: newStoppedDaemon(Config{StateDir: dir}, owner)}, nil
}
func (r *nativeFinalizationRecovery) Close() {
	if r != nil && r.daemon != nil {
		r.daemon.Close()
	}
}

// Explicit bounded SAME-attempt read before any new native publication. There is
// no network work on open, no submit/source retry, and no caller receipt input.
func (r *nativeFinalizationRecovery) resume(ctx context.Context, timeout time.Duration, maxAttempts int) error {
	if r == nil || r.daemon == nil || ctx == nil || ctx.Err() != nil {
		return errEnrollmentContextStale
	}
	d := r.daemon
	d.mu.Lock()
	if err := d.stateHealthLocked(); err != nil {
		d.mu.Unlock()
		return err
	}
	installed := d.stateOwner.saved.native != nil
	ready := d.enrollmentPreparation != nil && d.enrollmentPreparation.historical
	d.mu.Unlock()
	if installed {
		return d.reconcileNativeFinalization()
	}
	if !ready {
		if _, err := d.recoverNativeHistory(ctx, timeout, maxAttempts, true, true); err != nil {
			return err
		}
	}
	return d.finalizeNativeEnrollment(ctx)
}
