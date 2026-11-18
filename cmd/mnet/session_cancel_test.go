package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

func TestCLIRunCancelInterruptsPendingHello(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	entered := make(chan struct{})
	hostDone := make(chan struct{})
	go func() {
		defer close(hostDone)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		if consumeSessionHello(c) != nil {
			return
		}
		close(entered)
		io.Copy(io.Discard, c)
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	peer := daemon.SessionPeer{Name: "host", IPv4: "127.0.0.1", PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := dialAuthorizedCLIContext(ctx, conn, "viewer", time.Minute, peer, func() ([]daemon.SessionPeer, error) { return []daemon.SessionPeer{peer}, nil })
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("hello not reached")
	}
	go func() { defer cancel(); scanSessionLines(ctx, strings.NewReader(""), io.Discard, make(chan string, 1)) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled hello succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("EOF cancellation waited for hello deadline")
	}
	select {
	case <-hostDone:
	case <-time.After(time.Second):
		t.Fatal("host socket did not close")
	}
}
func TestCLIRunCancelClosesActiveMuxOnly(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	result := make(chan *session.Mux, 1)
	go func() {
		m, _ := session.Listen(ln, "host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		result <- m
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	peer := daemon.SessionPeer{Name: "host", IPv4: "127.0.0.1", PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	viewer, err := dialAuthorizedCLIContext(ctx, conn, "viewer", time.Second, peer, func() ([]daemon.SessionPeer, error) { return []daemon.SessionPeer{peer}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	host := <-result
	if host == nil {
		t.Fatal("host handshake failed")
	}
	defer host.Close()
	// A separate local socket is not tied to this run's cancellation.
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	cancel()
	select {
	case <-viewer.Closed():
	case <-time.After(time.Second):
		t.Fatal("active run escaped cancellation")
	}
	sent := make(chan error, 1)
	go func() { _, err := a.Write([]byte{1}); sent <- err }()
	b.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := b.Read(make([]byte, 1)); err != nil {
		t.Fatal("unrelated socket affected", err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
}
func TestCLIRunAlreadyCanceledRefusesDialAndHello(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a, b := net.Pipe()
	defer b.Close()
	if _, err := dialAuthorizedCLIContext(ctx, a, "viewer", time.Minute, daemon.SessionPeer{}, func() ([]daemon.SessionPeer, error) { t.Fatal("canceled attempt read identity"); return nil, nil }); err == nil {
		t.Fatal("canceled hello admitted")
	}
	if _, err := dialSessionTryAuthorizedContext(ctx, &cliSessionAuthorization{lookup: func() ([]daemon.SessionPeer, error) { t.Fatal("canceled attempt read local API"); return nil, nil }}, "viewer"); err == nil {
		t.Fatal("canceled dial admitted")
	}
}
func TestCLIRunScannerFullQueueCancelsAndKeepsPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lines := make(chan string, 1)
	lines <- "already full"
	done := make(chan struct{})
	go func() { scanSessionLines(ctx, strings.NewReader("blocked\n"), io.Discard, lines); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scanner remained blocked enqueueing after run ended")
	}
	var prompt bytes.Buffer
	queue := make(chan string, 2)
	scanSessionLines(context.Background(), strings.NewReader("one\n\ntwo\n"), &prompt, queue)
	if <-queue != "one" || <-queue != "two" {
		t.Fatal("chat lines changed")
	}
	if prompt.String() != "> > > > " {
		t.Fatal("terminal prompt changed", prompt.String())
	}
}
