package session

import (
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type countedSessionConn struct {
	net.Conn
	closes atomic.Int32
}

func (c *countedSessionConn) Close() error { c.closes.Add(1); return c.Conn.Close() }

func TestDialFailedHandshakeClosesOwnedConnection(t *testing.T) {
	replies := []Frame{
		{StreamID: StreamChat, Type: msgData, Payload: []byte(`{}`)},
		{StreamID: StreamControl, Type: msgData, Payload: []byte(`bad-json`)},
		{StreamID: StreamControl, Type: msgData, Payload: []byte(`{"version":99,"accepted":true}`)},
		{StreamID: StreamControl, Type: msgData, Payload: []byte(`{"version":1,"accepted":false}`)},
	}
	for _, reply := range replies {
		a, b := net.Pipe()
		conn := &countedSessionConn{Conn: a}
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer b.Close()
			if _, err := readFrame(b); err == nil {
				_ = writeFrame(b, reply)
			}
		}()
		_, err := Dial(conn, "fixture")
		if err == nil {
			t.Error("bad handshake accepted")
		}
		if conn.closes.Load() != 1 {
			t.Error("failed Dial did not close owned connection")
		}
		conn.Close()
		<-done
	}
	a, b := net.Pipe()
	b.Close()
	conn := &countedSessionConn{Conn: a}
	if _, err := Dial(conn, "fixture"); err == nil {
		t.Fatal("closed transport accepted")
	}
	if conn.closes.Load() != 1 {
		t.Error("failed hello write did not close")
	}
	conn.Close()
}

func TestDialSuccessTransfersConnectionToMux(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	conn := &countedSessionConn{Conn: a}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := readFrame(b); err != nil {
			return
		}
		payload, _ := json.Marshal(HelloReply{Version: protoVersion, Accepted: true})
		_ = writeFrame(b, Frame{StreamID: StreamControl, Type: msgData, Payload: payload})
		<-time.After(20 * time.Millisecond)
	}()
	mux, err := Dial(conn, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if conn.closes.Load() != 0 {
		t.Fatal("success closed active transport")
	}
	mux.Close()
	if conn.closes.Load() != 1 {
		t.Fatal("mux did not close transport")
	}
	<-done
}
