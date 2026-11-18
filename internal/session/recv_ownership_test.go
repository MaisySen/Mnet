package session

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func ownedRecvFixture(t *testing.T) (*Mux, Stream) {
	t.Helper()
	a, b := net.Pipe()
	m := newMux(a, StreamFile)
	t.Cleanup(func() { m.Close(); b.Close() })
	st := &streamState{ch: make(chan Frame, 16), done: make(chan struct{})}
	m.streams[StreamFile] = st
	return m, Stream{Mux: m, ID: StreamFile, in: st.ch, state: st}
}

func TestRecvTerminalWinsQueuedWork(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		for _, end := range []string{"mux", "release", "release-after-fin"} {
			for _, frameType := range []byte{msgData, msgRst, msgFin} {
				m, s := ownedRecvFixture(t)
				if frameType == msgFin {
					close(s.in)
				} else {
					s.in <- Frame{Type: frameType, Payload: []byte("not-for-dispatch")}
				}
				switch end {
				case "mux":
					m.Close()
				case "release":
					s.release()
				case "release-after-fin":
					// FIN removes the table entry before the owner releases its handle.
					m.streamMu.Lock()
					delete(m.streams, s.ID)
					m.streamMu.Unlock()
					s.release()
					s.release()
				}
				if _, err := s.Recv(timeout); !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("%s type%d timeout%v returned %v", end, frameType, timeout, err)
				}
			}
		}
	}
}

func TestRecvLocalReleaseWakesWaiter(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Hour} {
		m, s := ownedRecvFixture(t)
		entered, done := make(chan struct{}), make(chan error, 1)
		go func() { close(entered); _, err := s.Recv(timeout); done <- err }()
		<-entered
		s.release()
		select {
		case err := <-done:
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("local release retained Recv waiter")
		}
		select {
		case <-m.Closed():
			t.Fatal("local release closed unrelated session")
		default:
		}
	}
}

func TestRecvLiveStreamDataFINAndReset(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		_, s := ownedRecvFixture(t)
		s.in <- Frame{Type: msgData, Payload: []byte("live")}
		s.in <- Frame{Type: msgRst}
		close(s.in)
		f, err := s.Recv(timeout)
		if err != nil || string(f.Payload) != "live" {
			t.Fatal("live DATA lost", f, err)
		}
		if _, err = s.Recv(timeout); !errors.Is(err, ErrStreamReset) {
			t.Fatal("reset lost", err)
		}
		if _, err = s.Recv(timeout); !errors.Is(err, io.EOF) {
			t.Fatal("FIN lost", err)
		}
	}
}

func TestRecvCloseAfterSelectionRefusesLateFrame(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		m, s := ownedRecvFixture(t)
		s.in = make(chan Frame)
		s.state.ch = s.in
		s.state.accountMu.Lock()
		s.state.queuedBytes = 1
		m.inboundBytes.Store(1)
		done := make(chan error, 1)
		go func() { _, err := s.Recv(timeout); done <- err }()
		// Unbuffered handoff proves Recv selected DATA, then blocks returning budget.
		s.in <- Frame{Type: msgData, Payload: []byte{1}, queueOwner: s.state, queueCost: 1}
		closed := make(chan struct{})
		go func() { m.Close(); close(closed) }()
		<-m.Closed()
		s.state.accountMu.Unlock()
		<-closed
		select {
		case err := <-done:
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal("late frame dispatched", err)
			}
		case <-time.After(time.Second):
			t.Fatal("selected frame did not retire")
		}
		if m.inboundBytes.Load() != 0 {
			t.Fatal("discarded frame retained budget")
		}
	}
}
