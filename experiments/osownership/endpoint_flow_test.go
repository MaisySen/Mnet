//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"testing"
)

// Protocol DATA only. No WG object, original controller, native context,
// inherited admission constructor, syscall, worker, timer or callback.
func flowMemoryPair(t *testing.T) (*endpointFrameState, *endpointFrameState) {
	t.Helper()
	parent, _ := newEndpointFrameState(7, 2, packetRig(t))
	child, _ := newEndpointFrameState(7, 1, packetRig(t))
	transfer := func(from, to *endpointFrameState) {
		raw, e := from.nextWrite()
		if e != nil {
			t.Fatal(e)
		}
		f, e := decodeEndpointFrame(raw)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = to.receive(f); e != nil {
			t.Fatal(e)
		}
		if e = from.finishWrite(len(raw), nil); e != nil {
			t.Fatal(e)
		}
	}
	if e := child.enqueue(endpointPrepared, endpointTestPrepared(t)); e != nil {
		t.Fatal(e)
	}
	transfer(child, parent)
	if e := parent.enqueue(endpointAdmission, endpointTestAdmission(t)); e != nil {
		t.Fatal(e)
	}
	transfer(parent, child)
	return parent, child
}
func TestEndpointFlowStartMemory(t *testing.T) {
	admission, e := decodeEndpointAdmission(endpointTestAdmission(t))
	if e != nil {
		t.Fatal(e)
	}
	body := endpointFlowBody{BridgeGeneration: admission.BridgeGeneration, PairDigest: admission.BridgePairDigest}
	payload, e := body.encode()
	if e != nil {
		t.Fatal(e)
	}
	round, e := decodeEndpointFlow(payload)
	if e != nil || round != body || !round.matches(admission) {
		t.Fatal("flow data roundtrip", e)
	}
	for _, name := range []string{"valid", "pair-generation", "pair-digest", "before-admission", "wrong-role", "replay"} {
		t.Run(name, func(t *testing.T) {
			parent, child := flowMemoryPair(t)
			f := endpointFrame{kind: endpointFlowStart, direction: 1, generation: 7, sequence: 2, payload: append([]byte(nil), payload...)}
			switch name {
			case "pair-generation":
				binary.BigEndian.PutUint64(f.payload, 3)
			case "pair-digest":
				f.payload[8] ^= 1
			case "before-admission":
				child, _ = newEndpointFrameState(7, 1, packetRig(t))
				f.sequence = 1
			case "wrong-role":
				f.direction = 2
				child = parent
			}
			_, e := child.receive(f)
			if name == "valid" || name == "replay" {
				if e != nil || !child.flowStarted {
					t.Fatal("flow ordering", e)
				}
			} else if e == nil || !child.stopped {
				t.Fatal("invalid flow ordering")
			}
			if name == "replay" {
				f.sequence++
				if _, e = child.receive(f); e == nil {
					t.Fatal("flow replay")
				}
			}
		})
	}
	for _, direction := range []string{"receive", "enqueue"} {
		t.Run("no-flow-data-"+direction, func(t *testing.T) {
			_, child := flowMemoryPair(t)
			if direction == "receive" {
				if _, e := child.receive(endpointFrame{kind: endpointDatagram, direction: 1, generation: 7, sequence: 2, payload: testIP(17, testUDP(8), true, 0)}); e == nil || !child.stopped {
					t.Fatal("pre-flow incoming data")
				}
			} else if e := child.enqueue(endpointDatagram, testIP(17, testUDP(8), false, 0)); e == nil {
				t.Fatal("pre-flow outgoing data")
			}
		})
	}
	for _, n := range []int{0, 39, 41} {
		if _, e = decodeEndpointFlow(make([]byte, n)); e == nil {
			t.Fatal("bad length")
		}
	}
	bad := append([]byte(nil), payload...)
	clear(bad[:8])
	if _, e = decodeEndpointFlow(bad); e == nil {
		t.Fatal("zero generation")
	}
	bad = append([]byte(nil), payload...)
	clear(bad[8:])
	if _, e = decodeEndpointFlow(bad); e == nil {
		t.Fatal("zero digest")
	}
	parent, _ := flowMemoryPair(t)
	if e = parent.enqueue(endpointFlowStart, payload); e != nil || !parent.flowStarted {
		t.Fatal("outgoing flow ordering", e)
	}
}
func TestEndpointPolicyDataStillUnresolvedMemory(t *testing.T) {
	link, sys := fullAFSynthetic()
	link.MTU = MTU
	link.Flags = 0x110d1
	family, e := decodeAttributes(link.Raw[26])
	if e != nil {
		t.Fatal(e)
	}
	v6, e := decodeAttributes(family[10])
	if e != nil {
		t.Fatal(e)
	}
	binary.NativeEndian.PutUint32(v6[2][8:], MTU)
	family[10] = encodeIPv6PolicyTestAttributes(v6)
	link.Raw[26] = encodeIPv6PolicyTestAttributes(family)
	for _, p := range effectiveV4Paths() {
		sys[p] = "0"
	}
	policy, e := assessOwnedTUNPolicy(link)
	if e != nil || len(policy.Words) != 93 || policy.Unresolved != 93 {
		t.Fatal("metadata resolved original policy", e)
	}
	for _, w := range policy.Words {
		if w.Resolved {
			t.Fatal("source-free resolved word")
		}
	}
	effective, e := parseEffectiveV4(sys)
	if e != nil {
		t.Fatal(e)
	}
	consumers, e := assessIPv4ConsumerOperands(link, effective)
	if e != nil || len(consumers) != 33 {
		t.Fatal("consumer shape", e)
	}
	for _, c := range consumers {
		if c.Resolved {
			t.Fatal("source-free resolved consumer")
		}
	}
	// Matching-looking flags/MTU/disable metadata still provides no private source
	// pass, before/after original readback, two-phase channel or completion.
}
