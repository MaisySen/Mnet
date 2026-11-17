package session

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// FuzzFrameDecoder exercises untrusted session bytes without network, files,
// credentials or an account. Successful decoding must preserve the exact frame
// prefix on re-encoding, including payload, and never exceed the protocol cap.
func FuzzFrameDecoder(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 1, msgData, PriScreen, 0, 0, 0, 0, 0, 0})
	for _, frame := range []Frame{
		{StreamID: StreamChat, Type: msgData, Priority: PriChat, Payload: []byte("fixture")},
		{StreamID: StreamFile, Type: msgFin, Priority: PriBulk},
		{StreamID: StreamControl, Type: msgPing, Priority: PriInteractive, Payload: make([]byte, 8)},
	} {
		var wire bytes.Buffer
		if err := writeFrame(&wire, frame); err != nil {
			f.Fatal(err)
		}
		f.Add(wire.Bytes())
	}
	invalid := make([]byte, headerLen)
	invalid[4], invalid[5] = msgData, PriBulk
	binary.BigEndian.PutUint32(invalid[8:], maxPayload+1)
	f.Add(invalid)
	f.Fuzz(func(t *testing.T, wire []byte) {
		input := bytes.NewReader(wire)
		frame, err := readFrame(input)
		if err != nil {
			return
		}
		if len(frame.Payload) > maxPayload {
			t.Fatal("decoded payload exceeds cap")
		}
		var output bytes.Buffer
		if err := writeFrame(&output, frame); err != nil {
			t.Fatalf("decoded frame cannot be encoded: %v", err)
		}
		consumed := len(wire) - input.Len()
		if !bytes.Equal(output.Bytes(), wire[:consumed]) {
			t.Fatal("frame changed after decode/encode")
		}
	})
}
