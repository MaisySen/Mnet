//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"reflect"
)

// Controller publishes this before opening anything. Every post-open failure
// retains the same original custody, even without a validated Handle. Fields
// are owned by the controller lane, never the packet-generation mutex.
type originalAttempt struct {
	offloadHistory   *originalOffloadHistory
	createRequest    uint16 // source records only after actual successful TUNSETIFF
	owner            *operation1Controller
	plan             Plan
	custody          *fileCustody
	handle           *Handle
	stage            string
	createErr        error
	closeAttempted   bool
	closeErr         error
	absenceConfirmed bool
	absenceSequence  uint64
	absenceTicket    *originalAbsenceTicket
}

func (a *originalAttempt) retain(c *fileCustody) error {
	if a == nil || c == nil || a.custody != nil || a.closeAttempted {
		return ErrConflict
	}
	a.custody = c
	a.stage = "original-open"
	return nil
}

// Shared post-open seam: every fallible check is invoked only AFTER original
// custody is published. The opener must prevalidate the empty attempt before open.
func (a *originalAttempt) afterOpen(custody *fileCustody, check func() error) error {
	if e := a.retain(custody); e != nil {
		return e
	}
	if check == nil {
		return a.fail(ErrConflict)
	}
	return a.fail(check())
}
func (a *originalAttempt) fail(e error) error {
	if a == nil {
		return errors.Join(e, ErrConflict)
	}
	a.createErr = errors.Join(a.createErr, e)
	return e
}

// No Snapshot/Rollback/Handle gate here. Called on the ownership lane after all
// in-flight owned calls have completed. Error remains sticky on every call.
func (a *originalAttempt) closeOriginal() error {
	if a == nil {
		return ErrConflict
	}
	if a.closeAttempted {
		return a.closeErr
	}
	if a.custody == nil {
		return nil
	} // no successful open, no original close owed
	a.closeAttempted = true
	a.stage = "close-attempted"
	a.closeErr = a.custody.Close()
	return a.closeErr
}

var errAbsenceUnverified = errors.New("original close does not prove raw absence; cleanup unverified")

// Private scoped absence, NOT an Inventory or readiness capability. Production
// has no supplier/mint until complete raw reconciliation provenance is reviewed.
type originalAbsenceTicket struct {
	attempt      *originalAttempt
	owner        *operation1Controller
	sequence     uint64
	active, used bool
}
type originalAbsenceEvidence struct {
	ticket  *originalAbsenceTicket
	attempt *originalAttempt
	custody *fileCustody
}

// Shared pre-open/post-close baseline comparison. Only the already-reviewed
// pinned comparison-copy counter/timestamp policy applies. Target name must be
// absent in both snapshots, even if an invalid baseline had included it.
func originalBaselineMatches(p Plan, b *CapturedBaseline, raw RawInventory, sys SysctlSnapshot) bool {
	if b == nil || !sameNamespace(p.Binding.Namespace, b.ns) || p.Binding.Name == "" {
		return false
	}
	for _, snapshot := range []RawInventory{b.raw, raw} {
		for _, link := range snapshot.Links {
			if link.Name == p.Binding.Name {
				return false
			}
		}
	}
	return exactSetupPostMatches(raw, b.raw) && reflect.DeepEqual(map[string]string(sys), b.sys)
}

// Pure equality/correlation seam. Calling it alone does not establish source
// provenance; only resident source checks lifetime/ns before and after reads.
func parseOriginalAbsence(a *originalAttempt, ticket *originalAbsenceTicket, b *CapturedBaseline, raw RawInventory, sys SysctlSnapshot) (*originalAbsenceEvidence, error) {
	if a == nil || ticket == nil || b == nil || a.owner == nil || a.owner.adapter == nil || a.owner.adapter.session == nil || a.owner.adapter.session.baseline != b || a.absenceTicket != ticket || !ticket.active || ticket.used || ticket.attempt != a || ticket.owner != a.owner || ticket.sequence != a.absenceSequence || !a.closeAttempted || a.custody == nil || !sameNamespace(a.plan.Binding.Namespace, b.ns) || !originalBaselineMatches(a.plan, b, raw, sys) {
		return nil, ErrConflict
	}
	return &originalAbsenceEvidence{ticket: ticket, attempt: a, custody: a.custody}, nil
}

type originalAbsenceSource interface {
	observeOriginalAbsence(context.Context, *originalAttempt) (*originalAbsenceEvidence, error)
}

