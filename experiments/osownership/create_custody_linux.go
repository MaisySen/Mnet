//go:build linux && mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"os"
)

type ownedCreation struct {
	handle Handle
	fd     *fileCustody
}

// Binding helper never drops a newly opened FD on a no-handle return. It does
// not set alias/MTU: that is its own explicit ACK/readback transaction step.
// On readback failure original FD is closed exactly once before returning nil.
func bindOriginalFD(f *os.File, p Plan, observe func() (Link, error)) (_ *ownedCreation, err error) {
	if f == nil {
		return nil, ErrConflict
	}
	c := &fileCustody{file: f}
	keep := false
	defer func() {
		if !keep {
			err = errors.Join(err, c.Close())
		}
	}()
	if observe == nil {
		return nil, ErrConflict
	}
	l, e := observe()
	if e != nil {
		return nil, e
	}
	h, e := AttachCreated(p, l.Index, l.MTU)
	if e != nil || l != h.CreationLink {
		return nil, ErrConflict
	}
	keep = true
	return &ownedCreation{handle: h, fd: c}, nil
}
func createTUNSource(ctx context.Context, ns *namespaceCustody, p Plan, observe func() (Link, error)) (*ownedCreation, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if ns == nil {
		return nil, ErrConflict
	}
	if !sameNamespace(ns.identity, p.Binding.Namespace) {
		return nil, ErrConflict
	}
	f, e := openExclusiveNonpersistentTUN(ns, p.Binding.Name)
	if e != nil {
		return nil, e
	}
	return bindOriginalFD(f, p, observe)
}

// Guard every post-creation configuration path before its first possible effect.
// Initial TUNSETIFF belongs to creation before an ownedCreation exists; subsequent
// ioctls/dup/export have no admitted real helper here. Missing lineage refuses.
func (c *ownedCreation) beforeEffect(h Handle, kind string) error {
	if c == nil || c.fd == nil || c.fd.closed || c.fd.file == nil || c.fd.lineage == nil {
		return ErrConflict
	}
	a, b := h, c.handle
	a.TagConfirmed = false
	b.TagConfirmed = false
	if a != b {
		return ErrConflict
	}
	return c.fd.lineage.invalidate(kind)
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
	if effect == nil {
		return ErrConflict
	}
	if e := c.beforeEffect(h, kind); e != nil {
		return e
	}
	// Callback cannot restore lineage, even on success, failure or ambiguous ACK.
	return effect()
}
