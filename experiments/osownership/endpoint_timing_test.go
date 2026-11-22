//go:build mnet_enrollment_development

package osownership

import (
	"testing"
	"time"
)

// Memory-only codec/state exercise. No timers, contexts, native owners,
// controllers, descriptors, factories, IPC, source collectors or workers.
func TestEndpointTimingAndCheckedStartMemory(t *testing.T) {
	if endpointHandshakeBudget != 45*time.Second || endpointCleanupBudget != 500*time.Millisecond || endpointFinalStatsBudget != 500*time.Millisecond || endpointGraceBudget < endpointCleanupBudget+endpointFinalStatsBudget || endpointIdleSafetyBudget > 50*time.Millisecond {
		t.Fatal("timing contract")
	}
	start := endpointStartBody{Prepared: endpointTestBody(), Name: "mno-test", Alias: "mno-0123456789abcdef0123456789abcdef"}
	payload, e := start.encode()
	if e != nil {
		t.Fatal(e)
	}
	frame := endpointFrame{kind: endpointStart, direction: 1, generation: 7, sequence: 1, payload: payload}
	for _, name := range []string{"valid", "wrong-role", "wrong-generation", "wrong-sequence", "wrong-direction", "malformed-body", "replay"} {
		t.Run(name, func(t *testing.T) {
			direction := byte(1)
			if name == "wrong-role" {
				direction = 2
			}
			state, e := newEndpointFrameState(7, direction, packetRig(t))
			if e != nil {
				t.Fatal(e)
			}
			f := frame
			switch name {
			case "wrong-generation":
				f.generation = 8
			case "wrong-sequence":
				f.sequence = 2
			case "wrong-direction":
				f.direction = 2
			case "malformed-body":
				f.payload = nil
			}
			e = state.consumeStart(f)
			if name == "valid" || name == "replay" {
				if e != nil || state.received != 1 || state.prepared || state.admission || state.datagramRoom() {
					t.Fatal("START accounting widened authority", e)
				}
			} else {
				if e == nil || !state.stopped {
					t.Fatal("bad START accepted")
				}
			}
			if name == "replay" && state.consumeStart(f) == nil {
				t.Fatal("START replay")
			}
		})
	}
	if e = validateEndpointStop(endpointStopPayload(endpointReasonTimeout)); e != nil {
		t.Fatal(e)
	}
	stats := endpointStatsBody{Reason: endpointReasonTimeout, CleanupVerified: true}
	p, e := stats.encode()
	if e != nil {
		t.Fatal(e)
	}
	got, e := decodeEndpointStats(p)
	if e != nil || got != stats {
		t.Fatal("timeout stats", e)
	}
}
func TestEndpointBridgeAdmissionDataMemory(t *testing.T) {
	p := endpointTestAdmission(t)
	b, e := decodeEndpointAdmission(p)
	if e != nil || b.BridgeGeneration != 2 || b.BridgePairDigest != ([32]byte{5}) || b.Prepared != endpointTestBody() {
		t.Fatal("bridge correlation roundtrip", e)
	}
	if len(p) != 264 {
		t.Fatal("body size")
	}
	for _, name := range []string{"old-body", "zero-generation", "unconfigured-generation", "zero-digest"} {
		t.Run(name, func(t *testing.T) {
			bad := append([]byte(nil), p...)
			switch name {
			case "old-body":
				bad = bad[:224]
			case "zero-generation":
				clear(bad[224:232])
			case "unconfigured-generation":
				clear(bad[224:232])
				bad[231] = 1
			case "zero-digest":
				clear(bad[232:264])
			}
			if _, e := decodeEndpointAdmission(bad); e == nil {
				t.Fatal("invalid bridge claim accepted")
			}
		})
	}
	// Nonzero wire metadata remains DATA. These helpers produce no private
	// daemon bridge pointer, live readback, completion receipt or Ready state.
}
