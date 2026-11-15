// Package session implements Mnet's remote-desktop session layer:
// typed streams (screen, input, chat, file) multiplexed with
// priorities over a single connection between two mesh peers.
// The mesh (WireGuard) provides authentication and encryption, so
// this layer adds no second crypto stack.
package session

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	headerLen    = 12
	maxPayload   = 1 << 20 // 1 MiB
	protoVersion = 1
)

// Message types on a stream.
const (
	msgOpen byte = 1
	msgData byte = 2
	msgFin  byte = 3
	msgRst  byte = 4
	msgPing byte = 5
	msgPong byte = 6
)

// Priority classes; lower drains first.
const (
	PriInteractive = 0 // input events, session control
	PriScreen      = 1 // screen frames
	PriChat        = 2 // chat, clipboard, capabilities
	PriBulk        = 3 // file transfer
)

// Well-known stream IDs.
const (
	StreamControl uint32 = 0
	StreamScreen  uint32 = 1
	StreamInput   uint32 = 2
	StreamChat    uint32 = 3
	StreamFile    uint32 = 4 // first of the dynamic range starts here
)

// Frame is one message on one stream.
type Frame struct {
	StreamID uint32
	Type     byte
	Priority uint8
	Payload  []byte
}

func writeFrame(w io.Writer, f Frame) error {
	if len(f.Payload) > maxPayload {
		return fmt.Errorf("session: payload %d exceeds %d", len(f.Payload), maxPayload)
	}
	var h [headerLen]byte
	binary.BigEndian.PutUint32(h[0:4], f.StreamID)
	h[4] = f.Type
	h[5] = f.Priority
	binary.BigEndian.PutUint16(h[6:8], 0)
	binary.BigEndian.PutUint32(h[8:12], uint32(len(f.Payload)))
	if _, err := w.Write(h[:]); err != nil {
		return err
	}
	if len(f.Payload) > 0 {
		_, err := w.Write(f.Payload)
		return err
	}
	return nil
}

func readFrame(r io.Reader) (Frame, error) {
	var h [headerLen]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return Frame{}, err
	}
	n := binary.BigEndian.Uint32(h[8:12])
	if n > maxPayload {
		return Frame{}, fmt.Errorf("session: frame payload %d exceeds %d", n, maxPayload)
	}
	f := Frame{
		StreamID: binary.BigEndian.Uint32(h[0:4]),
		Type:     h[4],
		Priority: h[5],
	}
	if n > 0 {
		f.Payload = make([]byte, n)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return Frame{}, err
		}
	}
	return f, nil
}
