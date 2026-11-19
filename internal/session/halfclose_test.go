package session

import (
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

func TestHalfCloseRetainsBoundedOwnerAndLateResetWins(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	incoming := make(chan Stream, maxActiveStreams)
	host.OnStream(func(s Stream) { incoming <- s })
	var received []Stream
	for i := 0; i < maxActiveStreams; i++ {
		s, err := client.Open(PriBulk, json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		r := <-incoming
		received = append(received, r)
		if err = client.send(s.ID, msgFin, PriBulk, nil); err != nil {
			t.Fatal(err)
		}
		select {
		case <-r.state.peerFIN:
		case <-time.After(time.Second):
			t.Fatal("FIN missing")
		}
		if _, err = r.Recv(time.Second); !errors.Is(err, io.EOF) {
			t.Fatal("FIN not EOF", err)
		}
		// Duplicate FIN and post-FIN bytes must not close/send a closed queue twice.
		if err = client.send(s.ID, msgFin, PriBulk, nil); err != nil {
			t.Fatal(err)
		}
		if err = client.SendBytes(s.ID, PriBulk, []byte("ignored")); err != nil {
			t.Fatal(err)
		}
		s.release() // peer local ownership does not retire host finalization
	}
	host.streamMu.Lock()
	count := len(host.streams)
	host.streamMu.Unlock()
	if count != maxActiveStreams {
		t.Fatal("half-closed table escaped owner bound", count)
	}
	if _, err := host.Open(PriBulk, nil); err == nil {
		t.Fatal("half-closed ownership not counted toward cap")
	}
	if host.inboundBytes.Load() != 0 {
		t.Fatal("post-FIN DATA charged")
	}
	first := received[0]
	if !client.tryReset(first.ID) {
		t.Fatal("late reset not sent")
	}
	select {
	case <-first.state.peerReset:
	case <-time.After(time.Second):
		t.Fatal("late reset lost")
	}
	if _, err := first.Recv(time.Second); !errors.Is(err, ErrStreamReset) {
		t.Fatal("reset lost to prior EOF", err)
	}
	for _, r := range received {
		r.release()
	}
	host.streamMu.Lock()
	count = len(host.streams)
	host.streamMu.Unlock()
	if count != 0 || host.inboundBytes.Load() != 0 {
		t.Fatal("release leaked half-close state", count)
	}
}

func TestPeerResetDoesNotWaitForFullInboundQueue(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	incoming := make(chan Stream, 2)
	host.OnStream(func(s Stream) { incoming <- s })
	sent, err := client.Open(PriBulk, nil)
	if err != nil {
		t.Fatal(err)
	}
	received := <-incoming
	defer received.release()
	defer sent.release()
	for i := 0; i < cap(received.in); i++ {
		if err := client.SendBytes(sent.ID, PriBulk, []byte("queued")); err != nil {
			t.Fatal(err)
		}
	}
	if !client.tryReset(sent.ID) {
		t.Fatal("reset not queued")
	}
	select {
	case <-received.state.peerReset:
	case <-time.After(time.Second):
		t.Fatal("reset blocked behind queued DATA")
	}
	if _, err := received.Recv(time.Second); !errors.Is(err, ErrStreamReset) {
		t.Fatal("queued DATA beat known reset", err)
	}
	other, err := client.Open(PriChat, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.release()
	select {
	case r := <-incoming:
		r.release()
	case <-time.After(time.Second):
		t.Fatal("reset stalled unrelated dispatch")
	}
	received.release()
	if host.inboundBytes.Load() != 0 {
		t.Fatal("reset queue budget leaked")
	}
}
