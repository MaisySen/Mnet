//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
)

// Pure setup slice supports ONLY the first exact alias/MTU step. This has a
// deterministic raw poststate without guessed kernel address/IPv6 side effects.
// Later disable/address/up/route operations remain unreleased until their full
// raw expected side effects are independently derived. No generic diff allowlist.
type packetSetupTransition struct {
	ledger      *heldPacketLedger
	owner       *Transaction
	original    heldPacketIdentity
	generation  uint64
	plan        Plan
	operation   Operation
	expectedRaw RawInventory
	expectedSys SysctlSnapshot
	request     []byte
	done        bool
}

// Inert wire has NO real implementation. Identity records model original held
// FD/device namespace checks. No real FD verification has been implemented.
type inertSetupWire interface {
	setupEffect(context.Context, []byte) (Datagram, error)
	setupReadback(context.Context) (setupReadbackRecord, error)
	inertSetupWireOnly()
}
type setupReadbackRecord struct {
	before, after heldPacketIdentity
	raw           RawInventory
	sys           SysctlSnapshot
	names         *verifiedFeatureNames
	features      ethtoolActiveFeatures
}
type setupACKEvidence struct {
	transition *packetSetupTransition
	request    []byte
}
type setupPostEvidence struct {
	transition *packetSetupTransition
	generation uint64
	raw        RawInventory
	sys        SysctlSnapshot
	identity   heldPacketIdentity
}

func cloneSysExact(v SysctlSnapshot) SysctlSnapshot {
	if v == nil {
		return nil
	}
	out := SysctlSnapshot{}
	for k, s := range v {
		out[k] = s
	}
	return out
}
func (l *heldPacketLedger) beginExactSetup(p Plan, op Operation, diagnostic ownedSourceDiagnostic, sequence, recipient uint32) (*packetSetupTransition, error) {
	if l == nil {
		return nil, ErrConflict
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.setup != nil || l.current.closed || l.current.uncertain || l.current.generation >= ^uint64(0)-2 || diagnostic.held == nil || diagnostic.held.ledger != l || diagnostic.held.identity != l.current || !diagnostic.lineageUsable || len(p.Operations) != 6 || op != p.Operations[1] || op.Kind != "set-alias-mtu-exact" || !sameNamespace(l.current.target, p.Binding.Namespace) || diagnostic.held.session == nil || diagnostic.held.session.forwardStep != 1 || diagnostic.held.session.plan == nil || !reflect.DeepEqual(p, *diagnostic.held.session.plan) || sequence == 0 || recipient == 0 {
		return nil, ErrConflict
	}
	t := diagnostic.held.owner
	if t == nil || diagnostic.held.session.owner != t || t.stopStarted || t.fdCloseAttempted || t.clock == nil || t.deadline.expired(t.clock.Now()) || !reflect.DeepEqual(t.plan, p) {
		return nil, ErrConflict
	}
	h := l.current.handle
	checked, e := sourceDiagnosticFromRecords(diagnostic.raw, diagnostic.sys, h, true)
	if e != nil || checked.framing != diagnostic.framing || !reflect.DeepEqual(checked.effective, diagnostic.effective) {
		return nil, ErrConflict
	}
	if h.TagConfirmed || h.Link.Name != p.Binding.Name || h.Link.Alias != p.Binding.Alias || op.MTU != MTU || op.Name != h.Link.Name || op.Alias != h.Link.Alias {
		return nil, ErrConflict
	}
	raw := cloneRawExact(diagnostic.raw)
	found := false
	for n := range raw.Links {
		link := &raw.Links[n]
		if link.Index == h.CreationLink.Index {
			if found || (Link{Index: link.Index, Name: link.Name, Alias: link.Alias, Kind: "tun", MTU: link.MTU}) != h.CreationLink {
				return nil, ErrConflict
			}
			if _, e := parseTUNNested(*link); e != nil {
				return nil, e
			}
			link.Alias = h.Link.Alias
			link.MTU = MTU
			link.Raw[20] = append([]byte(link.Alias), 0)
			link.Raw[4] = u32(MTU)
			found = true
		}
	}
	if !found {
		return nil, ErrConflict
	}
	mutation, e := BuildExactMutation(h, op)
	if e != nil {
		return nil, e
	}
	request := make([]byte, 16+len(mutation.payload))
	binary.NativeEndian.PutUint32(request, uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:], mutation.kind)
	flags, e := mutationRequestFlags(mutation.kind)
	if e != nil {
		return nil, e
	}
	binary.NativeEndian.PutUint16(request[6:], flags)
	binary.NativeEndian.PutUint32(request[8:], sequence)
	binary.NativeEndian.PutUint32(request[12:], recipient)
	copy(request[16:], mutation.payload)
	original := l.current
	if e = l.invalidateLocked("config-intent"); e != nil {
		return nil, e
	}
	token := &packetSetupTransition{ledger: l, owner: t, original: original, generation: l.current.generation, plan: p, operation: op, expectedRaw: raw, expectedSys: cloneSysExact(diagnostic.sys), request: request}
	token.plan.Operations = append([]Operation(nil), p.Operations...)
	l.setup = token
	return token, nil
}

