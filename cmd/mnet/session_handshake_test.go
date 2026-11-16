package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
)

type deadlineSessionConn struct {
	net.Conn
	mu        sync.Mutex
	deadlines []time.Time
}

func (c *deadlineSessionConn) SetDeadline(at time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, at)
	c.mu.Unlock()
	return c.Conn.SetDeadline(at)
}
func consumeSessionHello(conn net.Conn) error {
	var h [12]byte
	if _, err := io.ReadFull(conn, h[:]); err != nil {
		return err
	}
	_, err := io.CopyN(io.Discard, conn, int64(binary.BigEndian.Uint32(h[8:])))
	return err
}
func TestSessionHandshakeSilentHostTimesOut(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	done := make(chan struct{})
	go func() { defer close(done); _ = consumeSessionHello(b) }()
	start := time.Now()
	mux, err := dialSessionHandshake(a, "fixture", 30*time.Millisecond)
	if mux != nil || err == nil {
		t.Fatal("silent host accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("handshake deadline ignored")
	}
	if _, err := a.Write([]byte("closed")); err == nil {
		t.Fatal("failed handshake left connection open")
	}
	<-done
}
func TestSessionHandshakeClearsSuccessfulDeadline(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	conn := &deadlineSessionConn{Conn: a}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if consumeSessionHello(b) != nil {
			return
		}
		payload, _ := json.Marshal(session.HelloReply{Version: 1, Accepted: true})
		var h [12]byte
		h[4] = 2
		binary.BigEndian.PutUint32(h[8:], uint32(len(payload)))
		_, _ = b.Write(h[:])
		_, _ = b.Write(payload)
	}()
	mux, err := dialSessionHandshake(conn, "fixture", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()
	<-done
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.deadlines) != 2 || conn.deadlines[0].IsZero() || !conn.deadlines[1].IsZero() {
		t.Fatalf("deadlines: %v", conn.deadlines)
	}
}
