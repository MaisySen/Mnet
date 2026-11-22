//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/netmap"
	"fmt"
	"reflect"
)

// Actual original-resource collector, never a supplier or an inert correlation
// conversion. The original file mutex spans identity + independent full state,
// effective ALL/link operands, framing and verified feature names/bitmap.
// It produces actionable evidence even while final consumer reasoning and
// authenticated peer-MTU binding remain incomplete. Those are not bool waivers.
type sourceRuntimeEvidence struct {
	observedMTU              uint32
	admission                *residentEndpointAdmission
	supplier                 *originalEndpointReactor
	peer                     *daemon.DevelopmentEndpointPeer
	selfProfile, peerProfile netmap.EndpointProfile
	offload                  *originalOffloadHistory
	resources                *originalHeldPacketResources
	routed                   *postPeerRouteResult
	before, after            *originalDeviceEvidence
	diagnostic               ownedSourceDiagnostic
	qdisc                    upQdiscObservation
	names                    *verifiedFeatureNames
	features                 ethtoolActiveFeatures
	policy                   ownedTUNPolicyAssessment
	consumer                 []ipv4ConsumerAssessment
	framing                  tunNestedFraming
}

func (s *operation1ScopedDiagnostic) collectRuntimeCompletion(ctx context.Context, r *postPeerRouteResult) (completionEvidence, error) {
	local, err := s.collectLocalEndpointSource(ctx, r)
	if err != nil {
		return nil, err
	}
	if s.scope.channel != nil {
		a := s.scope.channel
		if err := a.checkResidentSource(ctx, local.resources.namespace, local.resources.owner.plan.Binding, local.routed.identity.generation); err != nil {
			return nil, err
		}
		local.admission = a
		return local, nil
	}
	if err := requireEndpointPeerBinding(s.scope.peer, local.resources.owner.plan.Binding); err != nil {
		return nil, err
	}
	self, peer, err := s.scope.peer.Snapshot()
	if err != nil {
		return nil, err
	}
	local.peer = s.scope.peer
	local.selfProfile = self
	local.peerProfile = peer
	return local, nil
}

