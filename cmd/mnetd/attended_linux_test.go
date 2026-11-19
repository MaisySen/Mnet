//go:build linux

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type tracedAttendedUI struct {
	attendedUI
	t *testing.T
}

func (u tracedAttendedUI) Ask(c context.Context, p attendedPrompt) (string, error) {
	a, e := u.attendedUI.Ask(c, p)
	u.t.Logf("dialog kind=%s action=%s error=%v", p.Kind, a, e)
	return a, e
}

func TestAttendedDialogArgumentsAndProcessCancellation(t *testing.T) {
	for _, kind := range []string{"view", "control", "active"} {
		args, err := attendedDialogArgs(attendedPrompt{kind, "<b>fixture</b>"})
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(args, "\n")
		if !strings.Contains(joined, "--no-markup") || (kind != "active" && !strings.Contains(joined, "--default-cancel")) {
			t.Fatal("unsafe prompt arguments")
		}
	}
	if _, err := attendedDialogArgs(attendedPrompt{Kind: "remote-grant"}); err == nil {
		t.Fatal("unknown prompt accepted")
	}
	dir := t.TempDir()
	program := filepath.Join(dir, "fixture-dialog")
	for _, fixture := range []struct {
		body string
		kind string
		ok   bool
		want string
	}{{"printf 'Revoke control'; exit 1", "active", true, "revoke"}, {"printf 'unknown'", "active", false, ""}, {"printf 'unexpected'", "control", false, ""}, {"exit 1", "view", false, ""}, {"exec sleep 10", "view", false, ""}} {
		if err := os.WriteFile(program, []byte("#!/bin/sh\n"+fixture.body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		start := time.Now()
		a, err := (zenityUI{program}).Ask(ctx, attendedPrompt{Kind: fixture.kind})
		cancel()
		if (err == nil) != fixture.ok || a != fixture.want {
			t.Fatal("unexpected process result", a, err)
		}
		if time.Since(start) > time.Second {
			t.Fatal("dialog cancellation not joined")
		}
	}
	t.Setenv("WAYLAND_DISPLAY", "fixture")
	if _, err := newAttendedUI(); err == nil {
		t.Fatal("unsupported display accepted")
	}
}

// Opt in only on a disposable display. The external rig answers actual Zenity
// windows and saves pixels. No personal desktop, real account or mesh is used.
func TestAttendedX11RealLocalSurface(t *testing.T) {
	if os.Getenv("MNET_ATTENDED_X11_TEST") != "1" {
		t.Skip("requires disposable X display and local window driver")
	}
	dir := os.Getenv("MNET_ATTENDED_ARTIFACTS")
	if dir == "" {
		t.Fatal("explicit artifact directory required")
	}
	stage := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "stage"), []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ui, err := newAttendedUI()
	if err != nil {
		t.Fatal(err)
	}
	ui = tracedAttendedUI{ui, t}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ready := make(chan *session.Mux, 1)
	fail := make(chan error, 1)
	stage("view")
	go func() {
		m, err := listenHeadlessSession(ln, "fixture-host", 20*time.Second, func(session.Hello, net.Addr) (bool, string) {
			return approveAttendedView(ctx, ui, "Fixture mesh viewer"), "local viewing decision"
		})
		if err != nil {
			fail <- err
		} else {
			ready <- m
		}
	}()
	clientReady := make(chan *session.Mux, 1)
	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			fail <- err
			return
		}
		m, err := session.DialWithControl(conn, "fixture-viewer", nil, true)
		if err != nil {
			fail <- err
		} else {
			clientReady <- m
		}
	}()
	var client, host *session.Mux
	select {
	case host = <-ready:
	case err := <-fail:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case client = <-clientReady:
	case err := <-fail:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	defer client.Close()
	defer host.Close()
	vc := session.NewViewerControl(nil)
	frames := make(chan struct{}, 1)
	v := session.ConnectViewerWithOptions(client, session.ViewerOptions{Control: vc, OnFrame: func(session.ScreenFrame) {
		select {
		case frames <- struct{}{}:
		default:
		}
	}})
	safety, err := newAttendedSafety()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = safety.Close() })
	cap, err := newCapturer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, sessionDone := serveAttendedSession(ctx, host, "Fixture mesh viewer", true, ui, cap, testAttendedProcessInjector, attendedSafetyAuthorized(safety, func() bool { return true }))
	done := bindAttendedSafety(ctx, host, safety, sessionDone, func(err error) { t.Errorf("safety cleanup: %v", err) })
	defer func() {
		host.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("UI/capture teardown not joined")
		}
	}()
	waitAttended(t, vc, "available")
	select {
	case <-frames:
	case <-ctx.Done():
		t.Fatal("no actual X11 frame")
	}
	stage("active")
	// Explicit driver acknowledgement avoids timing the screenshot against a sleep.
	for {
		ack, _ := os.ReadFile(filepath.Join(dir, "driver-ack"))
		if string(ack) == "active" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("driver did not inspect active window")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	stage("control")
	waitAttended(t, vc, "allowed")
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	screen := xproto.Setup(observer).DefaultScreen(observer)
	root := screen.Root
	pointerScale := capScale(int(screen.WidthInPixels), int(screen.HeightInPixels))
	if err := v.SendInput(session.InputEvent{Kind: "pointer", Action: "click", Button: "left", X: 20, Y: 20}); err != nil {
		t.Fatal(err)
	}
	pointerUntil := time.Now().Add(time.Second)
	var p *xproto.QueryPointerReply
	for {
		p, err = xproto.QueryPointer(observer, root).Reply()
		if err == nil && int(p.RootX) == 20*pointerScale && int(p.RootY) == 20*pointerScale {
			break
		}
		if time.Now().After(pointerUntil) {
			t.Fatal("granted actual XTEST pointer not observed")
		}
		time.Sleep(time.Millisecond)
	}
	stage("pause")
	waitAttended(t, vc, "paused")
	p, err = xproto.QueryPointer(observer, root).Reply()
	if err != nil || p.Mask&xproto.ButtonMask1 != 0 {
		t.Fatal("paused pointer state unavailable")
	}
	stage("resume")
	waitAttended(t, vc, "allowed")
	stage("revoke")
	waitAttended(t, vc, "revoked")
	select {
	case <-client.Closed():
		t.Fatal("ordinary revoke ended viewing")
	default:
	}

	if err := vc.Request(); err != nil {
		t.Fatal(err)
	}
	stage("control-again")
	waitAttended(t, vc, "allowed")
	if err := v.SendInput(session.InputEvent{Kind: "pointer", Action: "down", Button: "left", X: 20, Y: 20}); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	held := false
	for time.Now().Before(until) {
		p, err = xproto.QueryPointer(observer, root).Reply()
		if err == nil && p.Mask&xproto.ButtonMask1 != 0 {
			held = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !held {
		t.Fatal("granted held XTEST pointer not observed")
	}
	// A different mapped application window, not the Mnet dialog or root,
	// gets keyboard focus while the remote pointer remains held.
	foreign, err := xproto.NewWindowId(observer)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.CreateWindowChecked(observer, 0, foreign, root, 680, 400, 240, 120, 0, xproto.WindowClassInputOutput, 0, xproto.CwBackPixel, []uint32{0xffffff}).Check(); err != nil {
		t.Fatal(err)
	}
	defer xproto.DestroyWindow(observer, foreign)
	if err = xproto.MapWindowChecked(observer, foreign).Check(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "reclaim-window"), []byte(fmt.Sprint(foreign)), 0600); err != nil {
		t.Fatal(err)
	}
	stage("global-stop-held")
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("end not joined")
	}
	p, err = xproto.QueryPointer(observer, root).Reply()
	if err != nil || p.Mask&xproto.ButtonMask1 != 0 {
		t.Fatal("end retained held pointer")
	}
	// End must have released both the selection and checked shortcut. A new
	// cooperating host can acquire only after all old input/UI teardown joined.
	next, err := newAttendedSafety()
	if err != nil {
		t.Fatal("input owner retained after joined End", err)
	}
	next.Close()
	stage("complete")
}
