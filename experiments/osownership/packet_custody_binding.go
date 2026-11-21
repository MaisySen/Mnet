//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"net/netip"
	"reflect"
)

// S39 is an inert correlation model. Only _test.go implements this interface.
// A real supplier must recover these records from original held resources and
// audited setup history. These values are not OS attestation or action authority.
type inertPacketCustodySource interface {
	samplePacketCustody(context.Context) (packetCustodySample, error)
	inertPacketCustodyOnly()
}

// Object identity, never an integer descriptor that the OS could reuse. In this
// model the fake supplier holds the object from creation until close. A future
// supplier needs private original *fileCustody and namespaceCustody ownership.
type packetOriginalObject struct{ serial uint64 }
type packetCustodySample struct {
	owner      *Transaction
	session    *adapterSession
	identity   adapterIdentity
	plan       Plan
	handle     Handle
	namespace  Namespace
	original   *packetOriginalObject
	generation uint64
	// Successful original exclusive TUNSETIFF and TUNSETOFFLOAD(0) history.
	// Generation must cover EVERY later ioctl, duplicate/pass/close and custody
	// loss. No TUNGETOFFLOAD exists. A caller-provided boolean is not a substitute.
	createRequest       uint16
	offloadSetArgument  uint32
	offloadSetSucceeded bool
	// Raw per-device tun_flags observation, NOT ambiguous TUNGETIFF NOFILTER bit.
	tunFlags            uint16
	link                LinkRecord
	peerOwnKey, peerKey string
	peerMTU             int
}
type packetCustodyPass struct {
	reader   *packetCustodyReader
	sequence uint64
	sample   packetCustodySample
	used     bool
}
type packetCustodyReader struct {
	source   inertPacketCustodySource
	sequence uint64
	last     *packetCustodyPass
}

