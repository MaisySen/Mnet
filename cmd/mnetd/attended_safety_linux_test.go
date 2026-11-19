//go:build linux

package main

import (
	"context"
	"fintex.au/mnet/internal/session"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func requireSafetyDisplay(t *testing.T) *xgb.Conn {
	t.Helper()
	if os.Getenv("MNET_ATTENDED_X11_TEST") != "1" {
		t.Skip("owned disposable X11 display only")
	}
	c, e := xgb.NewConn()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(c.Close)
	return c
}
func safetyAtom(t *testing.T, c *xgb.Conn) xproto.Atom {
	t.Helper()
	r, e := xproto.InternAtom(c, false, uint16(len(attendedInputOwnerAtom)), attendedInputOwnerAtom).Reply()
	if e != nil {
		t.Fatal(e)
	}
	return r.Atom
}
func assertSafetyOwner(t *testing.T, c *xgb.Conn, want bool) {
	t.Helper()
	r, e := xproto.GetSelectionOwner(c, safetyAtom(t, c)).Reply()
	if e != nil || (r.Owner != 0) != want {
		t.Fatal("unexpected shared owner", r, e)
	}
}
func waitSafetyStop(t *testing.T, s attendedSafety) {
	t.Helper()
	select {
	case <-s.Stopped():
	case <-time.After(time.Second):
		t.Fatal("local safety stop not delivered")
	}
}
func TestAttendedX11SafetyOwnerAndRelease(t *testing.T) {
	c := requireSafetyDisplay(t)
	s, e := newAttendedSafety()
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	assertSafetyOwner(t, c, true)
	if other, e := newAttendedSafety(); e == nil {
		other.Close()
		t.Fatal("second cooperating input owner acquired")
	}
	assertSafetyOwner(t, c, true)
	if !attendedSafetyAuthorized(s, func() bool { return true })() || attendedSafetyAuthorized(s, func() bool { return false })() {
		t.Fatal("local safety replaced peer authorization")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	waitSafetyStop(t, s)
	if attendedSafetyAuthorized(s, func() bool { return true })() {
		t.Fatal("retired owner authorized input")
	}
	assertSafetyOwner(t, c, false)
	next, e := newAttendedSafety()
	if e != nil {
		t.Fatal("owner/grabs leaked after close", e)
	}
	next.Close()
}
func TestAttendedX11SafetyGrabConflictRollsBack(t *testing.T) {
	c := requireSafetyDisplay(t)
	root := xproto.Setup(c).DefaultScreen(c).Root
	key := xproto.Keycode(keycodeFor(c, attendedStopKeysym))
	// Conflict late in modifier enumeration, proving partial grabs roll back too.
	mods := uint16(255)
	if e := xproto.GrabKeyChecked(c, false, root, mods, key, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); e != nil {
		t.Fatal(e)
	}
	if s, e := newAttendedSafety(); e == nil {
		s.Close()
		t.Fatal("conflicting reserved shortcut accepted")
	}
	// Reply round trip lets the failed connection's asynchronous close settle.
	deadline := time.Now().Add(time.Second)
	for {
		r, _ := xproto.GetSelectionOwner(c, safetyAtom(t, c)).Reply()
		if r != nil && r.Owner == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed acquisition retained owner")
		}
		time.Sleep(time.Millisecond)
	}
	if e := xproto.GrabKeyChecked(c, false, root, attendedStopModifiers, key, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); e != nil {
		t.Fatal("partial shortcut leaked", e)
	}
	xproto.UngrabKey(c, key, root, attendedStopModifiers)
	xproto.UngrabKey(c, key, root, mods)
	c.Sync()
	s, e := newAttendedSafety()
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
}
func TestAttendedX11SafetyOwnerLossAndMapChange(t *testing.T) {
	c := requireSafetyDisplay(t)
	for _, cause := range []string{"selection", "mapping"} {
		t.Run(cause, func(t *testing.T) {
			s, e := newAttendedSafety()
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			if cause == "selection" {
				if e := xproto.SetSelectionOwnerChecked(c, 0, safetyAtom(t, c), xproto.TimeCurrentTime).Check(); e != nil {
					t.Fatal(e)
				}
			} else {
				setup := xproto.Setup(c)
				key := setup.MinKeycode
				mapping, e := xproto.GetKeyboardMapping(c, key, 1).Reply()
				if e != nil {
					t.Fatal(e)
				}
				if e := xproto.ChangeKeyboardMappingChecked(c, 1, key, mapping.KeysymsPerKeycode, mapping.Keysyms).Check(); e != nil {
					t.Fatal(e)
				}
			}
			waitSafetyStop(t, s)
			if attendedSafetyAuthorized(s, func() bool { return true })() {
				t.Fatal("lost local owner remained live")
			}
			s.Close()
			assertSafetyOwner(t, c, false)
		})
	}
}

func TestAttendedX11SafetyStopsPendingRequest(t *testing.T) {
	observer := requireSafetyDisplay(t)
	client, server := attendedPair(t, true)
	vc := session.NewViewerControl(nil)
	_ = session.ConnectViewerWithOptions(client, session.ViewerOptions{Control: vc})
	s, e := newAttendedSafety()
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ui := &fixtureAttendedUI{make(chan localPromptCall, 8)}
	capture := &attendedFixtureCapture{}
	var creates atomic.Int32
	_, sessionDone := serveAttendedSession(context.Background(), server, "fixture-peer", true, ui, capture, func() (session.Injector, error) { creates.Add(1); return &attendedFixtureInput{}, nil }, attendedSafetyAuthorized(s, func() bool { return true }))
	done := bindAttendedSafety(context.Background(), server, s, sessionDone, func(err error) { t.Errorf("safety cleanup: %v", err) })
	_ = takePrompt(t, ui, "active")
	waitAttended(t, vc, "available")
	if e := vc.Request(); e != nil {
		t.Fatal(e)
	}
	prompt := takePrompt(t, ui, "control")
	// A different client replaces the selection while a local answer is pending.
	root := xproto.Setup(observer).DefaultScreen(observer).Root
	foreign, e := xproto.NewWindowId(observer)
	if e != nil {
		t.Fatal(e)
	}
	if e = xproto.CreateWindowChecked(observer, 0, foreign, root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0, 0, nil).Check(); e != nil {
		t.Fatal(e)
	}
	defer xproto.DestroyWindow(observer, foreign)
	if e = xproto.SetSelectionOwnerChecked(observer, foreign, safetyAtom(t, observer), xproto.TimeCurrentTime).Check(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("owner loss did not join pending session")
	}
	select {
	case <-prompt.ctx.Done():
	default:
		t.Fatal("pending grant dialog survived owner loss")
	}
	prompt.answer <- "allow"
	if creates.Load() != 0 || !capture.closed.Load() {
		t.Fatal("owner loss retained capture or allowed stale grant")
	}
	current, e := xproto.GetSelectionOwner(observer, safetyAtom(t, observer)).Reply()
	if e != nil || current.Owner != foreign {
		t.Fatal("old cleanup cleared replacement owner", e)
	}
	if other, e := newAttendedSafety(); e == nil {
		other.Close()
		t.Fatal("replacement owner ignored")
	}
}
