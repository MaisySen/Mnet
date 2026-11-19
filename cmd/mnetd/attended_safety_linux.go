//go:build linux

package main

import (
	"errors"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const attendedStopKeysym = xproto.Keysym(0xffc9) // F12
const attendedStopModifiers = uint16(xproto.ModMaskControl | xproto.ModMask1 | xproto.ModMaskShift)
const attendedInputOwnerAtom = "_MNET_NATIVE_INPUT_OWNER_V1"

type x11AttendedSafety struct {
	conn     *xgb.Conn
	window   xproto.Window
	key      xproto.Keycode
	roots    []xproto.Window
	closeErr error
	stopped  chan struct{}
	done     chan struct{}
	once     sync.Once
}

func (s *x11AttendedSafety) Stopped() <-chan struct{} { return s.stopped }
func (s *x11AttendedSafety) Close() error {
	s.once.Do(func() { s.closeErr = s.release(); s.conn.Close() })
	<-s.done
	return s.closeErr
}

// XGB Close queues shutdown, so waiting for our event loop alone is not proof
// that the server processed disconnect. Explicit checked release joins the
// server-side resources before a new attended slot may be admitted.
func (s *x11AttendedSafety) release() error {
	var first error
	check := func(err error) {
		if first == nil {
			first = err
		}
	}
	check(xproto.UngrabKeyboardChecked(s.conn, xproto.TimeCurrentTime).Check())
	for _, root := range s.roots {
		for extra := uint16(0); extra < 256; extra++ {
			if extra&attendedStopModifiers == 0 {
				check(xproto.UngrabKeyChecked(s.conn, s.key, root, attendedStopModifiers|extra).Check())
			}
		}
	}
	// Destroying our own window cannot clear a different process's new owner.
	check(xproto.DestroyWindowChecked(s.conn, s.window).Check())
	return first
}

func newAttendedSafety() (attendedSafety, error) {
	if onWayland() {
		return nil, errors.New("attended safety requires X11")
	}
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, errors.New("local stop connection unavailable")
	}
	success := false
	defer func() {
		if !success {
			conn.Close()
		}
	}()
	setup := xproto.Setup(conn)
	root := setup.DefaultScreen(conn).Root
	key := attendedStopKey(conn)
	if key == 0 {
		return nil, errors.New("local stop key unavailable")
	}
	atomReply, err := xproto.InternAtom(conn, false, uint16(len(attendedInputOwnerAtom)), attendedInputOwnerAtom).Reply()
	if err != nil {
		return nil, errors.New("input owner unavailable")
	}
	window, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, errors.New("input owner window unavailable")
	}
	if err = xproto.CreateWindowChecked(conn, 0, window, root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0, 0, nil).Check(); err != nil {
		return nil, errors.New("input owner window unavailable")
	}
	// A short X-server critical section makes selection acquisition atomic across
	// cooperating processes. No dialog, network or backend operation occurs here.
	s := &x11AttendedSafety{conn: conn, window: window, key: xproto.Keycode(key), stopped: make(chan struct{}), done: make(chan struct{})}
	for _, screen := range setup.Roots {
		s.roots = append(s.roots, screen.Root)
	}
	defer func() {
		if !success {
			_ = s.release()
		}
	}()
	if err = xproto.GrabServerChecked(conn).Check(); err != nil {
		return nil, errors.New("input ownership unavailable")
	}
	owner, ownerErr := xproto.GetSelectionOwner(conn, atomReply.Atom).Reply()
	if ownerErr != nil || owner.Owner != 0 {
		xproto.UngrabServer(conn)
		conn.Sync()
		return nil, errors.New("another Mnet input tool owns this desktop")
	}
	err = xproto.SetSelectionOwnerChecked(conn, window, atomReply.Atom, xproto.TimeCurrentTime).Check()
	xproto.UngrabServer(conn)
	conn.Sync()
	if err != nil {
		return nil, errors.New("input ownership unavailable")
	}
	// Reserve every unrelated modifier combination so Caps/Num/other lock state
	// cannot bypass the shortcut. Any conflict refuses the whole mode; checked
	// release rolls back partial grabs and ownership before closing conn.
	for extra := uint16(0); extra < 256; extra++ {
		if extra&attendedStopModifiers != 0 {
			continue
		}
		for _, screen := range setup.Roots {
			if err = xproto.GrabKeyChecked(conn, false, screen.Root, attendedStopModifiers|extra, xproto.Keycode(key), xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil {
				return nil, errors.New("Ctrl+Alt+Shift+F12 is unavailable; attended hosting refused")
			}
		}
	}
	go func() {
		defer close(s.done)
		defer close(s.stopped)
		for {
			event, err := conn.WaitForEvent()
			if err != nil || event == nil {
				return
			}
			switch e := event.(type) {
			case xproto.KeyPressEvent:
				if e.Detail == xproto.Keycode(key) && e.State&attendedStopModifiers == attendedStopModifiers {
					return
				}
			case xproto.SelectionClearEvent:
				return // Losing the shared owner is terminal, never silently reclaimed.
			case xproto.MappingNotifyEvent:
				return // Keyboard mapping changed, so the reserved shortcut needs recheck.
			}
		}
	}()
	success = true
	return s, nil
}

// Refuse ambiguous/custom modifier layouts rather than advertising a shortcut
// whose printed keys cannot produce our checked mask. Layout changes later
// terminate the session and require a fresh registration.
func attendedStopKey(c *xgb.Conn) byte {
	key := keycodeFor(c, attendedStopKeysym)
	if key == 0 {
		return 0
	}
	plain, err := xproto.GetKeyboardMapping(c, xproto.Keycode(key), 1).Reply()
	if err != nil || len(plain.Keysyms) == 0 || plain.Keysyms[0] != attendedStopKeysym {
		return 0
	}
	modifiers, err := xproto.GetModifierMapping(c).Reply()
	if err != nil || modifiers.KeycodesPerModifier == 0 {
		return 0
	}
	for slot, syms := range map[int][]xproto.Keysym{0: {0xffe1, 0xffe2}, 2: {0xffe3, 0xffe4}, 3: {0xffe9, 0xffea}} {
		found := false
		for j := 0; j < int(modifiers.KeycodesPerModifier); j++ {
			code := modifiers.Keycodes[slot*int(modifiers.KeycodesPerModifier)+j]
			if code == 0 {
				continue
			}
			mapping, err := xproto.GetKeyboardMapping(c, code, 1).Reply()
			if err != nil {
				return 0
			}
			for _, sym := range mapping.Keysyms {
				for _, want := range syms {
					if sym == want {
						found = true
					}
				}
			}
		}
		if !found {
			return 0
		}
	}
	for _, code := range modifiers.Keycodes {
		if code == xproto.Keycode(key) {
			return 0
		}
	}
	return key
}
