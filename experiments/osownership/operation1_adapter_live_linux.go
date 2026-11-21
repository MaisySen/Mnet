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
