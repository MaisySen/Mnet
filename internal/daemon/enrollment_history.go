package daemon

import (
	"context"
	"encoding/base64"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	"path/filepath"
	"time"
)

// Private explicit history read only. Original actual State key and exact
// installed policy govern disclosure, no key use/State installation occurs.
// Works for reopened record4 submit-unknown even after source/challenge expiry.
// Does not bypass normal-loader record5 conflict or reconstruct missing State.
func (d *Daemon) recoverNativeHistoryDATA(ctx context.Context, timeout time.Duration, maxAttempts int) (contract.HistoryReceiptWire, error) {
	var empty contract.HistoryReceiptWire
	if ctx == nil || ctx.Err() != nil {
		return empty, errEnrollmentContextStale
	}
	d.mu.Lock()
	original, err := d.enrollmentContextLocked()
	if err != nil || !d.enrollmentIdentityUnclaimedLocked() {
		d.mu.Unlock()
		return empty, errEnrollmentContextStale
	}
	var j *pending.Journal
	owned := false
	if d.enrollmentPreparation != nil {
		j = d.enrollmentPreparation.journal
	} else {
		j, err = pending.Open(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending"))
		owned = true
	}
	d.mu.Unlock()
	if err != nil {
		return empty, err
	}
	if owned {
		defer j.Close()
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(original.publicKey)
	if err != nil || len(raw) != 32 {
		return empty, errEnrollmentContextStale
	}
	device := base64.RawURLEncoding.EncodeToString(raw)
	current := func() pending.SourceConfig {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.checkEnrollmentContextLocked(original) != nil {
			return pending.SourceConfig{}
		}
		cfg, ok := pending.SourceConfigFor(original.policy)
		if !ok {
			return pending.SourceConfig{}
		}
		return cfg
	}
	// Close of a separately reopened journal must follow actual daemon Close.
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	d.mu.Lock()
	if d.checkEnrollmentContextLocked(original) != nil {
		d.mu.Unlock()
		return empty, errEnrollmentContextStale
	}
	if d.nativeHistoryCancel != nil {
		d.mu.Unlock()
		return empty, errEnrollmentContextStale
	}
	d.nativeHistoryCancel = cancel
	d.mu.Unlock()
	defer func() { d.mu.Lock(); d.nativeHistoryCancel = nil; d.mu.Unlock() }()
	return j.RecoverHistoryDATA(child, device, current, timeout, maxAttempts)
}
