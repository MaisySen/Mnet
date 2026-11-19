//go:build linux

package main

import (
	"strings"
	"testing"

	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// This first regression is intentionally against the existing attended entry,
// before its production fix. Mapping must remain byte-for-byte unchanged.
func TestAttendedX11TextDoesNotRemapDesktop(t *testing.T) {
	c := requireSafetyDisplay(t)
	setup := xproto.Setup(c)
	read := func() []xproto.Keysym {
		r, e := xproto.GetKeyboardMapping(c, setup.MinKeycode, byte(setup.MaxKeycode-setup.MinKeycode+1)).Reply()
		if e != nil {
			t.Fatal(e)
		}
		return r.Keysyms
	}
	before := read()
	input, e := newAttendedInjector()
	if e != nil {
		t.Fatal(e)
	}
	defer input.(interface{ Close() error }).Close()
	checked, ok := input.(interface {
		InjectAuthorized(session.InputEvent, func() bool) error
	})
	if !ok {
		t.Fatal("attended input has no mid-event authorization path")
	}
	_ = checked.InjectAuthorized(session.InputEvent{Kind: "key", Text: "\U0001f9ed"}, func() bool { return true })
	after := read()
	if len(before) != len(after) {
		t.Fatal("attended text changed global keyboard map length")
	}
	for i, v := range before {
		if after[i] != v {
			t.Fatal("attended text changed a global keyboard mapping")
		}
	}
}

func TestAttendedX11TextCancellationAndShiftRelease(t *testing.T) {
	c := requireSafetyDisplay(t)
	root := xproto.Setup(c).DefaultScreen(c).Root
	window, err := xproto.NewWindowId(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.CreateWindowChecked(c, 0, window, root, 0, 0, 100, 100, 0, xproto.WindowClassInputOutput, 0, xproto.CwEventMask, []uint32{xproto.EventMaskKeyPress}).Check(); err != nil {
		t.Fatal(err)
	}
	defer xproto.DestroyWindow(c, window)
	xproto.MapWindow(c, window)
	if err = xproto.SetInputFocusChecked(c, xproto.InputFocusPointerRoot, window, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	input, err := newAttendedInjector()
	if err != nil {
		t.Fatal(err)
	}
	defer input.(interface{ Close() error }).Close()
	checked := input.(*attendedX11Injector)
	calls := 0
	err = checked.InjectAuthorized(session.InputEvent{Kind: "key", Text: strings.Repeat("Aa", 100)}, func() bool { calls++; return calls < 12 })
	if err == nil {
		t.Fatal("retired text event was not interrupted")
	}
	if calls > 15 {
		t.Fatal("text continued through whole event after retirement", calls)
	}
	pointer, err := xproto.QueryPointer(c, root).Reply()
	if err != nil || pointer.Mask&255 != 0 {
		t.Fatal("retired text retained keyboard modifier", pointer, err)
	}
	xproto.GetInputFocus(c).Reply() // ordered server roundtrip before event drain
	presses := 0
	shiftSeen := false
	for {
		ev, e := c.PollForEvent()
		if e != nil {
			t.Fatal(e)
		}
		if ev == nil {
			break
		}
		if key, ok := ev.(xproto.KeyPressEvent); ok {
			presses++
			if key.State&xproto.ModMaskShift != 0 {
				shiftSeen = true
			}
		}
	}
	if presses == 0 || presses >= 200 || !shiftSeen {
		t.Fatal("actual bounded/shifted text input missing or not interrupted", presses, shiftSeen)
	}
	// Full preflight rejects a later unsupported rune before emitting the prefix.
	if err = checked.InjectAuthorized(session.InputEvent{Kind: "key", Text: "a\U0001f9ed"}, func() bool { return true }); err == nil {
		t.Fatal("unmapped text accepted")
	}
	xproto.GetInputFocus(c).Reply()
	for {
		ev, e := c.PollForEvent()
		if e != nil {
			t.Fatal(e)
		}
		if ev == nil {
			break
		}
		if _, ok := ev.(xproto.KeyPressEvent); ok {
			t.Fatal("rejected text emitted prefix")
		}
	}
	if err = checked.InjectAuthorized(session.InputEvent{Kind: "key", Text: strings.Repeat("a", 1025)}, func() bool { return true }); err == nil {
		t.Fatal("oversized text accepted")
	}
}

func TestAttendedX11TextRejectsForeignModifierWithoutReleasingIt(t *testing.T) {
	c := requireSafetyDisplay(t)
	if err := xtest.Init(c); err != nil {
		t.Fatal(err)
	}
	root := xproto.Setup(c).DefaultScreen(c).Root
	ctrl := keycodeFor(c, 0xffe3)
	if ctrl == 0 {
		t.Fatal("no test Control")
	}
	if err := xtest.FakeInputChecked(c, xproto.KeyPress, ctrl, 0, root, 0, 0, 0).Check(); err != nil {
		t.Fatal(err)
	}
	defer xtest.FakeInput(c, xproto.KeyRelease, ctrl, 0, root, 0, 0, 0)
	input, err := newAttendedInjector()
	if err != nil {
		t.Fatal(err)
	}
	checked := input.(*attendedX11Injector)
	if err = checked.InjectAuthorized(session.InputEvent{Kind: "key", Text: "a"}, func() bool { return true }); err == nil {
		t.Fatal("foreign Ctrl turned text into shortcut")
	}
	if err = checked.Close(); err != nil {
		t.Fatal(err)
	}
	pointer, err := xproto.QueryPointer(c, root).Reply()
	if err != nil || pointer.Mask&xproto.ModMaskControl == 0 {
		t.Fatal("attended failure released foreign modifier", pointer, err)
	}
	if err = xtest.FakeInputChecked(c, xproto.KeyRelease, ctrl, 0, root, 0, 0, 0).Check(); err != nil {
		t.Fatal(err)
	}
}

func TestAttendedX11ScaledPointer(t *testing.T) {
	observer := requireSafetyDisplay(t)
	screen := xproto.Setup(observer).DefaultScreen(observer)
	scale := capScale(int(screen.WidthInPixels), int(screen.HeightInPixels))
	if scale < 2 {
		t.Skip("requires downscaled capture display")
	}
	input, err := newAttendedInjector()
	if err != nil {
		t.Fatal(err)
	}
	defer input.(interface{ Close() error }).Close()
	ev := session.InputEvent{Kind: "pointer", Action: "click", Button: "left", X: 20, Y: 20}
	if checked, ok := input.(interface {
		InjectAuthorized(session.InputEvent, func() bool) error
	}); ok {
		err = checked.InjectAuthorized(ev, func() bool { return true })
	} else {
		err = input.Inject(ev)
	}
	if err != nil {
		t.Fatal(err)
	}
	p, err := xproto.QueryPointer(observer, screen.Root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if int(p.RootX) != 20*scale || int(p.RootY) != 20*scale {
		t.Fatalf("frame point20,20 at capture scale%d injected desktop point%d,%d; want%d,%d", scale, p.RootX, p.RootY, 20*scale, 20*scale)
	}
}

func TestAttendedX11PointerBoundsAndGeometry(t *testing.T) {
	c := requireSafetyDisplay(t)
	root := xproto.Setup(c).DefaultScreen(c).Root
	input, err := newAttendedInjector()
	if err != nil {
		t.Fatal(err)
	}
	checked := input.(*attendedX11Injector)
	defer checked.Close()
	w, h := checked.host.Geometry()
	if err = checked.CheckCaptureGeometry(w, h); err != nil {
		t.Fatal(err)
	}
	if err = checked.CheckCaptureGeometry(w+1, h); err == nil {
		t.Fatal("mismatched capture/input geometry accepted")
	}
	before, err := xproto.QueryPointer(c, root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []session.InputEvent{
		{Kind: "pointer", Action: "click", Button: "left", X: -1, Y: 2},
		{Kind: "pointer", Action: "move", X: w, Y: 2},
		{Kind: "pointer", Action: "move", X: 2, Y: h},
		{Kind: "pointer", Action: "move", X: 1 << 30, Y: 2},
		{Kind: "pointer", Action: "click", Button: "unknown", X: 2, Y: 2},
		{Kind: "pointer", Action: "unknown", X: 2, Y: 2},
	} {
		if err = checked.InjectAuthorized(ev, func() bool { return true }); err == nil {
			t.Fatal("invalid pointer accepted", ev)
		}
		after, err := xproto.QueryPointer(c, root).Reply()
		if err != nil || after.RootX != before.RootX || after.RootY != before.RootY || after.Mask != before.Mask {
			t.Fatal("invalid pointer changed desktop input", after, err)
		}
	}
}
