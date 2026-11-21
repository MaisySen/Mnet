//go:build linux && mnet_enrollment_development

package osownership

import (
	"context"
	"os"
)

type ownedCreation struct {
	handle  Handle
	fd      *fileCustody
	attempt *originalAttempt
}

// Bind never drops an opened original on no-handle error. The controller-owned
// attempt remains the close obligation, separately from validated Handle state.
func bindOriginalCustody(attempt *originalAttempt, p Plan, observe func() (Link, error)) (*ownedCreation, error) {
	if attempt == nil || attempt.custody == nil {
		return nil, ErrConflict
	}
	c := &ownedCreation{fd: attempt.custody, attempt: attempt}
	if attempt.closeAttempted || observe == nil {
		return c, attempt.fail(ErrConflict)
	}
	l, e := observe()
	if e != nil {
		return c, attempt.fail(e)
	}
	h, e := AttachCreated(p, l.Index, l.MTU)
	if e != nil || l != h.CreationLink {
		return c, attempt.fail(ErrConflict)
	}
	attempt.handle = &h
	attempt.stage = "original-link-validated"
	c.handle = h
	return c, nil
}

// Legacy value-binding seam now retains the original even when validation fails.
// Live creation always uses an already-published controller-owned attempt below.
func bindOriginalFD(f *os.File, p Plan, observe func() (Link, error)) (*ownedCreation, error) {
	if f == nil {
		return nil, ErrConflict
	}
	attempt := &originalAttempt{plan: p, custody: &fileCustody{file: f, closer: f}, stage: "original-open"}
	return bindOriginalCustody(attempt, p, observe)
}
func createTUNSource(ctx context.Context, ns *namespaceCustody, p Plan, attempt *originalAttempt, observe func() (Link, error)) (*ownedCreation, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || ns == nil || !sameNamespace(ns.identity, p.Binding.Namespace) || attempt == nil {
		return nil, ErrConflict
	}
	_, e := openExclusiveNonpersistentTUN(ns, p.Binding.Name, attempt)
	if e != nil {
		if attempt.custody != nil {
			return &ownedCreation{fd: attempt.custody, attempt: attempt}, e
		}
		return nil, attempt.fail(e)
	}
	return bindOriginalCustody(attempt, p, observe)
}

// Guard every post-creation configuration path before its first possible effect.
// Initial TUNSETIFF belongs to creation before an ownedCreation exists; subsequent
// ioctls/dup/export have no admitted real helper here. Missing lineage refuses.
func (c *ownedCreation) beforeEffect(h Handle, kind string) error {
	if c == nil || c.fd == nil {
		return ErrConflict
	}
	a, b := h, c.handle
	a.TagConfirmed = false
	b.TagConfirmed = false
	if a != b {
		return ErrConflict
	}
	lineage, e := c.fd.effectLineage()
	if e != nil {
		return e
	}
	return lineage.invalidate(kind)
}

// No raw descriptor escape or post-creation ioctl path is exposed. Even a refused
// request invalidates lineage before returning, so it can't leave evidence live.
func (c *ownedCreation) refuseControlEscape(h Handle, kind string) error {
	if e := c.beforeEffect(h, kind); e != nil {
		return e
	}
	return ErrExecutorUnreleased
}

func (c *ownedCreation) guardedEffect(h Handle, kind string, effect func() error) error {
	if c == nil || c.fd == nil || effect == nil {
		return ErrConflict
	}
	c.fd.mu.Lock()
	defer c.fd.mu.Unlock()
	if c.fd.closed || c.fd.file == nil || c.fd.lineage == nil {
		return ErrConflict
	}
	a, b := h, c.handle
	a.TagConfirmed = false
	b.TagConfirmed = false
	if a != b {
		return ErrConflict
	}
	if e := c.fd.lineage.invalidate(kind); e != nil {
		return e
	}
	// Close cannot pass the file lock while this callback is executing. No ledger
	// mutex is held during effects; a concurrent lineage invalidation still wins.
	return effect()
}
