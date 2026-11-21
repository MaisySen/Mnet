package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
)

// The home notices state that no viewing session remains. That statement is
// only honest because the transport, viewer control and display owners are
// joined before the viewer function returns, on clean and abnormal ends.
// These tests prove the ordering at the function boundary: by the time the
// caller can show a notice, the session transport is already closed, not
// closing eventually.

func desktopTeardownPair(t *testing.T) (*session.Mux, *session.Mux) {
	t.Helper()
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted := make(chan *session.Mux, 1)
	go func() {
		host, _ := session.Listen(ln, "fixture", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		accepted <- host
	}()
	right, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	mux, err := session.Dial(right, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	host := <-accepted
	if host == nil {
		t.Fatal("host unavailable")
	}
	active := session.ServeHost(host, &onceScreenCapture{data: jpegData.Bytes()}, nil, 100)
	t.Cleanup(active.Stop)
	t.Cleanup(func() { host.Close() })
	return mux, host
}

func assertMuxClosed(t *testing.T, mux *session.Mux) {
	t.Helper()
	select {
	case <-mux.Closed():
	default:
		t.Fatal("viewer returned before its session transport was closed")
	}
}

func TestDesktopLocalEndJoinsTransportBeforeReturn(t *testing.T) {
	mux, _ := desktopTeardownPair(t)
	view := &firstScreenEndView{endAfterFrame: make(chan struct{})}
	viewed, err := runDesktopWindowWithFirstScreenWait(context.Background(), view, func(context.Context) (*session.Mux, error) { return mux, nil }, false, 5*time.Second)
	if err != nil || !viewed || view.frames == 0 || !view.closed {
		t.Fatal("local end outcome", err, viewed, view.frames, view.closed)
	}
	assertMuxClosed(t, mux)
}

func TestDesktopHostCloseJoinsTransportBeforeReturn(t *testing.T) {
	mux, host := desktopTeardownPair(t)
	view := &testDesktopView{}
	closed := make(chan struct{})
	go func() {
		<-mux.Closed()
		close(closed)
	}()
	time.AfterFunc(300*time.Millisecond, func() { host.Close() })
	viewed, err := runDesktopWindowWithFirstScreenWait(context.Background(), view, func(context.Context) (*session.Mux, error) { return mux, nil }, false, 5*time.Second)
	if !errors.Is(err, errDesktopHostEnded) {
		t.Fatal("host close outcome", err)
	}
	assertMuxClosed(t, mux)
	<-closed
	// The one-shot capture delivers a frame before the host close, so this is
	// a mid-session abnormal end with viewed already true.
	if !viewed {
		t.Fatal("served frame not reported as viewed")
	}
	if !view.closed {
		t.Fatal("display owner not joined")
	}
}

func TestDesktopCanceledDialJoinsBeforeReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dialEntered := make(chan struct{})
	dialReturn := make(chan struct{})
	view := &testDesktopView{}
	done := make(chan error, 1)
	go func() {
		_, err := runDesktopWindowWithFirstScreenWait(ctx, view, func(ctx context.Context) (*session.Mux, error) {
			close(dialEntered)
			<-ctx.Done()
			close(dialReturn)
			return nil, ctx.Err()
		}, false, 5*time.Second)
		done <- err
	}()
	<-dialEntered
	cancel()
	if err := <-done; err != nil {
		t.Fatal("canceled dial outcome", err)
	}
	select {
	case <-dialReturn:
	default:
		t.Fatal("viewer returned before its dial owner joined")
	}
	if !view.closed {
		t.Fatal("display owner not joined")
	}
}
