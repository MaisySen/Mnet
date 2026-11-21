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
