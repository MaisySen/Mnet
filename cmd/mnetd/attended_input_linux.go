//go:build linux

package main

import (
	"errors"
	"unicode/utf8"

	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// Attended text never invokes legacy clipboard/global-remapping behavior.
// It supports only bounded characters already available at level 0/Shift in
// the current base X keyboard group. Unsupported text fails before typing.
// This is logical text, not a Shared Desk physical-key protocol.
type attendedX11Injector struct{ host *x11Host }
type attendedStroke struct {
	key   byte
	shift bool
}

func newAttendedX11Injector() (session.Injector, error) {
	input, err := newX11Injector()
	if err != nil {
		return nil, err
	}
	host := input.(*x11Host)
	screen := xproto.Setup(host.conn).DefaultScreen(host.conn)
	host.root = screen.Root
	host.w, host.h = int(screen.WidthInPixels), int(screen.HeightInPixels)
	host.scale = capScale(host.w, host.h)
	return &attendedX11Injector{host: host}, nil
}
func (i *attendedX11Injector) CheckCaptureGeometry(width, height int) error {
	w, h := i.host.Geometry()
	if width != w || height != h {
		return errors.New("attended input/capture geometry differs")
	}
	return nil
}

func (i *attendedX11Injector) Close() error { return i.host.Close() }
func (i *attendedX11Injector) Inject(session.InputEvent) error {
	return errors.New("attended input requires current authorization")
}
func (i *attendedX11Injector) InjectAuthorized(ev session.InputEvent, live func() bool) error {
	if live == nil || !live() {
		return errors.New("retired attended input")
	}
	if ev.Kind == "pointer" {
		switch ev.Action {
		case "move", "down", "up", "click":
			w, h := i.host.Geometry()
			if ev.X < 0 || ev.Y < 0 || ev.X >= w || ev.Y >= h {
				return errors.New("attended pointer outside frame")
			}
			if ev.Action != "move" && ev.Button != "left" && ev.Button != "right" && ev.Button != "middle" {
				return errors.New("unsupported attended pointer button")
			}
		case "scroll":
			if ev.DY == 0 {
				return errors.New("empty attended scroll")
			}
			x := i.host
			x.inputMu.Lock()
			defer x.inputMu.Unlock()
			if x.closed.Load() {
				return errors.New("attended injector retired")
			}
			button := byte(5)
			if ev.DY < 0 {
				button = 4
			}
			state, err := xproto.QueryPointer(x.conn, x.root).Reply()
			if err != nil || state.Mask&(uint16(1)<<uint(button+7)) != 0 {
				return errors.New("preheld scroll refused")
			}
			if !live() {
				return errors.New("retired attended scroll")
			}
			if err = xtest.FakeInputChecked(x.conn, xproto.ButtonPress, button, 0, x.root, 0, 0, 0).Check(); err != nil {
				return err
			}
			x.pressed[button] = true
			err = xtest.FakeInputChecked(x.conn, xproto.ButtonRelease, button, 0, x.root, 0, 0, 0).Check()
			if err == nil {
				x.pressed[button] = false
			}
			return err
		default:
			return errors.New("unsupported attended pointer action")
		}
		return i.host.Inject(ev)
	}
	if ev.Kind != "key" {
		return errors.New("unsupported attended input kind")
	}
	x := i.host
	x.inputMu.Lock()
	defer x.inputMu.Unlock()
	if x.conn == nil || x.closed.Load() {
		return errors.New("attended injector unavailable")
	}
	if len(ev.Text) > 4096 || !utf8.ValidString(ev.Text) || utf8.RuneCountInString(ev.Text) > 1024 {
		return errors.New("attended text exceeds supported bounds")
	}
	setup := xproto.Setup(x.conn)
	mapping, err := xproto.GetKeyboardMapping(x.conn, setup.MinKeycode, byte(setup.MaxKeycode-setup.MinKeycode+1)).Reply()
	if err != nil {
		return errors.New("attended keyboard map unavailable")
	}
	per := int(mapping.KeysymsPerKeycode)
	shift := keycodeFor(x.conn, 0xffe1)
	if shift == 0 {
		return errors.New("attended Shift unavailable")
	}
	strokes := make([]attendedStroke, 0, utf8.RuneCountInString(ev.Text))
	for _, r := range ev.Text {
		sym := keysymForRune(r)
		switch r {
		case '\n':
			sym = 0xff0d
		case '\t':
			sym = 0xff09
		}
		found := false
		for level := 0; level < 2 && !found; level++ {
			if level >= per {
				continue
			}
			for code := int(setup.MinKeycode); code <= int(setup.MaxKeycode); code++ {
				if mapping.Keysyms[(code-int(setup.MinKeycode))*per+level] == sym {
					strokes = append(strokes, attendedStroke{byte(code), level == 1})
					found = true
					break
				}
			}
		}
		if !found {
			return errors.New("attended text unavailable in current keyboard layout; no remapping performed")
		}
	}
	root := setup.DefaultScreen(x.conn).Root
	for _, stroke := range strokes {
		if !live() {
			return errors.New("retired attended text")
		}
		// Do not turn text into Ctrl/Alt/Super shortcuts or mutate local modifier
		// state. Lock/group state also changes keysym meaning, so refuse it here.
		pointer, err := xproto.QueryPointer(x.conn, root).Reply()
		if err != nil || pointer.Mask&(255|0x6000) != 0 {
			return errors.New("attended text requires unmodified local keyboard state")
		}
		if !live() {
			return errors.New("retired attended text")
		}
		keys, keyErr := xproto.QueryKeymap(x.conn).Reply()
		if keyErr != nil || keys.Keys[stroke.key/8]&(1<<uint(stroke.key%8)) != 0 {
			return errors.New("preheld attended text key refused")
		}
		if !live() {
			return errors.New("retired attended text")
		}
		heldShift := false
		if stroke.shift {
			if err = xtest.FakeInputChecked(x.conn, xproto.KeyPress, shift, 0, root, 0, 0, 0).Check(); err != nil {
				return err
			}
			heldShift = true
		}
		// Always attempt paired releases, even when owner cancellation or a request
		// error intervenes. Only this one stroke is in flight between liveness checks.
		attempted := live()
		if attempted {
			err = xtest.FakeInputChecked(x.conn, xproto.KeyPress, stroke.key, 0, root, 0, 0, 0).Check()
		}
		var releaseErr error
		if attempted {
			releaseErr = xtest.FakeInputChecked(x.conn, xproto.KeyRelease, stroke.key, 0, root, 0, 0, 0).Check()
		}
		if heldShift {
			releaseErr = errors.Join(releaseErr, xtest.FakeInputChecked(x.conn, xproto.KeyRelease, shift, 0, root, 0, 0, 0).Check())
		}
		if err != nil || releaseErr != nil {
			return errors.Join(err, releaseErr)
		}
	}
	return nil
}
