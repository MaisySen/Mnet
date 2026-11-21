//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"reflect"
	"time"
)

// Controller publishes this before opening anything. Every post-open failure
// retains the same original custody, even without a validated Handle. Fields
// are owned by the controller lane, never the packet-generation mutex.
type originalAttempt struct {
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

// Pure equality/correlation seam. Calling it alone does not establish source
// provenance; only resident source checks lifetime/ns before and after reads.
func parseOriginalAbsence(a *originalAttempt, ticket *originalAbsenceTicket, b *CapturedBaseline, raw RawInventory, sys SysctlSnapshot) (*originalAbsenceEvidence, error) {
	if a == nil || ticket == nil || b == nil || a.owner == nil || a.owner.adapter == nil || a.owner.adapter.session == nil || a.owner.adapter.session.baseline != b || a.absenceTicket != ticket || !ticket.active || ticket.used || ticket.attempt != a || ticket.owner != a.owner || ticket.sequence != a.absenceSequence || !a.closeAttempted || a.custody == nil || !sameNamespace(a.plan.Binding.Namespace, b.ns) || !reflect.DeepEqual(raw, b.raw) || !reflect.DeepEqual(map[string]string(sys), b.sys) {
		return nil, ErrConflict
	}
	return &originalAbsenceEvidence{ticket: ticket, attempt: a, custody: a.custody}, nil
}

type originalAbsenceSource interface {
	observeOriginalAbsence(context.Context, *originalAttempt) (*originalAbsenceEvidence, error)
}

func (c *operation1Controller) stopOwnedLocked(ctx context.Context) error {
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
	bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
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
