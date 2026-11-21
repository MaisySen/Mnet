//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"reflect"
)

// Pure source protocol. Only test-memory wire implements this marker. Actual
// held FD/netns observations and ethtool family/profile resolution are absent.
type inertHeldPacketWire interface {
	heldIdentity(context.Context) (heldPacketIdentity, error)
	heldLink(context.Context) (LinkRecord, error)
	heldFeatures(context.Context, []byte) (Datagram, error)
	inertHeldPacketWireOnly()
}

// fdObject is private object identity, never a reusable integer descriptor.
// Namespace evidence represents held TUN device netns AND pinned target netns.
// These records remain claims unless a reviewed original-resource supplier
// collects them. They cannot authorize adopting a device or another namespace.
type heldPacketIdentity struct {
	fdObject          *packetOriginalObject
	generation        uint64
	target, device    Namespace
	identity          adapterIdentity
	handle            Handle
	closed, uncertain bool
}
type heldPacketLedger struct{ current heldPacketIdentity }

func newHeldPacketLedger(v heldPacketIdentity) (*heldPacketLedger, error) {
	if v.fdObject == nil || v.fdObject.serial == 0 || v.generation != 1 || v.closed || v.uncertain || !v.identity.valid() || !sameNamespace(v.target, v.device) || !sameNamespace(v.target, v.handle.Namespace) || !sameNamespace(v.target, v.identity.Namespace) {
		return nil, ErrConflict
	}
	return &heldPacketLedger{current: v}, nil
}

// Every possible mutation/duplication/export/close first invalidates all passes.
// There is intentionally no generic 'recover custody' or unpoison API. Actions
// are only a model enumeration, never syscalls. Even failed/ambiguous close or
// control leaves this lineage permanently unusable for packet evidence.
func (l *heldPacketLedger) invalidate(kind string) error {
	if l == nil {
		return ErrConflict
	}
	switch kind {
	case "ioctl-intent", "duplicate-intent", "export-intent", "close-intent", "namespace-change", "custody-loss":
	default:
		return ErrConflict
	}
	l.current.uncertain = true
	if kind == "close-intent" {
		l.current.closed = true
	}
	if l.current.generation == ^uint64(0) {
		return ErrConflict
	}
	l.current.generation++
	return nil
}
func (l *heldPacketLedger) matches(v heldPacketIdentity) bool {
	return l != nil && !l.current.closed && !l.current.uncertain && reflect.DeepEqual(v, l.current) && v.fdObject == l.current.fdObject
}

type heldPacketObservation struct {
	reader    *heldPacketReader
	af        *afReadPass
	serial    uint64
	identity  heldPacketIdentity
	link      LinkRecord
	framing   tunNestedFraming
	features  ethtoolActiveFeatures
	effective []effectiveV4Word
}
type heldPacketReader struct {
	wire                inertHeldPacketWire
	ledger              *heldPacketLedger
	family              uint16
	recipient, sequence uint32
	profile             string
	last                *heldPacketObservation
	serial              uint64
}

func (r *heldPacketReader) read(ctx context.Context, p *afReadPass) (*heldPacketObservation, error) {
	if r == nil {
		return nil, ErrConflict
	}
	r.last = nil
	if ctx == nil || ctx.Err() != nil || r.wire == nil || r.ledger == nil || r.sequence == ^uint32(0) || r.serial == ^uint64(0) {
		return nil, ErrConflict
	}
	before, e := r.wire.heldIdentity(ctx)
	if e != nil || !r.ledger.matches(before) {
		return nil, ErrConflict
	}
	link, e := r.wire.heldLink(ctx)
	if e != nil {
		return nil, e
	}
	h := before.handle
	if link.Index != h.Link.Index || link.Name != h.Link.Name || link.Alias != h.Link.Alias || link.MTU != MTU || link.Hardware != 65534 || link.Flags & ^uint32(1|0x40|0x10000) != 0x1090 {
		return nil, ErrConflict
	}
	// Cross the independently recorded raw ALL/link pass without substituting
	// records. Nested TUN info is supported here, not silently grandfathered into
	// legacy generic Compose (which remains blocked for this owned link).
	other, e := p.link(link.Index)
	if e != nil || !reflect.DeepEqual(link, other) {
		return nil, afRequest
	}
	effective, e := p.effectiveV4(link.Index)
	if e != nil {
		return nil, e
	}
	framing, e := parseTUNNested(link)
	if e != nil {
		return nil, e
	}
	r.sequence++
	req := ethtoolFeatureRequest{family: r.family, sequence: r.sequence, recipient: r.recipient, ifindex: uint32(link.Index), name: link.Name, profile: r.profile}
	request, e := buildEthtoolFeatureRequest(req)
	if e != nil {
		return nil, e
	}
	d, e := r.wire.heldFeatures(ctx, request)
	if e != nil {
		return nil, e
	}
	features, e := parseEthtoolActive(d, req)
	if e != nil {
		return nil, e
	}
	after, e := r.wire.heldIdentity(ctx)
	if e != nil || !r.ledger.matches(after) || !reflect.DeepEqual(before, after) || before.fdObject != after.fdObject || ctx.Err() != nil {
		return nil, ErrConflict
	}
	// Verify the AF pass is still the same current pass after later observations.
	if _, e = p.requestClaim(); e != nil {
		return nil, e
	}
	link.Raw = cloneRawAttrs(link.Raw)
	r.serial++
	out := &heldPacketObservation{reader: r, af: p, serial: r.serial, identity: before, link: link, framing: framing, features: features, effective: effective}
	r.last = out
	return out, nil
}
func (o *heldPacketObservation) current() bool {
	if o == nil || o.af == nil {
		return false
	}
	if _, e := o.af.requestClaim(); e != nil {
		return false
	}
	link, e := o.af.link(o.link.Index)
	if e != nil || !reflect.DeepEqual(link, o.link) {
		return false
	}
	effective, e := o.af.effectiveV4(o.link.Index)
	if e != nil || !reflect.DeepEqual(effective, o.effective) {
		return false
	}
	return o != nil && o.reader != nil && o.reader.last == o && o.serial == o.reader.serial && o.reader.ledger.matches(o.identity)
}

// No endpoint policy conversion: authenticated peer MTU and original supplier
// facts are not present. Compatible features are diagnostic state, not readiness.
