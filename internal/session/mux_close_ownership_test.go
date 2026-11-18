package session

import (
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type countedClosedWriter struct {
	net.Conn
	writes atomic.Int32
}

func (c *countedClosedWriter) Write(p []byte) (int, error) { c.writes.Add(1); return c.Conn.Write(p) }

func TestMuxCloseDropsQueuedOutboundOwnership(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	conn := &countedClosedWriter{Conn: a}
	m := newMux(conn, StreamFile)
	defer m.Close()
	m.started = true
	m.opened[StreamFile] = true
	payload := make([]byte, maxPayload)
	for i := 0; i < maxQueuedBytes/(maxPayload+headerLen); i++ {
		if err := m.SendBytes(StreamFile, PriBulk, payload); err != nil {
			t.Fatal(err)
		}
	}
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() { close(entered); done <- m.SendBytes(StreamFile, PriBulk, payload) }()
	<-entered
	m.Close()
	m.Close()
	select {
	case err := <-done:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal("producer not retired", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued producer did not wake")
	}
	m.wmu.Lock()
	retained := m.queuedBytes + len(m.opened)
	for _, queue := range m.pending {
		retained += len(queue) + cap(queue)
	}
	m.wmu.Unlock()
	if retained != 0 {
		t.Fatal("terminal outbound queue retained buffers/markers", retained)
	}
	writerDone := make(chan struct{})
	go func() { m.writeLoop(); close(writerDone) }()
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("terminal writer retained")
	}
	if conn.writes.Load() != 0 {
		t.Fatal("terminal writer dispatched queued frames", conn.writes.Load())
	}
	if err := m.SendBytes(StreamFile, PriBulk, payload); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("closed send accepted", err)
	}
}

func TestMuxCloseConcurrentOpenCannotRestoreMarkers(t *testing.T) {
	for i := 0; i < 100; i++ {
		a, b := net.Pipe()
		m := newMux(a, StreamFile)
		m.started = true
		done := make(chan struct{})
		go func() { defer close(done); _, _ = m.Open(PriBulk, nil) }()
		m.Close()
		<-done
		m.wmu.Lock()
		retained := len(m.opened) + m.queuedBytes
		for _, queue := range m.pending {
			retained += len(queue)
		}
		m.wmu.Unlock()
		b.Close()
		if retained != 0 {
			t.Fatal("racing open restored terminal outbound state", retained)
		}
	}
}
