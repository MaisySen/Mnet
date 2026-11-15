package session

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"
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

func TestQueueBackpressureUnblocksOnClose(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	m := newMux(a, StreamFile)
	defer m.Close()
	m.started = true
	m.opened[StreamFile] = true
	payload := make([]byte, maxPayload)
	for i := 0; i < maxQueuedBytes/(maxPayload+headerLen); i++ {
		if err := m.SendBytes(StreamFile, PriBulk, payload); err != nil {
			t.Fatal(err)
		}
	}
	finished := make(chan error, 1)
	go func() { finished <- m.SendBytes(StreamFile, PriBulk, payload) }()
	select {
	case err := <-finished:
		t.Fatalf("send exceeded queue cap: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	m.Close()
	select {
	case err := <-finished:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("send after close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked enqueue did not wake")
	}
}

func TestBulkCannotStarve(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	m := newMux(a, StreamFile)
	m.pending[PriBulk] = []Frame{{Type: msgFin, Priority: PriBulk}}
	for i := 0; i < len(drainOrder); i++ {
		// Keep screen traffic permanently available.
		m.pending[PriScreen] = append(m.pending[PriScreen], Frame{Priority: PriScreen})
		f, ok := m.popLocked()
		if ok && f.Type == msgFin {
			return
		}
	}
	t.Fatal("bulk completion starved by screen traffic")
}
