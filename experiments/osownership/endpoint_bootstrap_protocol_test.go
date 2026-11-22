//go:build mnet_enrollment_development

package osownership

import "testing"

func TestEndpointBootstrapTypedStartStopStats(t *testing.T) {
	start := endpointStartBody{Prepared: endpointTestBody(), Name: "mno-test", Alias: "mno-0123456789abcdef0123456789abcdef"}
	p, e := start.encode()
	if e != nil {
		t.Fatal(e)
	}
	got, e := decodeEndpointStart(p)
	if e != nil || got != start {
		t.Fatal("start roundtrip", e)
	}
	bad := append([]byte(nil), p...)
	bad[159] = 1
	if _, e = decodeEndpointStart(bad); e == nil {
		t.Fatal("noncanonical name padding accepted")
	}
	bad = append([]byte(nil), p...)
	bad[164] = 'z'
	if _, e = decodeEndpointStart(bad); e == nil {
		t.Fatal("bad owner alias accepted")
	}
	if e = validateEndpointStop(endpointStopPayload(endpointReasonRequested)); e != nil {
		t.Fatal(e)
	}
	if e = validateEndpointStop(nil); e == nil {
		t.Fatal("untyped stop accepted")
	}
	if e = validateEndpointStop(endpointStopPayload(99)); e == nil {
		t.Fatal("unknown stop reason")
	}
	stats := endpointStatsBody{Reason: endpointReasonPolicyIncomplete, Counts: endpointLoopCounts{Turns: 1, Read: 2, Written: 3, PolicyDrops: 4, KernelDrops: 5, QueueDrops: 6, Yields: 7, Fatal: 8}, CleanupVerified: true}
	p, e = stats.encode()
	if e != nil {
		t.Fatal(e)
	}
	round, e := decodeEndpointStats(p)
	if e != nil || round != stats {
		t.Fatal("stats roundtrip", e)
	}
	for _, i := range []int{0, 5, 72, 79} {
		bad = append([]byte(nil), p...)
		bad[i] ^= 1
		if _, e = decodeEndpointStats(bad); e == nil {
			t.Fatalf("stats reserved/version %d", i)
		}
	}
	bad = append([]byte(nil), p...)
	bad[4] = 2
	if _, e = decodeEndpointStats(bad); e == nil {
		t.Fatal("unknown stats flag")
	}
	// A cleanup acknowledgement is historical DATA, never a prepared/admission
	// body and never a way to change the packet state into admitted mode.
	state, _ := newEndpointFrameState(7, 2, packetRig(t))
	f := endpointFrame{kind: endpointStopped, direction: 2, generation: 7, sequence: 1, payload: p}
	if _, e = state.receive(f); e == nil || state.admission || !state.stopped {
		t.Fatal("stats revived lane")
	}
}
func TestEndpointStreamSmallChunksRemainBounded(t *testing.T) {
	var stream []byte
	for i := uint64(1); i <= 51; i++ {
		n := 1
		if i == 1 {
			n = MTU
		}
		p, e := encodeEndpointFrame(endpointFrame{kind: endpointDatagram, direction: 2, generation: 7, sequence: i, payload: make([]byte, n)})
		if e != nil {
			t.Fatal(e)
		}
		stream = append(stream, p...)
	}
	decoder := &endpointFrameDecoder{}
	total := 0
	for len(stream) > 0 {
		n := 512
		if n > len(stream) {
			n = len(stream)
		}
		frames, e := decoder.feed(stream[:n])
		if e != nil {
			t.Fatal(e)
		}
		if len(frames) > endpointQueue {
			t.Fatal("unbounded decode result")
		}
		for _, f := range frames {
			total++
			if f.sequence != uint64(total) {
				t.Fatal("sequence reordered")
			}
		}
		stream = stream[n:]
	}
	priority, terminal := endpointTerminalFirst([]endpointFrame{{kind: endpointDatagram}, {kind: endpointStop}, {kind: endpointDatagram}})
	if !terminal || len(priority) != 1 || priority[0].kind != endpointStop {
		t.Fatal("decoded stop lost behind data")
	}
	if total != 51 || len(decoder.pending) != 0 {
		t.Fatal("lost coalesced/fragmented frames", total)
	}
}
