//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type packetMemorySupplier struct {
	sample packetCustodySample
	err    error
}

func (*packetMemorySupplier) inertPacketCustodyOnly() {}
func (m *packetMemorySupplier) samplePacketCustody(context.Context) (packetCustodySample, error) {
	return m.sample, m.err
}

// Bind creation while ledger is at phase1, then advance using actual inert
// begin/ACK/readback transitions. No OS reader/FD source, not kernel proof.
func packetBindingRig(t *testing.T, activate bool) (*packetBindingCandidate, *packetCustodyReader, *packetMemorySupplier) {
	t.Helper()
	s, _, _, _ := diagnosticComposeRig(t, 1)
	_, _, now := rig()
	inv, e := s.baseline.Compose(s.baseline.raw, SysctlSnapshot(s.baseline.sys), nil, false)
	if e != nil {
		t.Fatal(e)
	}
	h := *s.canonical
	inv.Links = append(inv.Links, h.CreationLink)
	k := &sessionFake{fakeKernel: &fakeKernel{i: inv}}
	a := &inertAdapterProtocol{session: s, effects: k}
	tx, e := NewTransaction(a, &fakeClock{Stamp{Wall: now}}, s.plan.Binding)
	if e != nil {
		t.Fatal(e)
	}
	tx.plan = *s.plan
	tx.plan.Operations = append([]Operation(nil), s.plan.Operations...)
	tx.handle = &h
	l := LinkRecord{Index: h.Link.Index, Name: h.CreationLink.Name, MTU: h.CreationLink.MTU, Hardware: 65534, Flags: 0x1090, Raw: cloneRawAttrs(s.afCreationAttrs)}
	m := &packetMemorySupplier{sample: packetCustodySample{owner: tx, session: s, identity: s.identity, plan: tx.plan, handle: h, namespace: s.identity.Namespace, original: &packetOriginalObject{serial: 1}, generation: 1, createRequest: 0x9001, offloadSetSucceeded: true, tunFlags: 0x1001, link: l, peerOwnKey: tx.plan.Binding.OwnKey, peerKey: tx.plan.Binding.PeerKey, peerMTU: MTU}}
	r := &packetCustodyReader{source: m}
	pass, e := r.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	bound, e := bindPacketCreation(pass)
	if e != nil {
		t.Fatal("creation", e)
	}
	if !activate {
		return bound, r, m
	}
	for n := 1; n < len(s.plan.Operations); n++ {
		op := s.plan.Operations[n]
		if e = s.begin(h, op); e != nil {
			t.Fatal(e)
		}
		if e = k.Apply(context.Background(), h, op); e != nil {
			t.Fatal(e)
		}
		if e = s.recordACK(nil); e != nil {
			t.Fatal(e)
		}
		if e = s.observe(k.i); e != nil {
			t.Fatal(e)
		}
		h = *s.canonical
	}
	tx.handle = &h
	tx.Ready = true
	m.sample.handle = h
	m.sample.link.Alias = h.Link.Alias
	m.sample.link.MTU = h.Link.MTU
	m.sample.link.Flags = 0x1091
	m.sample.link.Raw = cloneRawAttrs(m.sample.link.Raw)
	m.sample.link.Raw[20] = append([]byte(h.Link.Alias), 0)
	m.sample.link.Raw[4] = u32(MTU)
	return bound, r, m
}
func nextPacketPass(t *testing.T, r *packetCustodyReader) *packetCustodyPass {
	t.Helper()
	p, e := r.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestPacketBindingBothDirectionsAndImmutableOutput(t *testing.T) {
	b, r, _ := packetBindingRig(t, true)
	for _, direction := range []packetDirection{packetToPeer, packetToKernel} {
		raw := testIP(17, testUDP(16), direction == packetToKernel, 0)
		out, c, e := b.diagnoseBatch(nextPacketPass(t, r), []packetChunk{{bytes: raw}}, direction)
		if e != nil || c.Accepted != 1 || len(out) != 1 || !reflect.DeepEqual(out[0].bytes, raw) {
			t.Fatal(direction, c, e)
		}
		raw[20] ^= 1
		if reflect.DeepEqual(out[0].bytes, raw) {
			t.Fatal("alias")
		}
		wrong := testIP(17, testUDP(16), direction != packetToKernel, 0)
		_, c, e = b.diagnoseBatch(nextPacketPass(t, r), []packetChunk{{bytes: wrong}}, direction)
		if e != nil || c.Refusals[packetEndpoint] != 1 {
			t.Fatal("direction endpoint", c, e)
		}
	}
}
func TestPacketBindingCustodyAndPlanRefusals(t *testing.T) {
	for _, mode := range []string{"owner", "session", "namespace", "identity", "original", "generation", "create-request", "offload-argument", "offload-failure", "pi", "vnet", "persist", "multiqueue", "hardware", "flags", "index", "name", "alias", "mtu", "peer-mtu", "peer-key", "own-key", "plan", "creation-link", "stopped", "expired", "poisoned", "pending", "replaced-owner-handle"} {
		t.Run(mode, func(t *testing.T) {
			b, r, m := packetBindingRig(t, true)
			v := &m.sample
			switch mode {
			case "owner":
				copy := *v.owner
				v.owner = &copy
			case "session":
				copy := *v.session
				v.session = &copy
			case "namespace":
				v.namespace.Inode++
			case "identity":
				v.identity.AttemptID += "other"
			case "original":
				v.original = &packetOriginalObject{serial: 1}
			case "generation":
				v.generation++
			case "create-request":
				v.createRequest &^= 0x8000
			case "offload-argument":
				v.offloadSetArgument = 1
			case "offload-failure":
				v.offloadSetSucceeded = false
			case "pi":
				v.tunFlags &^= 0x1000
			case "vnet":
				v.tunFlags |= 0x4000
			case "persist":
				v.tunFlags |= 0x800
			case "multiqueue":
				v.tunFlags |= 0x100
			case "hardware":
				v.link.Hardware = 1
			case "flags":
				v.link.Flags |= 2
			case "index":
				v.link.Index++
			case "name":
				v.link.Name += "x"
				v.link.Raw[3] = append([]byte(v.link.Name), 0)
			case "alias":
				v.link.Alias += "x"
				v.link.Raw[20] = append([]byte(v.link.Alias), 0)
			case "mtu":
				v.link.MTU = 1500
				v.link.Raw[4] = u32(1500)
			case "peer-mtu":
				v.peerMTU = 1500
			case "peer-key":
				v.peerKey = v.peerOwnKey
			case "own-key":
				v.peerOwnKey = v.peerKey
			case "plan":
				v.plan.Operations = append([]Operation(nil), v.plan.Operations...)
				v.plan.Operations[5].Route.Protocol++
			case "creation-link":
				v.handle.CreationLink.MTU++
			case "stopped":
				b.owner.stopStarted = true
			case "expired":
				b.owner.clock.(*fakeClock).s.Monotonic = b.owner.deadline.Mono
			case "poisoned":
				b.session.poisoned = true
			case "pending":
				op := b.plan.Operations[5]
				b.session.pending = &op
			case "replaced-owner-handle":
				h := *b.owner.handle
				h.Link.Index++
				b.owner.handle = &h
			}
			if _, e := b.policy(nextPacketPass(t, r)); e == nil {
				t.Fatal("accepted", mode)
			}
		})
	}
}
func TestPacketBindingPassFreshnessAndSingleUse(t *testing.T) {
	b, r, m := packetBindingRig(t, true)
	old := nextPacketPass(t, r)
	latest := nextPacketPass(t, r)
	if _, e := b.policy(old); e == nil {
		t.Fatal("stale")
	}
	if _, e := b.policy(latest); e != nil {
		t.Fatal(e)
	}
	if _, e := b.policy(latest); e == nil {
		t.Fatal("reused")
	}
	old = nextPacketPass(t, r)
	m.err = errors.New("fake collection failed")
	if _, e := r.read(context.Background()); e == nil {
		t.Fatal("missing failure")
	}
	if _, e := b.policy(old); e == nil {
		t.Fatal("failed read retained pass")
	}
	m.err = nil
	other := &packetCustodyReader{source: m}
	if _, e := b.policy(nextPacketPass(t, other)); e == nil {
		t.Fatal("cross reader")
	}
}
func TestPacketBindingCreationFrozenAndNotRetrofitted(t *testing.T) {
	b, r, m := packetBindingRig(t, false)
	before := cloneRawAttrs(b.creation.Raw)
	m.sample.link.Raw[26][8] ^= 1
	if !reflect.DeepEqual(before, b.creation.Raw) {
		t.Fatal("creation alias")
	}
	if _, e := bindPacketCreation(nextPacketPass(t, r)); e == nil {
		t.Fatal("second creation")
	}
	if _, e := b.policy(nextPacketPass(t, r)); e == nil {
		t.Fatal("pre-ready model")
	}
}
func TestPacketBindingChunkAndDrainBounds(t *testing.T) {
	b, r, _ := packetBindingRig(t, true)
	raw := testIP(17, testUDP(16), false, 0)
	batch := []packetChunk{{bytes: raw}, {bytes: raw, truncated: true}, {bytes: make([]byte, MTU+1)}, {bytes: raw[:18]}, {bytes: raw[18:]}}
	out, c, e := b.diagnoseBatch(nextPacketPass(t, r), batch, packetToPeer)
	if e != nil || len(out) != 1 || c.Seen != 5 || c.Accepted != 1 || c.Dropped != 4 || c.Refusals[packetFraming] != 2 {
		t.Fatal(c, e)
	}
	p := nextPacketPass(t, r)
	if out, c, e = b.diagnoseBatch(p, make([]packetChunk, 65), packetToPeer); e == nil || len(out) != 0 || c.Seen != 0 {
		t.Fatal("unbounded")
	}
	if _, e = b.policy(p); e == nil {
		t.Fatal("failed oversized batch retained pass")
	}
	if _, _, e = b.diagnoseBatch(nextPacketPass(t, r), nil, 99); e == nil {
		t.Fatal("unknown direction")
	}
}

func TestPacketCreationOriginalFactsRefuseIndependently(t *testing.T) {
	for _, mode := range []string{"hardware", "up-at-creation", "broadcast", "no-multicast", "vectors", "vector-tail", "bad-framing", "no-offload-history", "wrong-transaction", "namespace", "wrong-creation-handle", "late-creation"} {
		t.Run(mode, func(t *testing.T) {
			b, r, m := packetBindingRig(t, false)
			// Reset only the one-shot slot in this fixture; all other ledger state is
			// the same known-good creation phase. Each vector breaks one prerequisite.
			b.session.packetCreation = nil
			v := &m.sample
			switch mode {
			case "hardware":
				v.link.Hardware = 1
			case "up-at-creation":
				v.link.Flags |= 1
			case "broadcast":
				v.link.Flags |= 2
			case "no-multicast":
				v.link.Flags &^= 0x1000
			case "vectors":
				v.link.Raw[26] = append([]byte(nil), v.link.Raw[26]...)
				v.link.Raw[26][8] ^= 1
			case "vector-tail":
				v.link.Raw[26] = append(append([]byte(nil), v.link.Raw[26]...), 0, 0, 0, 0)
			case "bad-framing":
				v.tunFlags = 1
			case "no-offload-history":
				v.offloadSetSucceeded = false
			case "wrong-transaction":
				copy := *v.owner
				copy.k = &inertAdapterProtocol{session: &adapterSession{}}
				v.owner = &copy
			case "namespace":
				v.namespace.Inode++
			case "wrong-creation-handle":
				v.handle.CreationLink.MTU++
			case "late-creation":
				v.session.forwardStep = 2
			}
			if _, e := bindPacketCreation(nextPacketPass(t, r)); e == nil {
				t.Fatal("accepted", mode)
			}
		})
	}
}
