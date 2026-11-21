//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// Eventual real ownership adapter Apply(set-alias-mtu-exact) must call THIS
// entrypoint, not send/collect individually. That adapter does not exist yet;
// inertAdapterProtocol cannot call or implement this source integration.
// Precondition: resident locked OS thread already owns target namespace. This
// method never enters/switches namespace or unlocks a thread on its own.
func (o *originalHeldPacketResources) applyFirstStepSource(ctx context.Context, p Plan) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || o == nil || o.creation == nil || o.creation.fd == nil || o.namespace == nil || o.ledger == nil {
		return ErrConflict
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	if e := o.namespace.checkCurrent(); e != nil {
		return e
	}
	if _, e := o.verifyOriginalDeviceLockedSource(ctx); e != nil {
		return e
	}
	diagnostic, e := o.collectSourceDiagnostic(ctx)
	if e != nil {
		return e
	}
	socket, e := openRouteSocket(o.namespace)
	if e != nil {
		return e
	}
	defer socket.Close()
	// Fresh, one-exchange socket: sequence1 cannot collide with another request.
	return orchestrateFirstStep(ctx, o.ledger, p, diagnostic, 1, socket.pid, firstStepDependencies{
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