func (c *operation1Controller) stopOwnedLocked(ctx context.Context) (ret error) {
	completed := false
	if c == nil || c.tx == nil {
		return ErrConflict
	}
	if record := c.endpointCleanup; record != nil {
		if record.owner != c {
			return errors.Join(errAbsenceUnverified, ErrConflict)
		}
		if record.started {
			if record.attempt != c.attempt || !record.finished {
				return errors.Join(errAbsenceUnverified, ErrConflict)
			}
			return record.err
		}
		// Claim before close, PendingCleanup changes, sequence increment or ticket.
		record.started = true
		record.attempt = c.attempt
		record.err = errAbsenceUnverified
		var initialSequence uint64
		if c.attempt != nil {
			initialSequence = c.attempt.absenceSequence
		}
		defer func() {
			if completed {
				record.err = ret
			} else {
				record.err = errors.Join(errAbsenceUnverified, ErrConflict)
			}
			record.pendingCleanup = c.tx.PendingCleanup
			record.closeErr = c.tx.fdCloseError
			if c.attempt != nil {
				record.absenceConfirmed = c.attempt.absenceConfirmed
				record.absenceAttempted = c.attempt.absenceSequence != initialSequence
			}
			record.finished = completed
		}()
		if ctx != nil {
			// First-entry cleanup-only interval includes original close and absence.
			// Operation cancellation never skips close. Later calls, including the
			// defer, return the result without another deadline or observation.
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), endpointCleanupBudget)
			record.deadline, _ = ctx.Deadline()
			defer cancel()
		}
		// Nil ctx still closes the original, records not-observed/unverified and
		// is final. An expiry/refusal within the single interval is also final.
	}

	ret = c.stopOwnedOnceLocked(ctx)
	completed = true
	return ret
}

// Only stopOwnedLocked calls this body, after the endpoint memo was claimed.
func (c *operation1Controller) stopOwnedOnceLocked(ctx context.Context) error {
	// Revoke complete route/UP/address/disable ancestry before close/absence.
	if c != nil && c.adapter != nil && c.adapter.routed != nil {
		c.adapter.routed.lifetime.revoke()
	}
	if c != nil && c.adapter != nil && c.adapter.upped != nil {
		c.adapter.upped.lifetime.revoke()
	}
	if c != nil && c.adapter != nil && c.adapter.addressed != nil {
		c.adapter.addressed.lifetime.revoke()
	}
	if c != nil && c.adapter != nil && c.adapter.disabled != nil {
		c.adapter.disabled.lifetime.revoke()
	}
	c.tx.stopStarted = true
	c.tx.Ready = false
	if c.attempt == nil {
		// No attempt record cannot excuse a legacy handle/custody obligation.
		if c.tx.handle != nil || c.tx.PendingCleanup {
			c.tx.PendingCleanup = true
			return ErrConflict
		}
		return nil
	}
	attempt := c.attempt
	if attempt.owner != c {
		c.tx.PendingCleanup = true
		return ErrConflict
	}
	// Original close ALWAYS precedes optional observation. Context cancellation
	// and readiness-policy errors can prevent observation, never this close.
	closeErr := attempt.closeOriginal()
	c.tx.fdCloseAttempted = attempt.closeAttempted
	if c.tx.fdCloseError == nil {
		c.tx.fdCloseError = closeErr
	}
	c.tx.PendingCleanup = attempt.custody != nil
	if attempt.custody == nil {
		return attempt.createErr
	}

	attempt.absenceConfirmed = false
	if ctx == nil || ctx.Err() != nil || c.absence == nil {
		return errors.Join(closeErr, errAbsenceUnverified)
	}
	if attempt.absenceSequence == ^uint64(0) {
		return errors.Join(closeErr, ErrConflict)
	}
	bounded, cancel := context.WithTimeout(ctx, endpointCleanupBudget)
	defer cancel()
	ctx = bounded
	attempt.absenceSequence++
	ticket := &originalAbsenceTicket{attempt: attempt, owner: c, sequence: attempt.absenceSequence, active: true}
	attempt.absenceTicket = ticket
	defer func() { ticket.active = false; attempt.absenceTicket = nil }()
	proof, e := c.absence.observeOriginalAbsence(ctx, attempt)
	if e != nil {
		return errors.Join(closeErr, e, errAbsenceUnverified)
	}
	if ctx.Err() != nil || proof == nil || proof.ticket != ticket || !ticket.active || ticket.used || ticket.owner != c || ticket.attempt != attempt || ticket.sequence != attempt.absenceSequence || proof.attempt != attempt || proof.custody != attempt.custody || !attempt.closeAttempted {
		return errors.Join(closeErr, ErrConflict, errAbsenceUnverified)
	}
	ticket.used = true
	attempt.absenceConfirmed = true

	if attempt.closeErr != nil || c.tx.fdCloseError != nil {
		return c.tx.fdCloseError
	}
	c.tx.PendingCleanup = false
	return nil
}