// An explicitly token-bound second guard is permitted. Generic invalidation
// (close/another config/dup/export) cancels the token instead. Never assume +1.
func (t *packetSetupTransition) guardAgain() error {
	if t == nil || t.ledger == nil {
		return ErrConflict
	}
	l := t.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.setup != t || t.done || l.current.closed || l.current.generation != t.generation {
		return ErrConflict
	}
	if e := l.invalidateLocked("config-intent"); e != nil {
		l.setup = nil
		t.done = true
		return e
	}
	t.generation = l.current.generation
	return nil
}
func (t *packetSetupTransition) parseACK(d Datagram) (*setupACKEvidence, error) {
	if t == nil {
		return nil, ErrConflict
	}
	b := d.Bytes
	// netlink_ack in pinned af_netlink.c marks zero ACKs capped, regardless of
	// NETLINK_CAP_ACK. Error/TLV/multipart variants deliberately refuse here.
	// The zero ACK carries the echoed original request header.
	if d.SenderPID != 0 || d.Truncated || len(b) != 36 || binary.NativeEndian.Uint32(b) != 36 || binary.NativeEndian.Uint16(b[4:]) != 2 || binary.NativeEndian.Uint16(b[6:]) != 0x100 || binary.NativeEndian.Uint32(b[8:]) != binary.NativeEndian.Uint32(t.request[8:]) || binary.NativeEndian.Uint32(b[12:]) != binary.NativeEndian.Uint32(t.request[12:]) || binary.NativeEndian.Uint32(b[16:]) != 0 || !bytes.Equal(b[20:], t.request[:16]) {
		return nil, ErrConflict
	}
	return &setupACKEvidence{transition: t, request: append([]byte(nil), t.request...)}, nil
}
func (t *packetSetupTransition) parsePost(v setupReadbackRecord) (*setupPostEvidence, error) {
	if t == nil || t.ledger == nil {
		return nil, ErrConflict
	}
	l := t.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.setup != t || t.done || l.current.closed || l.current.generation != t.generation || !reflect.DeepEqual(v.before, l.current) || v.before.fdObject != t.original.fdObject || !reflect.DeepEqual(v.before, v.after) || v.before.fdObject != v.after.fdObject || !exactSetupPostMatches(v.raw, t.expectedRaw) || !reflect.DeepEqual(v.sys, t.expectedSys) || !v.features.observedNamesCompatible(v.names) {
		return nil, ErrConflict
	}
	var matched bool
	for _, link := range v.raw.Links {
		if link.Index == t.original.handle.Link.Index {
			if _, e := parseTUNNested(link); e != nil {
				return nil, e
			}
			matched = true
		}
	}
	if !matched {
		return nil, ErrConflict
	}
	return &setupPostEvidence{transition: t, generation: t.generation, raw: cloneRawExact(v.raw), sys: cloneSysExact(v.sys), identity: v.after}, nil
}
func (t *packetSetupTransition) finish(ack *setupACKEvidence, post *setupPostEvidence) error {
	if t == nil || t.ledger == nil {
		return ErrConflict
	}
	l := t.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.setup != t || t.done {
		return ErrConflict
	}
	t.done = true
	l.setup = nil
	if ack == nil || t.owner == nil || t.owner.stopStarted || t.owner.fdCloseAttempted || t.owner.clock == nil || t.owner.deadline.expired(t.owner.clock.Now()) || post == nil || ack.transition != t || post.transition != t || !bytes.Equal(ack.request, t.request) || post.generation != t.generation || l.current.generation != t.generation || l.current.closed || !reflect.DeepEqual(l.current, post.identity) || !exactSetupPostMatches(post.raw, t.expectedRaw) || !reflect.DeepEqual(post.sys, t.expectedSys) || l.current.generation == ^uint64(0) {
		return ErrConflict
	}
	l.current.generation++
	l.current.uncertain = false
	l.current.handle.TagConfirmed = true
	return nil
}
func (t *packetSetupTransition) abandon() {
	if t == nil || t.ledger == nil {
		return
	}
	l := t.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	t.done = true
	if l.setup == t {
		l.setup = nil
		l.current.uncertain = true
	}
}
func (t *packetSetupTransition) runInert(ctx context.Context, wire inertSetupWire) (err error) {
	if t == nil || wire == nil || ctx == nil || ctx.Err() != nil {
		if t != nil {
			t.abandon()
		}
		return ErrConflict
	}
	defer func() {
		if err != nil {
			t.abandon()
		}
	}()
	if err = t.guardAgain(); err != nil {
		return err
	}
	d, e := wire.setupEffect(ctx, append([]byte(nil), t.request...))
	if e != nil {
		return e
	}
	ack, e := t.parseACK(d)
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	raw, e := wire.setupReadback(ctx)
	if e != nil {
		return e
	}
	post, e := t.parsePost(raw)
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return t.finish(ack, post)
}