func (r *packetCustodyReader) read(ctx context.Context) (*packetCustodyPass, error) {
	if r == nil {
		return nil, ErrConflict
	}
	r.last = nil // a failed collection invalidates older data too
	if ctx == nil || ctx.Err() != nil || r.source == nil || r.sequence == ^uint64(0) {
		return nil, ErrConflict
	}
	r.sequence++
	v, e := r.source.samplePacketCustody(ctx)
	if e != nil {
		return nil, e
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	v.plan.Operations = append([]Operation(nil), v.plan.Operations...)
	v.link.Raw = cloneRawAttrs(v.link.Raw)
	p := &packetCustodyPass{reader: r, sequence: r.sequence, sample: v}
	r.last = p
	return p, nil
}
func (p *packetCustodyPass) current() bool {
	return p != nil && p.reader != nil && p.reader.last == p && p.sequence != 0 && p.sequence == p.reader.sequence && !p.used
}

type packetBindingCandidate struct {
	reader     *packetCustodyReader
	owner      *Transaction
	session    *adapterSession
	identity   adapterIdentity
	plan       Plan
	handle     Handle
	original   *packetOriginalObject
	generation uint64
	creation   LinkRecord
}

func custodyLink(v packetCustodySample, creation bool) bool {
	h := v.handle
	want := h.Link
	if creation {
		want = h.CreationLink
	}
	l := v.link
	kind, e := linkKind(l)
	// ARPHRD_NONE=65534; initial flags exactly POINTOPOINT|NOARP|MULTICAST.
	// Later only UP/RUNNING/LOWER_UP may vary, never BROADCAST/PROMISC/etc.
	flags := uint32(0x1090)
	if !creation {
		flags = l.Flags & ^uint32(1|0x40|0x10000)
	}
	name, ne := attrText(l.Raw, 3, true)
	alias, ae := attrText(l.Raw, 20, false)
	mtu, me := attrUint(l.Raw, 4, true)
	return e == nil && ne == nil && ae == nil && me == nil && kind == "tun" && l.Hardware == 65534 && flags == 0x1090 && (!creation || l.Flags == 0x1090) && name == l.Name && alias == l.Alias && mtu == l.MTU && Link{Index: l.Index, Name: l.Name, Alias: l.Alias, Kind: kind, MTU: l.MTU} == want
}
func custodyHistory(v packetCustodySample) bool {
	return v.original != nil && v.original.serial != 0 && v.generation != 0 && v.createRequest == 0x9001 && v.offloadSetSucceeded && v.offloadSetArgument == 0 && v.tunFlags == 0x1001
}

// Called only at the original creation phase, not reconstructed from a later
// name/index lookup. Whole creation AF record is retained, not normalized words.
func bindPacketCreation(p *packetCustodyPass) (*packetBindingCandidate, error) {
	if !p.current() {
		return nil, ErrConflict
	}
	p.used = true
	v := p.sample
	s := v.session
	t := v.owner
	if s == nil || (s.owner != nil && s.owner != t) || s.packetCreation != nil || t == nil || s.canonical == nil || s.plan == nil || s.forwardStep != 1 || s.pending != nil || s.poisoned || s.cleanupMode || s.closeAttempted || t.stopStarted || t.fdCloseAttempted || t.clock == nil || t.deadline.expired(t.clock.Now()) || !s.afCreationBound || !custodyHistory(v) || !custodyLink(v, true) {
		return nil, ErrConflict
	}
	a, ok := t.k.(transactionLedgerAdapter)
	if !ok || a.ownershipSession() != s || v.identity != s.identity || !sameNamespace(v.namespace, s.identity.Namespace) || !reflect.DeepEqual(v.plan, *s.plan) || !reflect.DeepEqual(t.plan, *s.plan) || v.handle != *s.canonical || !reflect.DeepEqual(v.link.Raw, s.afCreationAttrs) {
		return nil, ErrConflict
	}
	bound := &packetBindingCandidate{reader: p.reader, owner: t, session: s, identity: v.identity, plan: v.plan, handle: v.handle, original: v.original, generation: v.generation, creation: v.link}
	s.owner = t // original transaction retained at creation, not first packet
	s.packetCreation = bound
	return bound, nil
}

// Fresh one-shot pass for each bounded diagnostic batch. No cached policy lease,
// no sender/reader callbacks, no live conversion. Effective ALL and complete raw
// composition remain separate blocked prerequisites even when this model passes.
func (b *packetBindingCandidate) policy(p *packetCustodyPass) (*endpointPacketPolicy, error) {
	if b == nil || !p.current() {
		return nil, ErrConflict
	}
	p.used = true
	v := p.sample
	s := b.session
	if s == nil || s.packetCreation != b || p.reader != b.reader || v.owner != b.owner || v.session != s || v.identity != b.identity || v.original != b.original || v.generation != b.generation || !custodyHistory(v) || !reflect.DeepEqual(v.plan, b.plan) || !sameNamespace(v.namespace, b.handle.Namespace) || !s.activeEligible() || !s.ownerEligible(b.owner) || !sameSessionHandle(v.handle, *s.canonical) || v.handle.CreationLink != b.handle.CreationLink || !custodyLink(v, false) {
		return nil, ErrConflict
	}
	if !v.handle.TagConfirmed || v.link.MTU != MTU || v.peerMTU != MTU || v.peerOwnKey != b.plan.Binding.OwnKey || v.peerKey != b.plan.Binding.PeerKey {
		return nil, packetFraming
	}
	own, e := netip.ParseAddr(b.plan.Binding.OwnIP)
	if e != nil {
		return nil, e
	}
	peer, e := netip.ParseAddr(b.plan.Binding.PeerIP)
	if e != nil {
		return nil, e
	}
	return newEndpointPacketPolicy(packetFramingReadback{Own: own, Peer: peer, LocalMTU: int(v.link.MTU), PeerMTU: v.peerMTU, TUN: true, NoPI: true, NoVNET: true, NoOffload: true})
}

type packetChunk struct {
	bytes []byte
	// A TUN read record is a whole datagram, NOT a stream chunk. No concatenation
	// or partial tail retry. Future FD reader reserves MTU+1 and marks a full
	// buffer/known truncation suspect, even if an inner length looks valid.
	truncated bool
}

func (b *packetBindingCandidate) diagnoseBatch(p *packetCustodyPass, batch []packetChunk, direction packetDirection) ([]admittedEndpointPacket, packetDrainCounts, error) {
	zero := packetDrainCounts{Refusals: map[packetRefusal]uint64{}}
	if len(batch) > 64 || (direction != packetToPeer && direction != packetToKernel) {
		if p.current() {
			p.used = true
		}
		return nil, zero, ErrConflict
	}
	policy, e := b.policy(p)
	if e != nil {
		return nil, zero, e
	}
	var out []admittedEndpointPacket
	c := zero
	for _, chunk := range batch {
		c.Seen++
		c.Bytes += uint64(min(len(chunk.bytes), MTU+1))
		if chunk.truncated || len(chunk.bytes) > MTU {
			c.Dropped++
			c.Refusals[packetFraming]++
			continue
		}
		admitted, e := policy.admit(chunk.bytes, direction)
		if e != nil {
			c.Dropped++
			reason, ok := e.(packetRefusal)
			if !ok {
				reason = packetHeader
			}
			c.Refusals[reason]++
			continue
		}
		out = append(out, admitted)
		c.Accepted++
	}
	return out, c, nil
}