// Local preparation is independent of the remote declaration. It still reads
// actual original custody/history/framing/features and the full postsetup graph.
func (s *operation1ScopedDiagnostic) collectLocalEndpointSource(ctx context.Context, r *postPeerRouteResult) (*sourceRuntimeEvidence, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if s == nil || s.scope == nil || s.scope.owner == nil || s.source == nil || s.source.resources == nil || r == nil {
		return nil, afPhase
	}
	o := s.source.resources
	c := s.scope.owner
	if !c.adapter.authorized(ctx) || c.adapter.routed != r || !r.current() || o.owner != c.tx || o.session != c.adapter.session || o.ledger != r.ledger || o.namespace != s.scope.ns || o.creation == nil || o.creation.fd == nil || c.attempt == nil || o.creation.fd != c.attempt.custody || o.creation.attempt != c.attempt {
		return nil, afPhase
	}
	if e := s.scope.check(ctx, c.attempt); e != nil {
		return nil, e
	}
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	if before.identity != r.identity {
		return nil, afPhase
	}
	history := o.creation.attempt.offloadHistory
	if !history.matches(o.creation.attempt, o.creation.fd, o.namespace.identity, o.session.canonical.Link.Name) {
		return nil, errOriginalOffloadHistory
	}
	raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
	if e != nil {
		return nil, e
	}
	sys, e := readPolicySysctlsSource(ctx, o.namespace, raw)
	if e != nil {
		return nil, e
	}
	q, e := o.collectUpQdiscSource(ctx)
	if e != nil {
		return nil, e
	}
	if !exactSetupPostMatches(raw, r.raw) || !reflect.DeepEqual(sys, r.sys) || !sameConstructionQdisc(q, r.qdisc) {
		return nil, afPhase
	}
	var link *LinkRecord
	for i := range raw.Links {
		if raw.Links[i].Index == o.session.canonical.Link.Index {
			if link != nil {
				return nil, afShape
			}
			link = &raw.Links[i]
		}
	}
	if link == nil {
		return nil, afShape
	}
	framing, e := parseTUNNested(*link)
	if e != nil {
		return nil, e
	}
	if framing != (tunNestedFraming{typ: 1}) || link.MTU != MTU {
		return nil, packetFraming
	}
	effective, e := parseEffectiveV4(sys)
	if e != nil {
		return nil, e
	}
	consumers, e := assessIPv4ConsumerOperands(*link, effective)
	if e != nil {
		return nil, e
	}
	policy, e := assessOwnedTUNPolicy(*link)
	if e != nil {
		return nil, e
	}
	socket, e := openOwnedEthtoolSource(o.namespace)
	if e != nil {
		return nil, e
	}
	names, features, readErr := socket.collectFeaturesSource(ctx, uint32(link.Index), link.Name)
	if e = errors.Join(readErr, socket.closeSource()); e != nil {
		return nil, e
	}
	if !features.observedNamesCompatible(names) {
		return nil, packetFraming
	}
	after, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	if before.identity != after.identity || before.resources != o || after.resources != o || before.deviceNamespace != after.deviceNamespace || before.tunName != after.tunName || !r.current() {
		return nil, afPhase
	}
	held, e := heldSourceSnapshot(o.ledger, o.owner, o.session)
	if e != nil {
		return nil, e
	}
	d, e := diagnosticForHeldRecords(raw, sys, held)
	if e != nil {
		return nil, e
	}
	if e = s.scope.check(ctx, c.attempt); e != nil {
		return nil, e
	}
	return &sourceRuntimeEvidence{supplier: s.scope.reactor, observedMTU: link.MTU, offload: history, resources: o, routed: r, before: before, after: after, diagnostic: d, qdisc: cloneUpQdisc(q), names: names, features: features, policy: policy, consumer: consumers, framing: framing}, nil
}
func (e *sourceRuntimeEvidence) validateCompletion(ctx context.Context, t *Transaction) error {
	if e == nil || ctx == nil || ctx.Err() != nil || e.resources == nil || e.routed == nil || e.before == nil || e.after == nil {
		return afPhase
	}
	o := e.resources
	r := e.routed
	s := o.session
	if o.owner != t || s == nil || s.owner != t || s.forwardStep != 6 || s.canonical == nil || t.handle == nil || !sameSessionHandle(*t.handle, *s.canonical) || !r.current() || r.identity != e.after.identity || e.before.identity != e.after.identity || e.before.resources != o || e.after.resources != o || o.ledger != r.ledger || o.creation == nil || o.creation.fd == nil || o.creation.fd.closed || o.creation.attempt == nil || o.creation.attempt.closeAttempted || o.creation.attempt.createRequest != 0x9001 || e.after.deviceNamespace != o.namespace.identity || e.after.tunName != s.canonical.Link.Name || s.poisoned || s.cleanupMode || s.closeAttempted {
		return afPhase
	}
	if e.offload == nil || e.offload != o.creation.attempt.offloadHistory || !o.creation.attempt.offloadHistory.matches(o.creation.attempt, o.creation.fd, o.namespace.identity, s.canonical.Link.Name) {
		return errOriginalOffloadHistory
	}
	if e.framing != (tunNestedFraming{typ: 1}) {
		return packetFraming
	}
	if !exactSetupPostMatches(e.diagnostic.raw, r.raw) || !reflect.DeepEqual(e.diagnostic.sys, r.sys) || !sameConstructionQdisc(e.qdisc, r.qdisc) || !e.features.observedNamesCompatible(e.names) {
		return afPhase
	}
	if e.admission != nil {
		if err := e.admission.checkResidentSource(ctx, o.namespace, t.plan.Binding, r.identity.generation); err != nil {
			return err
		}
	} else {
		if err := requireEndpointPeerBinding(e.peer, t.plan.Binding); err != nil {
			return err
		}
		self, peer, err := e.peer.Snapshot()
		if err != nil || self != e.selfProfile || peer != e.peerProfile {
			return errEndpointPeerProfile
		}
	}
	supplierReason := "installed per-I/O byte-gated supplier"
	if e.admission != nil {
		if e.supplier == nil || e.supplier.owner == nil || e.supplier.owner.tx != t || e.supplier.evidence == nil || e.supplier.evidence.resources != o || e.supplier.frames == nil || e.supplier.receipt != nil {
			return afPhase
		}
		own, err := endpointIPv4(t.plan.Binding.OwnIP)
		if err != nil {
			return err
		}
		peer, err := endpointIPv4(t.plan.Binding.PeerIP)
		if err != nil {
			return err
		}
		f := e.supplier.frames
		f.mu.Lock()
		installed := !f.stopped && f.admission && f.generation == r.identity.generation && f.policy != nil && f.policy.mtu == MTU && f.policy.own == own && f.policy.peer == peer
		f.mu.Unlock()
		if !installed {
			return afPhase
		}
		supplierReason = "exclusive retained parent WireGuard/config/AllowedIPs bridge"
	}
	// Original custody and framing/features are now source-collected. Complete
	// named policy consumer decisions and installed supplier byte-gate are
	// still missing. Peer compatibility is a current native map declaration,
	// not remote kernel attestation. Original offload-control history is now separately bound
	// to its actual successful source syscall, never inferred from these reads.
	unresolved := e.policy.Unresolved
	for _, v := range e.consumer {
		if !v.Resolved {
			unresolved++
		}
	}
	return fmt.Errorf("runtime evidence collected on original generation %d; %d named policy decisions and %s remain unproven", r.identity.generation, unresolved, supplierReason)
}
