//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// Native Shared Desk adapter. No logical text/remapping/clipboard/capture is
// involved. This is not yet exposed as a paired consumer service. A surviving
// process owns cleanup; process SIGKILL is explicitly not solved here.
type deskNativeInput struct {
	mu            sync.Mutex
	conn          *xgb.Conn
	root          xproto.Window
	width, height int64
	keys          [256]bool
	buttons       [6]bool
	retired       bool
	safety        attendedSafety
	owner         *attendedInjectorOwner
	stop          chan struct{}
	done          chan struct{}
	once          sync.Once
	err           error
}

func deskKeyboardFingerprint(c *xgb.Conn) (string, error) {
	setup := xproto.Setup(c)
	keys, err := xproto.GetKeyboardMapping(c, setup.MinKeycode, byte(setup.MaxKeycode-setup.MinKeycode+1)).Reply()
	if err != nil {
		return "", err
	}
	modifiers, err := xproto.GetModifierMapping(c).Reply()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal([]any{setup.MinKeycode, setup.MaxKeycode, keys.KeysymsPerKeycode, keys.Keysyms, modifiers.KeycodesPerModifier, modifiers.Keycodes})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
func newDeskNativeInput(expectedKeyboard string, live func() bool) (*deskNativeInput, error) {
	if live == nil || !live() {
		return nil, errors.New("desk pair is not current")
	}
	safety, err := newDeskSafety(expectedKeyboard)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			safety.Close()
		}
	}()
	c, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			c.Close()
		}
	}()
	actual, err := deskKeyboardFingerprint(c)
	if err != nil || expectedKeyboard == "" || actual != expectedKeyboard {
		return nil, errors.New("desk keyboard map differs; raw input unavailable")
	}
	if err = xtest.Init(c); err != nil {
		return nil, err
	}
	if len(xproto.Setup(c).Roots) != 1 {
		return nil, errors.New("desk native multiple X screens unsupported")
	}
	screen := xproto.Setup(c).DefaultScreen(c)
	if screen.WidthInPixels > 32767 || screen.HeightInPixels > 32767 {
		return nil, errors.New("desk native signed coordinate range unsupported")
	}
	keymap, err := xproto.QueryKeymap(c).Reply()
	if err != nil {
		return nil, err
	}
	for _, mask := range keymap.Keys {
		if mask != 0 {
			return nil, errors.New("desk input preheld keyboard refused")
		}
	}
	pointer, err := xproto.QueryPointer(c, screen.Root).Reply()
	if err != nil || pointer.Mask&0x1fff != 0 {
		return nil, errors.New("desk input preheld pointer/modifier refused")
	}
	n := &deskNativeInput{conn: c, root: screen.Root, width: int64(screen.WidthInPixels), height: int64(screen.HeightInPixels), safety: safety, stop: make(chan struct{}), done: make(chan struct{})}
	owner, err := claimAttendedInjectorOwner(func() { n.once.Do(func() { close(n.stop) }) })
	if err != nil {
		return nil, err
	}
	n.owner = owner
	go func() {
		defer close(n.done)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-n.stop:
				n.release()
				return
			case <-safety.Stopped():
				n.release()
				return
			case <-tick.C:
				geometry, err := xproto.GetGeometry(c, xproto.Drawable(n.root)).Reply()
				if !live() || err != nil || int64(geometry.Width) != n.width || int64(geometry.Height) != n.height {
					n.release()
					return
				}
			}
		}
	}()
	success = true
	return n, nil
}
func (n *deskNativeInput) Apply(ev desk.Input) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	select {
	case <-n.safety.Stopped():
		return errors.New("desk native stop activated")
	case <-n.stop:
		return errors.New("desk native retired")
	default:
	}
	if n.retired {
		return errors.New("desk native input retired")
	}
	switch ev.Kind {
	case "pointer":
		if ev.Action != "move" || ev.X < 0 || ev.Y < 0 || ev.X >= n.width || ev.Y >= n.height {
			return errors.New("desk native pointer outside display")
		}
		return xtest.FakeInputChecked(n.conn, xproto.MotionNotify, 0, 0, n.root, int16(ev.X), int16(ev.Y), 0).Check()
	case "key":
		if ev.Code < 8 || ev.Code > 255 {
			return errors.New("desk native key invalid")
		}
		code := byte(ev.Code)
		if ev.Action == "up" && !n.keys[code] || ev.Action == "down" && n.keys[code] || ev.Action == "repeat" && !n.keys[code] {
			return errors.New("desk native key ownership mismatch")
		}
		if ev.Action == "down" {
			state, err := xproto.QueryKeymap(n.conn).Reply()
			if err != nil {
				return err
			}
			if state.Keys[code/8]&(1<<(code%8)) != 0 {
				return errors.New("desk native foreign preheld key")
			}
		}
		kind := byte(xproto.KeyPress)
		if ev.Action == "up" {
			kind = xproto.KeyRelease
		} else if ev.Action != "down" && ev.Action != "repeat" {
			return errors.New("invalid native key action")
		}
		if err := xtest.FakeInputChecked(n.conn, kind, code, 0, n.root, 0, 0, 0).Check(); err != nil {
			return err
		}
		n.keys[code] = ev.Action != "up"
		return nil
	case "button":
		if ev.Code < 1 || ev.Code > 5 {
			return errors.New("desk native button invalid")
		}
		code := byte(ev.Code)
		if ev.Action == "up" && !n.buttons[code] || ev.Action == "down" && n.buttons[code] {
			return errors.New("desk native button ownership mismatch")
		}
		if ev.Action == "down" {
			state, err := xproto.QueryPointer(n.conn, n.root).Reply()
			if err != nil {
				return err
			}
			if state.Mask&(1<<uint(code+7)) != 0 {
				return errors.New("desk native foreign preheld button")
			}
		}
		kind := byte(xproto.ButtonPress)
		if ev.Action == "up" {
			kind = xproto.ButtonRelease
		} else if ev.Action != "down" {
			return errors.New("invalid native button action")
		}
		if err := xtest.FakeInputChecked(n.conn, kind, code, 0, n.root, 0, 0, 0).Check(); err != nil {
			return err
		}
		n.buttons[code] = ev.Action != "up"
		return nil
	default:
		return errors.New("unsupported desk native input")
	}
}
func (n *deskNativeInput) release() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.retired = true
	for code, held := range n.keys {
		if held {
			err := xtest.FakeInputChecked(n.conn, xproto.KeyRelease, byte(code), 0, n.root, 0, 0, 0).Check()
			n.err = errors.Join(n.err, err)
			if err == nil {
				n.keys[code] = false
			}
		}
	}
	for code, held := range n.buttons {
		if held {
			err := xtest.FakeInputChecked(n.conn, xproto.ButtonRelease, byte(code), 0, n.root, 0, 0, 0).Check()
			n.err = errors.Join(n.err, err)
			if err == nil {
				n.buttons[code] = false
			}
		}
	}
	n.err = errors.Join(n.err, n.owner.Close(), n.safety.Close())
	n.conn.Close()
}
func (n *deskNativeInput) Release() error {
	n.once.Do(func() { close(n.stop) })
	<-n.done
	return n.err
}

func newDeskSafety(keyboard string) (attendedSafety, error) {
	return newNativeInputSafety(func(c *xgb.Conn) bool {
		actual, err := deskKeyboardFingerprint(c)
		return err == nil && keyboard != "" && actual == keyboard
	})
}

func (n *deskNativeInput) Stopped() <-chan struct{} { return n.done }
