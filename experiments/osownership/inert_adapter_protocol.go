//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
)

// Explicit fake protocol, not a real adapter. No production implementation of
// this marker exists. A test-owned Kernel can model ACK/readback/cleanup without
// putting a live namespace/device callback into an ordinary unit binary.
type inertKernelProtocol interface {
	Kernel
	inertProtocolOnly()
}
type inertAdapterProtocol struct {
	effects inertKernelProtocol
	session *adapterSession
}

var _ Kernel = (*inertAdapterProtocol)(nil)

func newInertAdapterProtocol(e inertKernelProtocol, b *CapturedBaseline) (*inertAdapterProtocol, error) {
	if e == nil {
		return nil, ErrConflict
	}
	s, err := newAdapterSession(b)
	if err != nil {
		return nil, err
	}
	return &inertAdapterProtocol{effects: e, session: s}, nil
}
func (a *inertAdapterProtocol) Snapshot(ctx context.Context) (Inventory, error) {
	i, e := a.effects.Snapshot(ctx)
	if e != nil {
		if a.session.pending != nil {
			a.session.poisoned = true
		}
		return i, e
	}
	if a.session.cleanupPending != nil {
		if e = a.session.observeCleanup(i); e != nil {
			return i, e
		}
	}
	if a.session.pending != nil && a.session.pendingACK && !a.session.poisoned && !a.session.cleanupMode {
		if e = a.session.observe(i); e != nil {
			return i, e
		}
	}
	return i, nil
}
func (a *inertAdapterProtocol) Create(ctx context.Context, p Plan) (Handle, error) {
	if a.session.createAttempted || a.session.canonical != nil || a.session.poisoned || a.session.closeAttempted {
		return Handle{}, ErrConflict
	}
	a.session.createAttempted = true
	h, e := a.effects.Create(ctx, p)
	if h.Link.Index > 0 {
		if bind := a.session.bindCreated(p, h); bind != nil {
			a.session.poisoned = true
			copy := h
			a.session.rejectedHandle = &copy
			return h, errors.Join(e, bind)
		}
	}
	if e != nil {
		a.session.poisoned = true
	}
	return h, e
}
func (a *inertAdapterProtocol) Apply(ctx context.Context, h Handle, op Operation) error {
	if op.Kind == "route-del-exact" || op.Kind == "address-del-exact" {
		// Never use cached snapshot as deletion authority. Each call reads anew.
		i, e := a.effects.Snapshot(ctx)
		if e != nil {
			return e
		}
		if e = a.session.beginCleanup(h, i, op); e != nil {
			return e
		}
		return a.effects.Apply(ctx, *a.session.canonical, op)
	}
	if e := a.session.begin(h, op); e != nil {
		return e
	}
	return a.session.recordACK(a.effects.Apply(ctx, *a.session.canonical, op))
}
func (a *inertAdapterProtocol) CloseOriginalFD(h Handle) error {
	i, e := a.effects.Snapshot(context.Background())
	if e != nil {
		return e
	}
	if e = a.session.beginClose(h, i); e != nil {
		return e
	}
	// Only original canonical handle. A repeated/failed close is not retried.
	return a.session.recordClose(a.effects.CloseOriginalFD(*a.session.canonical))
}
