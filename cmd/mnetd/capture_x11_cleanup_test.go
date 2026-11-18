//go:build linux

package main

import (
	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

func TestX11CleanupReleasesOwnedPointerAndConnection(t *testing.T) {
	if os.Getenv("MNET_X11_CLEANUP_TEST") != "1" {
		t.Skip("requires explicitly disposable X display")
	}
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	root := xproto.Setup(observer).DefaultScreen(observer).Root
	inj, err := newX11Injector()
	if err != nil {
		t.Fatal(err)
	}
	// The injected down is the production XTEST path, not a fake key-state flag.
	if err := inj.Inject(session.InputEvent{Kind: "pointer", Action: "down", Button: "left", X: 20, Y: 20}); err != nil {
		t.Fatal(err)
	}
	state, err := xproto.QueryPointer(observer, root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if state.Mask&xproto.ButtonMask1 == 0 {
		t.Fatal("pointer-down was not observed")
	}
	closer, ok := inj.(io.Closer)
	if !ok {
		_ = inj.Inject(session.InputEvent{Kind: "pointer", Action: "up", Button: "left"})
		inj.(*x11Host).conn.Close()
		t.Fatal("X11 injector has no owned cleanup")
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = xproto.QueryPointer(observer, root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if state.Mask&xproto.ButtonMask1 != 0 {
		t.Fatal("remote button remained held after cleanup")
	}
	if err := inj.Inject(session.InputEvent{Kind: "pointer", Action: "click"}); err == nil {
		t.Fatal("closed injector reused")
	}
}

func TestX11CleanupHostStopReleasesHeldInput(t *testing.T) {
	if os.Getenv("MNET_X11_CLEANUP_TEST") != "1" {
		t.Skip("requires explicitly disposable X display")
	}
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	root := xproto.Setup(observer).DefaultScreen(observer).Root
	cap, err := newX11Capturer()
	if err != nil {
		t.Fatal(err)
	}
	defer cap.(io.Closer).Close()
	inj, err := newX11Injector()
	if err != nil {
		t.Fatal(err)
	}
	defer inj.(io.Closer).Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hostResult := make(chan *session.Mux, 1)
	go func() {
		m, _ := listenHeadlessSession(ln, "host", time.Second, func(session.Hello, net.Addr) (bool, string) { return true, "" })
		hostResult <- m
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := session.Dial(conn, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	host := <-hostResult
	if host == nil {
		t.Fatal("host handshake failed")
	}
	h := session.ServeHost(host, cap, inj, 8)
	defer h.Stop()
	v := session.ConnectViewer(viewer)
	if err := v.SendInput(session.InputEvent{Kind: "pointer", Action: "down", Button: "right", X: 20, Y: 20}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	held := false
	for time.Now().Before(deadline) {
		state, err := xproto.QueryPointer(observer, root).Reply()
		if err != nil {
			t.Fatal(err)
		}
		if state.Mask&xproto.ButtonMask3 != 0 {
			held = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !held {
		t.Fatal("session did not inject right-button down")
	}
	h.Stop()
	state, err := xproto.QueryPointer(observer, root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if state.Mask&xproto.ButtonMask3 != 0 {
		t.Fatal("session Stop left injected button held")
	}
	if !cap.(*x11Host).closed.Load() || !inj.(*x11Host).closed.Load() {
		t.Fatal("Stop leaked X11 resources")
	}
}
