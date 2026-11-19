package session

import (
	"net"
	"sync"
	"testing"
	"time"
)

type keepaliveBlockedWriter struct {
	net.Conn
	once    sync.Once
	entered chan struct{}
}

func (c *keepaliveBlockedWriter) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.entered) })
	return c.Conn.Write(p)
}

func fullKeepaliveQueue(t *testing.T) (*Mux, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { b.Close() })
	conn := &keepaliveBlockedWriter{Conn: a, entered: make(chan struct{})}
	m := newMux(conn, StreamFile)
	t.Cleanup(func() { m.Close() })
	m.start()
	if err := m.SendBytes(StreamFile, PriBulk, []byte("hold socket writer")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conn.entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not reach blocked peer")
	}
	// The writer is blocked on a real socket. Fill its bounded shared queue
	// through actual nonwaiting enqueue, leaving no room for even one ping.
	for {
		m.wmu.Lock()
		free := maxQueuedBytes - m.queuedBytes
		m.wmu.Unlock()
		if free == 0 {
			break
		}
		payload := free - headerLen
		if payload > maxPayload {
			payload = maxPayload
		}
		if payload < 0 {
			t.Fatal("fixture left unusable partial header")
		}
		if err := m.sendWithWait(StreamFile, msgData, PriBulk, make([]byte, payload), false); err != nil {
			t.Fatal(err)
		}
	}
	return m, b
}

func TestKeepaliveDeadlineSurvivesFullOutboundQueue(t *testing.T) {
	m, _ := fullKeepaliveQueue(t)
	m.Keepalive(5*time.Millisecond, 30*time.Millisecond)
	select {
	case <-m.Closed():
	case <-time.After(300 * time.Millisecond):
		t.Fatal("keepalive capacity wait suppressed dead-peer deadline")
	}
}

func TestKeepaliveFullQueueStillHonorsInboundLiveness(t *testing.T) {
	m, b := fullKeepaliveQueue(t)
	m.Keepalive(5*time.Millisecond, 80*time.Millisecond)
	for i := 0; i < 25; i++ {
		if err := writeFrame(b, Frame{StreamID: StreamControl, Type: msgPong, Priority: PriInteractive}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-m.Closed():
			t.Fatal("full queue alone closed a live session")
		case <-time.After(5 * time.Millisecond):
		}
	}
	m.wmu.Lock()
	queued := m.queuedBytes
	m.wmu.Unlock()
	if queued != maxQueuedBytes {
		t.Fatal("skipped pings changed or expanded saturated queue", queued)
	}
	select {
	case <-m.Closed():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("watchdog failed after inbound liveness stopped")
	}
}

func TestKeepaliveProbeResumesAfterCapacityReturns(t *testing.T) {
	m, b := fullKeepaliveQueue(t)
	m.Keepalive(5*time.Millisecond, 500*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	replied := make(chan struct{})
	go func() {
		for {
			f, err := readFrame(b)
			if err != nil {
				return
			}
			if f.Type == msgPing {
				if writeFrame(b, Frame{StreamID: StreamControl, Type: msgPong, Priority: PriInteractive, Payload: f.Payload}) == nil {
					close(replied)
				}
				return
			}
		}
	}()
	select {
	case <-replied:
	case <-time.After(time.Second):
		t.Fatal("probe did not resume when backlog drained")
	}
	deadline := time.Now().Add(time.Second)
	for m.RTT() <= 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.RTT() <= 0 {
		t.Fatal("resumed probe did not measure RTT")
	}
	select {
	case <-m.Closed():
		t.Fatal("recovered peer closed")
	default:
	}
}
