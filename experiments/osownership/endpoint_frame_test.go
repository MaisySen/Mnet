//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"testing"
)

func TestEndpointFrameCodecStateBoundaries(t *testing.T) {
	prepared := endpointTestPrepared(t)
	raw, err := encodeEndpointFrame(endpointFrame{kind: endpointPrepared, direction: 2, generation: 7, sequence: 1, payload: prepared})
	if err != nil {
		t.Fatal(err)
	}
	d := &endpointFrameDecoder{}
	if frames, err := d.feed(raw[:9]); err != nil || len(frames) != 0 {
		t.Fatal("partial header")
	}
	frames, err := d.feed(raw[9:])
	if err != nil || len(frames) != 1 {
		t.Fatal("bounded split frame", err)
	}
	state, err := newEndpointFrameState(7, 2, packetRig(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.receive(frames[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = state.receive(frames[0]); err == nil {
		t.Fatal("replay accepted")
	}
	if err = state.enqueue(endpointAdmission, nil); err == nil {
		t.Fatal("stopped lane revived")
	}
	bad := append([]byte(nil), raw...)
	binary.BigEndian.PutUint32(bad[24:28], ^uint32(0))
	if _, err = decodeEndpointFrame(bad); err == nil {
		t.Fatal("oversize accepted")
	}
	partial := &endpointFrameDecoder{}
	_, _ = partial.feed(raw[:3])
	if partial.eof() == nil {
		t.Fatal("partial EOF accepted")
	}
	if _, err = partial.feed(raw); err == nil {
		t.Fatal("EOF lane revived")
	}
	writer, _ := newEndpointFrameState(7, 1, packetRig(t))
	if err = writer.enqueue(endpointPrepared, prepared); err != nil {
		t.Fatal(err)
	}
	b, err := writer.nextWrite()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.nextWrite(); err == nil {
		t.Fatal("second writer allowed")
	}
	if err = writer.finishWrite(len(b)-1, nil); err == nil {
		t.Fatal("partial bridge write accepted")
	}
}
