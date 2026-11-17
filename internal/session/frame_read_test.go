package session

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

func TestFrameReadRejectsUnsupportedHeaderBeforePayload(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index int
		value byte
	}{
		{"zero-type", 4, 0}, {"unknown-type", 4, 7}, {"high-type", 4, 255},
		{"unknown-priority", 5, 4}, {"high-priority", 5, 255},
		{"reserved-high", 6, 1}, {"reserved-low", 7, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := make([]byte, headerLen)
			h[4] = msgData
			h[5] = PriBulk
			h[tc.index] = tc.value
			binary.BigEndian.PutUint32(h[8:], maxPayload)
			// Deliberately absent payload: a supported header would return EOF.
			_, err := readFrame(bytes.NewReader(h))
			if err == nil || !strings.Contains(err.Error(), "unsupported frame header") {
				t.Fatalf("invalid header not rejected before payload read: %v", err)
			}
		})
	}
}

func TestFrameReadAcceptsKnownTypesAndPriorities(t *testing.T) {
	for typ := byte(msgOpen); typ <= msgPong; typ++ {
		for pri := uint8(PriInteractive); pri <= PriBulk; pri++ {
			in := Frame{StreamID: 9, Type: typ, Priority: pri, Payload: []byte("roundtrip")}
			var wire bytes.Buffer
			if err := writeFrame(&wire, in); err != nil {
				t.Fatal(err)
			}
			out, err := readFrame(&wire)
			if err != nil || out.StreamID != in.StreamID || out.Type != in.Type || out.Priority != in.Priority || !bytes.Equal(out.Payload, in.Payload) {
				t.Fatalf("type%d priority%d: %+v %v", typ, pri, out, err)
			}
		}
	}
}

func TestMalformedHeaderClosesActiveMuxWithoutPayload(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	m := newMux(a, StreamFile)
	defer m.Close()
	m.start()
	h := make([]byte, headerLen)
	h[4] = 255
	binary.BigEndian.PutUint32(h[8:], maxPayload)
	if err := b.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write(h); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.Closed():
	case <-time.After(time.Second):
		t.Fatal("malformed header retained active mux awaiting payload")
	}
}

func TestOpenMetadataCapBeforeReadingOrWritingPayload(t *testing.T) {
	h := make([]byte, headerLen)
	h[4] = msgOpen
	h[5] = PriBulk
	binary.BigEndian.PutUint32(h[8:], maxOpenPayload+1)
	if _, err := readFrame(bytes.NewReader(h)); err == nil || !strings.Contains(err.Error(), "OPEN metadata") {
		t.Fatalf("oversized OPEN read body: %v", err)
	}
	var wire bytes.Buffer
	if err := writeFrame(&wire, Frame{Type: msgOpen, Payload: make([]byte, maxOpenPayload+1)}); err == nil {
		t.Fatal("oversized local OPEN accepted")
	}
	if wire.Len() != 0 {
		t.Fatal("rejected OPEN partially written")
	}
	for _, frame := range []Frame{{Type: msgOpen, Payload: make([]byte, maxOpenPayload)}, {Type: msgData, Payload: make([]byte, maxPayload)}} {
		wire.Reset()
		if err := writeFrame(&wire, frame); err != nil {
			t.Fatal(err)
		}
		got, err := readFrame(&wire)
		if err != nil || !bytes.Equal(got.Payload, frame.Payload) {
			t.Fatalf("valid boundary rejected: %v", err)
		}
	}
}
