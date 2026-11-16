package main

import (
	"fintex.au/mnet/internal/session"
	"net"
	"testing"
	"time"
)

func TestHeadlessHelloTimeoutAllowsNextViewer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	firstDone := make(chan error, 1)
	accepted := make(chan *session.Mux, 1)
	failure := make(chan error, 1)
	go func() {
		_, err := listenHeadlessSession(listener, "host", 40*time.Millisecond, func(session.Hello, net.Addr) (bool, string) { return true, "" })
		firstDone <- err
		mux, err := listenHeadlessSession(listener, "host", 40*time.Millisecond, func(session.Hello, net.Addr) (bool, string) { return true, "" })
		if err != nil {
			failure <- err
			return
		}
		accepted <- mux
	}()
	silent, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	select {
	case err := <-firstDone:
		if err == nil {
			t.Fatal("silent peer accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("silent hello blocked listener")
	}
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := session.Dial(conn, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	var host *session.Mux
	select {
	case host = <-accepted:
	case err := <-failure:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("next viewer not accepted")
	}
	defer host.Close()
	time.Sleep(80 * time.Millisecond)
	if err := viewer.SendJSON(session.StreamChat, session.PriChat, session.ChatMessage{Text: "still active"}); err != nil {
		t.Fatal(err)
	}
	// Beyond the handshake deadline, a fresh ping must still receive a pong.
	host.Keepalive(10*time.Millisecond, time.Second)
	deadline := time.Now().Add(time.Second)
	for host.RTT() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if host.RTT() == 0 {
		t.Fatal("no active transport round trip after handshake deadline")
	}
	select {
	case <-host.Closed():
		t.Fatal("successful session retained handshake deadline")
	default:
	}
}
