package session

import (
	"bytes"
	"net"
	"testing"
)

func TestQueuedPayloadIsOwned(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	m := newMux(a, StreamFile)
	// Keep the writer stopped to test queue ownership deterministically.
	m.started = true
	payload := []byte("first chunk")
	if err := m.SendBytes(StreamFile, PriBulk, payload); err != nil {
		t.Fatal(err)
	}
	copy(payload, []byte("other chunk"))
	queued := m.pending[PriBulk]
	if !bytes.Equal(queued[len(queued)-1].Payload, []byte("first chunk")) {
		t.Fatal("queued payload aliases caller buffer")
	}
}
