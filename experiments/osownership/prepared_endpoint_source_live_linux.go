//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "context"

// This private object is local source evidence, not serializable authority or a
// runtime completion receipt. No API publishes it or writes a transport frame.
type preparedEndpointObservation struct {
	preparation *preparedTransaction
	source      *operation1ScopedDiagnostic
	local       *sourceRuntimeEvidence
}

// Descriptor is DATA for a future reviewed owner-bound inherited channel. A
// decoded/copied descriptor cannot construct preparedEndpointObservation,
// current native membership, completionReceipt, or a supplier-use permit.
type preparedEndpointDescriptor struct {
	Namespace                      Namespace
	Generation                     uint64
	MTU                            int
	OwnIP, PeerIP, OwnKey, PeerKey string
	SelfUntilMS, PeerUntilMS       int64
}

func (s *operation1ScopedDiagnostic) collectPreparedEndpointSource(ctx context.Context, p *preparedTransaction) (*preparedEndpointObservation, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if s == nil || s.scope == nil || s.scope.owner == nil || !p.current(s.scope.owner.tx) {
		return nil, afPhase
	}
	c := s.scope.owner
	local, err := s.collectLocalEndpointSource(ctx, c.adapter.routed)
	if err != nil {
		return nil, err
	}
	if !p.current(c.tx) || local.offload == nil || local.framing != (tunNestedFraming{typ: 1}) {
		return nil, afPhase
	}
	if err := c.tx.live(ctx); err != nil {
		return nil, err
	}
	return &preparedEndpointObservation{preparation: p, source: s, local: local}, nil
}
func (p *preparedEndpointObservation) descriptorSource(ctx context.Context) (preparedEndpointDescriptor, error) {
	var zero preparedEndpointDescriptor
	if !kernelExecutionReleased {
		return zero, ErrExecutorUnreleased
	}
	if p == nil || p.source == nil || p.source.scope == nil || p.source.scope.owner == nil || p.local == nil {
		return zero, afPhase
	}
	c := p.source.scope.owner
	o := p.local.resources
	if !p.preparation.current(c.tx) || !c.adapter.authorized(ctx) || o == nil || o.creation == nil || o.creation.fd == nil || !p.local.routed.current() {
		return zero, afPhase
	}
	if err := p.source.scope.check(ctx, c.attempt); err != nil {
		return zero, err
	}
	if err := c.tx.live(ctx); err != nil {
		return zero, err
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	now, err := o.verifyOriginalDeviceLockedSource(ctx)
	if err != nil {
		return zero, err
	}
	b := c.tx.plan.Binding
	if now.resources != o || now.identity != p.local.after.identity || now.identity != p.local.routed.identity || p.local.offload != o.creation.attempt.offloadHistory || !p.local.offload.matches(o.creation.attempt, o.creation.fd, o.namespace.identity, b.Name) || p.local.observedMTU != MTU || p.local.framing != (tunNestedFraming{typ: 1}) || !p.local.features.observedNamesCompatible(p.local.names) {
		return zero, afPhase
	}
	if err := c.tx.live(ctx); err != nil {
		return zero, err
	}
	return preparedEndpointDescriptor{Namespace: o.namespace.identity, Generation: now.identity.generation, MTU: int(p.local.observedMTU), OwnIP: b.OwnIP, PeerIP: b.PeerIP, OwnKey: b.OwnKey, PeerKey: b.PeerKey, SelfUntilMS: b.SelfExpiry.UnixMilli(), PeerUntilMS: b.PeerExpiry.UnixMilli()}, nil
}
