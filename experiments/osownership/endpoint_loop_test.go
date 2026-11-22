//go:build mnet_enrollment_development

package osownership

import (
	"errors"
	"testing"
)

func endpointTestBody() endpointPreparedBody {
	return endpointPreparedBody{PID: 12, StartTicks: 33, Namespace: Namespace{Device: 3, Inode: 4, Isolated: true}, Generation: 7, MTU: MTU, Own: [4]byte{100, 64, 0, 1}, Peer: [4]byte{100, 64, 0, 2}, OwnKey: [32]byte{1}, PeerKey: [32]byte{2}, SelfUntilMS: 10000, PeerUntilMS: 12000}
}
func endpointTestPrepared(t *testing.T) []byte {
	t.Helper()
	p, e := endpointTestBody().encode()
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func endpointTestAdmission(t *testing.T) []byte {
	t.Helper()
	b := endpointAdmissionBody{Prepared: endpointTestBody(), SelfPublication: [32]byte{3}, PeerPublication: [32]byte{4}, SelfRevision: 5, PeerRevision: 6}
	p, e := b.encode()
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestEndpointTypedControlBoundaries(t *testing.T) {
	p := endpointTestPrepared(t)
	b, e := decodeEndpointPrepared(p)
	if e != nil || b != endpointTestBody() {
		t.Fatal("prepared roundtrip", e)
	}
	for _, offset := range []int{0, 2, 132, 143} {
		bad := append([]byte(nil), p...)
		bad[offset] ^= 1
		if _, e = decodeEndpointPrepared(bad); e == nil {
			t.Fatalf("reserved/version accepted %d", offset)
		}
	}
	for _, n := range []int{0, 143, 145} {
		bad := make([]byte, n)
		copy(bad, p)
		if _, e = decodeEndpointPrepared(bad); e == nil {
			t.Fatalf("length accepted %d", n)
		}
	}
	a := endpointTestAdmission(t)
	if _, e = decodeEndpointAdmission(a); e != nil {
		t.Fatal(e)
	}
	for i := 144; i < 176; i++ {
		a[i] = 0
	}
	if _, e = decodeEndpointAdmission(a); e == nil {
		t.Fatal("zero publication accepted")
	}
	if _, e = encodeEndpointFrame(endpointFrame{kind: endpointPrepared, direction: 2, generation: 8, sequence: 1, payload: p}); e == nil {
		t.Fatal("generation mismatch accepted")
	}
}
func TestEndpointRoleQueueAndPacketDrops(t *testing.T) {
	parent, _ := newEndpointFrameState(7, 2, packetRig(t))
	child, _ := newEndpointFrameState(7, 1, packetRig(t))
	prepared := endpointTestPrepared(t)
	admission := endpointTestAdmission(t)
	if e := parent.enqueue(endpointPrepared, prepared); e == nil {
		t.Fatal("wrong prepared role")
	}
	if e := child.enqueue(endpointAdmission, admission); e == nil {
		t.Fatal("wrong admission role")
	}
	if e := child.enqueue(endpointPrepared, prepared); e != nil {
		t.Fatal(e)
	}
	raw, e := child.nextWrite()
	if e != nil {
		t.Fatal(e)
	}
	f, e := decodeEndpointFrame(raw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = parent.receive(f); e != nil {
		t.Fatal(e)
	}
	if e = child.finishWrite(len(raw), nil); e != nil {
		t.Fatal(e)
	}
	mismatch := append([]byte(nil), admission...)
	mismatch[123] ^= 1
	if e = parent.enqueue(endpointAdmission, mismatch); e == nil {
		t.Fatal("different admission prepared identity")
	}
	if e = parent.enqueue(endpointAdmission, admission); e != nil {
		t.Fatal(e)
	}
	raw, e = parent.nextWrite()
	if e != nil {
		t.Fatal(e)
	}
	f, e = decodeEndpointFrame(raw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = child.receive(f); e != nil {
		t.Fatal(e)
	}
	if e = parent.finishWrite(len(raw), nil); e != nil {
		t.Fatal(e)
	}
	// Invalid packet consumes its frame sequence but drops only that packet. The
	// following valid sequence must still work, not become a protocol shutdown.
	if _, e = child.receive(endpointFrame{kind: endpointDatagram, direction: 1, generation: 7, sequence: 2, payload: []byte{1}}); e == nil {
		t.Fatal("bad packet accepted")
	}
	valid := testIP(17, testUDP(8), true, 0)
	if _, e = child.receive(endpointFrame{kind: endpointDatagram, direction: 1, generation: 7, sequence: 3, payload: valid}); e != nil {
		t.Fatal("packet drop killed protocol", e)
	}
	outbound := testIP(17, testUDP(8), false, 0)
	for i := 0; i < endpointQueue; i++ {
		if e = child.enqueue(endpointDatagram, outbound); e != nil {
			t.Fatal("queue early", i, e)
		}
	}
	seq := child.sent
	if e = child.enqueue(endpointDatagram, outbound); e != errEndpointBackpressure || child.sent != seq {
		t.Fatal("full queue advanced sequence")
	}
	child.stop()
	if _, e = child.nextWrite(); e == nil {
		t.Fatal("stop retained data")
	}
}

type endpointFakeLane struct {
	readCalls, writeCalls, stopCalls int
	readErr, writeErr, stopErr       error
}

func (f *endpointFakeLane) drainOne() error                { f.readCalls++; return f.readErr }
func (f *endpointFakeLane) receiveOne(endpointFrame) error { f.writeCalls++; return f.writeErr }
func (f *endpointFakeLane) stopOwned() error               { f.stopCalls++; return f.stopErr }
func TestEndpointLoopBoundedStopAndErrors(t *testing.T) {
	f := &endpointFakeLane{readErr: errEndpointNoProgress, writeErr: packetHeader}
	l := &endpointResidentLoop{}
	frame := endpointFrame{kind: endpointDatagram, direction: 1, generation: 7, sequence: 1, payload: []byte{1}}
	if e := l.offer(frame); e != nil {
		t.Fatal(e)
	}
	if e := l.turn(f); e != nil {
		t.Fatal(e)
	}
	if f.writeCalls != 1 || f.readCalls != 1 || f.stopCalls != 0 || l.counts.PolicyDrops != 1 || l.counts.Yields != 1 {
		t.Fatal("drop/yield or bounded turn", l.counts)
	}
	for i := 0; i < endpointQueue; i++ {
		if e := l.offer(frame); e != nil {
			t.Fatal(e)
		}
	}
	if e := l.offer(frame); e != errEndpointBackpressure {
		t.Fatal("inbound bound")
	}
	if e := l.offer(endpointFrame{kind: endpointStop, direction: 1, generation: 7, sequence: 2, payload: endpointStopPayload(endpointReasonRequested)}); e != nil {
		t.Fatal(e)
	}
	if e := l.turn(f); !errors.Is(e, errEndpointStopped) {
		t.Fatal(e)
	}
	if f.stopCalls != 1 || f.writeCalls != 1 || f.readCalls != 1 {
		t.Fatal("stop lost to queued I/O")
	}
	if e := l.turn(f); !errors.Is(e, errEndpointStopped) || f.stopCalls != 1 {
		t.Fatal("stop repeated")
	}
	fatal := errors.New("fake custody loss")
	cleanup := errors.New("fake cleanup unresolved")
	f = &endpointFakeLane{readErr: fatal, stopErr: cleanup}
	l = &endpointResidentLoop{}
	e := l.turn(f)
	if !errors.Is(e, fatal) || !errors.Is(e, cleanup) || f.stopCalls != 1 || l.counts.Fatal != 1 {
		t.Fatal("fatal cleanup discarded", e)
	}
	f = &endpointFakeLane{}
	l = &endpointResidentLoop{}
	if e = l.end(f, errEndpointFrame); !errors.Is(e, errEndpointFrame) || f.stopCalls != 1 {
		t.Fatal("EOF no stop")
	}
	f = &endpointFakeLane{readErr: errEndpointNoProgress, writeErr: errEndpointPacketDrop}
	l = &endpointResidentLoop{}
	_ = l.offer(frame)
	if e = l.turn(f); e != nil || l.counts.KernelDrops != 1 || f.stopCalls != 0 {
		t.Fatal("kernel packet drop became fatal")
	}
}
