package session

import (
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestMuxInboundCloseRetiresActiveTable(t *testing.T) {
	m, s := ownedRecvFixture(t)
	st := s.state
	st.open = []byte(`{"fixture":"queued"}`)
	st.queuedBytes = 3
	m.inboundBytes.Store(3)
	st.ch <- Frame{Type: msgData, Payload: []byte("abc"), queueOwner: st, queueCost: 3}
	m.Close()
	m.Close()
	m.streamMu.Lock()
	retained := len(m.streams)
	m.streamMu.Unlock()
	if retained != 0 || m.inboundBytes.Load() != 0 {
		t.Fatal("closed mux retained active table/budget", retained, m.inboundBytes.Load())
	}
	select {
	case <-st.done:
	default:
		t.Fatal("table owner not retired")
	}
	// A stream retained by the caller cannot revive delivery or return budget twice.
	if _, err := s.Recv(0); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("terminal stream dispatched", err)
	}
	s.release()
	s.release()
	if m.inboundBytes.Load() != 0 {
		t.Fatal("late release double returned budget")
	}
	// Handler registration after Close must not replay queued peer OPENs.
	var callbacks atomic.Int32
	m.OnStream(func(Stream) { callbacks.Add(1) })
	m.streamMu.Lock()
	retained = len(m.streams)
	m.streamMu.Unlock()
	if retained != 0 || callbacks.Load() != 0 {
		t.Fatal("late handler revived table")
	}
}

func TestMuxInboundCloseBeforePeerDispatch(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	m := newMux(a, StreamFile)
	defer m.Close()
	var calls atomic.Int32
	m.OnStream(func(Stream) { calls.Add(1) })
	// Hold admission while a complete peer OPEN is already being read.
	m.streamMu.Lock()
	readerDone := make(chan struct{})
	go func() { m.readLoop(); close(readerDone) }()
	if err := writeFrame(b, Frame{StreamID: StreamFile + 1, Type: msgOpen, Priority: PriBulk, Payload: []byte(`{}`)}); err != nil {
		m.streamMu.Unlock()
		t.Fatal(err)
	}
	closeDone := make(chan struct{})
	go func() { m.Close(); close(closeDone) }()
	select {
	case <-m.Closed():
	case <-time.After(time.Second):
		m.streamMu.Unlock()
		t.Fatal("Close did not signal")
	}
	m.streamMu.Unlock()
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close retained")
	}
	select {
	case <-readerDone:
	case <-time.After(time.Second):
		t.Fatal("late peer OPEN retained reader")
	}
	m.streamMu.Lock()
	retained := len(m.streams)
	m.streamMu.Unlock()
	if retained != 0 || calls.Load() != 0 {
		t.Fatal("peer OPEN revived closed table", retained, calls.Load())
	}
}

func TestMuxInboundCloseConcurrentLocalAdmission(t *testing.T) {
	for i := 0; i < 100; i++ {
		a, b := net.Pipe()
		m := newMux(a, StreamFile)
		m.started = true
		done := make(chan struct{})
		go func() { defer close(done); _, _ = m.Open(PriBulk, nil) }()
		m.Close()
		<-done
		m.streamMu.Lock()
		retained := len(m.streams)
		m.streamMu.Unlock()
		b.Close()
		if retained != 0 {
			t.Fatal("local Open revived closed table", retained)
		}
	}
}
