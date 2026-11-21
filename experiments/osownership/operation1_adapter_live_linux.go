//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"time"
)

// SOURCE ONLY. Creates bookkeeping, never a namespace/device/FD/helper. The
// operation1 Kernel is connected, but Start's actual Snapshot conversion and
// Create are both held gates. No caller can obtain a Ready path through it.
func newOperation1SourceController(b *CapturedBaseline, id adapterIdentity, binding Binding) (*operation1Controller, error) {
	session, e := newAdapterSession(b, id)
	if e != nil {
		return nil, e
	}
	c := &operation1Controller{}
	adapter := &operation1Adapter{controller: c, session: session}
	c.adapter = adapter
	tx, e := NewTransaction(adapter, operation1Clock{origin: time.Now()}, binding)
	if e != nil {
		return nil, e
	}
	c.tx = tx
	session.owner = tx
	return c, nil
}

// Install only originally retained resources bound to this exact controller's
// owner/session. The current creation path cannot establish this precondition.
// Future creation must do so on the same ownership lane, including error custody.
func (c *operation1Controller) bindOriginalSource(ctx context.Context, o *originalHeldPacketResources) error {
	return c.withOwner(ctx, func(ctx context.Context) error {
		if o == nil || o.owner != c.tx || o.session != c.adapter.session || o.creation == nil || o.ledger == nil || o.namespace == nil || c.adapter.source != nil {
			return ErrConflict
		}
		if c.attempt == nil || c.attempt.custody != o.creation.fd || c.attempt.owner != c || c.attempt.handle == nil || *c.attempt.handle != o.creation.handle {
			return ErrConflict
		}
		c.adapter.source = &operation1OriginalSource{resources: o}
		return nil
	})
}

type operation1OriginalSource struct{ resources *originalHeldPacketResources }

func (s *operation1OriginalSource) diagnoseOperation1(ctx context.Context) (ownedSourceDiagnostic, error) {
	var zero ownedSourceDiagnostic
	if !kernelExecutionReleased {
		return zero, ErrExecutorUnreleased
	}
	if s == nil || s.resources == nil || s.resources.creation == nil || s.resources.creation.fd == nil {
		return zero, ErrConflict
	}
	o := s.resources
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return zero, e
	}
	diagnostic, e := o.collectSourceDiagnostic(ctx)
	if e != nil {
		return zero, e
	}
	after, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return zero, e
	}
	if before.identity != after.identity || before.tunName != after.tunName || before.deviceNamespace != after.deviceNamespace {
		return zero, ErrConflict
	}
	return diagnostic, nil
}
func (s *operation1OriginalSource) applyOperation1(ctx context.Context, p Plan) (*firstStepCompletion, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if s == nil || s.resources == nil {
		return nil, ErrConflict
	}
	return s.resources.applyFirstStepSource(ctx, p)
}

// No caller/release. Publish the owner attempt BEFORE opening. Errors return
// with the original retained attempt even when no validated Handle exists.
func (c *operation1Controller) createOriginalAttemptSource(ctx context.Context, ns *namespaceCustody, p Plan, observe func() (Link, error)) (*ownedCreation, error) {
	var creation *ownedCreation
	e := c.withOwner(ctx, func(ctx context.Context) error {
		if !kernelExecutionReleased {
			return ErrExecutorUnreleased
		}
		if c.attempt != nil || c.tx.stopStarted {
			return ErrConflict
		}
		c.attempt = &originalAttempt{owner: c, plan: p, stage: "unopened"}
		c.tx.PendingCleanup = true
		var e error
		creation, e = createTUNSource(ctx, ns, p, c.attempt, observe)
		return e
	})
	return creation, e
}
