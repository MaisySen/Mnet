//go:build mnet_enrollment_development

package osownership

import (
	"errors"
	"testing"
)

func TestEndpointInboxStopOverflowAndEOF(t *testing.T) {
	frame := endpointFrame{kind: endpointDatagram, direction: 1, sequence: 1, generation: 7, payload: []byte{1}}
	q := &endpointBridgeInbox{}
	for i := 0; i < endpointQueue; i++ {
		if e := q.push(frame); e != nil {
			t.Fatal(e)
		}
	}
	if e := q.push(frame); e != errEndpointBackpressure {
		t.Fatal("overflow not terminal")
	}
	if _, ok, e := q.take(); ok || e != errEndpointBackpressure {
		t.Fatal("overflow retained datagrams")
	}
	q = &endpointBridgeInbox{}
	if e := q.push(frame); e != nil {
		t.Fatal(e)
	}
	stop := endpointFrame{kind: endpointStop, direction: 1, sequence: 2, generation: 7}
	if e := q.push(stop); e != nil {
		t.Fatal(e)
	}
	f, ok, e := q.take()
	if !ok || e != nil || f.kind != endpointStop {
		t.Fatal("queued stop lost")
	}
	lane := &endpointFakeLane{}
	loop := &endpointResidentLoop{}
	if e = loop.turnInbox(lane, q); !errors.Is(e, errEndpointStopped) || lane.readCalls != 0 || lane.writeCalls != 0 || lane.stopCalls != 1 {
		t.Fatal("inbox stop allowed I/O", e)
	}
	q = &endpointBridgeInbox{}
	_ = q.push(frame)
	eof := errors.New("fake channel EOF")
	q.end(eof)
	lane = &endpointFakeLane{}
	loop = &endpointResidentLoop{}
	if e = loop.turnInbox(lane, q); !errors.Is(e, eof) || lane.readCalls != 0 || lane.writeCalls != 0 || lane.stopCalls != 1 {
		t.Fatal("EOF allowed queued I/O", e)
	}
	q = &endpointBridgeInbox{}
	bad := frame
	bad.payload = make([]byte, MTU+1)
	if e = q.push(bad); e == nil || q.halted() == nil {
		t.Fatal("unbounded frame accepted")
	}
}
