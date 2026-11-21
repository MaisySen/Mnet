//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// operation1Adapter.Apply calls this through operation1OriginalSource, never
// the send/collect dependencies individually. Original creation and Snapshot
// active conversion remain held; inertAdapterProtocol cannot call this path.
// Precondition: resident locked OS thread already owns target namespace. This
// method never enters/switches namespace or unlocks a thread on its own.
func (o *originalHeldPacketResources) applyFirstStepSource(ctx context.Context, p Plan) (*firstStepCompletion, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || o == nil || o.creation == nil || o.creation.fd == nil || o.namespace == nil || o.ledger == nil {
		return nil, ErrConflict
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	if e := o.namespace.checkCurrent(); e != nil {
		return nil, e
	}
	if _, e := o.verifyOriginalDeviceLockedSource(ctx); e != nil {
		return nil, e
	}
	diagnostic, e := o.collectSourceDiagnostic(ctx)
	if e != nil {
		return nil, e
	}
	socket, e := openRouteSocket(o.namespace)
	if e != nil {
		return nil, e
	}
	defer socket.Close()
	// Fresh, one-exchange socket: sequence1 cannot collide with another request.
	return orchestrateFirstStepResult(ctx, o.ledger, p, diagnostic, 1, socket.pid, firstStepDependencies{
		send: func(ctx context.Context, t *packetSetupTransition, permit *firstLinkSendPermit) (Datagram, error) {
			return o.sendFirstLinkUpdateLockedSource(ctx, socket, t, permit)
		},
		collect: func(ctx context.Context, t *packetSetupTransition) (setupReadbackRecord, error) {
			proposal, e := o.collectSetupPostLockedSource(ctx, t)
			if e != nil {
				return setupReadbackRecord{}, e
			}
			return proposal.record, nil
		},
	})
}
