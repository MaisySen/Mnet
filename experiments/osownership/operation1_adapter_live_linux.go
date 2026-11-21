//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"time"
)

// SOURCE ONLY. Creates bookkeeping, never a namespace/device/FD/helper. The
// operation1 Kernel has only precreation and three bounded DOWN-phase Snapshot
// conversions. Create/effects remain execution-unreleased. Address/UP/route and
// every Ready path are refused; no packet supplier is constructed here.
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

func (s *operation1OriginalSource) applyOperation1(ctx context.Context, p Plan) (*firstStepCompletion, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if s == nil || s.resources == nil {
		return nil, ErrConflict
	}
	return s.resources.applyFirstStepSource(ctx, p)
}
