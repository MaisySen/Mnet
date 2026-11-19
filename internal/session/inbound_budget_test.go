package session

import (
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

func TestInboundBudgetAcrossIndependentStreamQueues(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	incoming := make(chan Stream, 4)
	host.OnStream(func(s Stream) { incoming <- s })
	streams := make([]Stream, 0, 3)
	for i := 0; i < 3; i++ {
		_, err := client.Open(PriBulk, json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		select {
		case received := <-incoming:
			streams = append(streams, received)
		case <-time.After(time.Second):
			t.Fatal("open missing")
		}
	}
	payload := make([]byte, maxPayload)
	// No per-stream queue reaches its16frame limit. The aggregate must close.
	for i := 0; i < 17; i++ {
		if err := client.SendBytes(streams[i%3].ID, PriBulk, payload); err != nil {
			break
		}
	}
	select {
	case <-host.Closed():
	case <-time.After(3 * time.Second):
		t.Fatal("aggregate inbound payload unbounded")
	}
	if host.inboundBytes.Load() > maxInboundBytes {
		t.Fatal("overflow charge retained")
	}
}

func TestInboundBudgetReturnedOnConsumeAndRelease(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		client, host := pipe(t)
		incoming := make(chan Stream, 1)
		host.OnStream(func(s Stream) { incoming <- s })
		sent, err := client.Open(PriBulk, json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		received := <-incoming
		payload := make([]byte, 1024)
		if err := client.SendBytes(sent.ID, PriBulk, payload); err != nil {
			t.Fatal(err)
		}
		if _, err := received.Recv(timeout); err != nil {
			t.Fatal(err)
		}
		if host.inboundBytes.Load() != 0 {
			t.Fatal("consume kept budget")
		}
		for i := 0; i < 8; i++ {
			if err := client.SendBytes(sent.ID, PriBulk, payload); err != nil {
				t.Fatal(err)
			}
		}
		deadline := time.Now().Add(time.Second)
		for host.inboundBytes.Load() != 8192 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if host.inboundBytes.Load() != 8192 {
			t.Fatal("frames not charged")
		}
		host.releaseStream(received.ID)
		if host.inboundBytes.Load() != 0 {
			t.Fatal("released stream kept budget")
		}
		// A consumer racing local release must not decrement a second time.
		if _, err := received.Recv(time.Second); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal("released stream delivered queued work", err)
		}
		if host.inboundBytes.Load() != 0 {
			t.Fatal("released frame double charged")
		}
		client.Close()
		host.Close()
	}
}

func TestInboundBudgetReleaseAfterFINAndConcurrentDrain(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	incoming := make(chan Stream, 1)
	host.OnStream(func(s Stream) { incoming <- s })
	sent, err := client.Open(PriBulk, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	received := <-incoming
	for i := 0; i < 8; i++ {
		if err := client.SendBytes(sent.ID, PriBulk, make([]byte, 1024)); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.send(sent.ID, msgFin, PriBulk, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received.state.peerFIN:
	case <-time.After(time.Second):
		t.Fatal("FIN not received")
	}
	host.streamMu.Lock()
	retained := host.streams[sent.ID] == received.state
	host.streamMu.Unlock()
	if !retained {
		t.Fatal("FIN retired owner before release")
	}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			if _, err := received.Recv(time.Second); err != nil {
				return
			}
		}
	}()
	received.release()
	received.release()
	<-drained
	if host.inboundBytes.Load() != 0 {
		t.Fatal("FIN/release/drain leaked or double returned budget")
	}
}

func TestInboundBudgetIsNotLifetimeTransferQuota(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	incoming := make(chan Stream, 1)
	host.OnStream(func(s Stream) { incoming <- s })
	sent, err := client.Open(PriBulk, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	received := <-incoming
	payload := make([]byte, maxPayload)
	for i := 0; i < 32; i++ {
		if err := client.SendBytes(sent.ID, PriBulk, payload); err != nil {
			t.Fatal(err)
		}
		if _, err := received.Recv(time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if host.inboundBytes.Load() != 0 {
		t.Fatal("drained transfer kept budget")
	}
	select {
	case <-host.Closed():
		t.Fatal("drained traffic exhausted lifetime quota")
	default:
	}
}
