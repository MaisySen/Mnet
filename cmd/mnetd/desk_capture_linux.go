//go:build linux

package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Explicit forwarding capture for the first Linux slice. Start only after an
// exact approved destination is ready. Automatic local edge activation is not
// implemented here. X11 does not distinguish hardware from synthetic events.
// No hooks, remapping, clipboard or screen capture are installed.
type deskPrimaryCapture struct {
	conn      *xgb.Conn
	safety    attendedSafety
	once      sync.Once
	stopped   atomic.Bool
	forwarded atomic.Uint64
	done      chan struct{}
	err       error
	runErr    error
	board     *desk.Switchboard
}

func newDeskPrimaryCapture(board *desk.Switchboard, keyboard string, live func() bool) (*deskPrimaryCapture, error) {
	if board == nil || live == nil || !live() {
		return nil, errors.New("desk capture prerequisites absent")
	}
	target, remote := board.Destination()
	if !remote {
		return nil, errors.New("desk destination not ready")
	}
	safety, err := newDeskSafety(keyboard)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			safety.Close()
		}
	}()
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			conn.Close()
		}
	}()
	actual, err := deskKeyboardFingerprint(conn)
	if err != nil || actual != keyboard || keyboard == "" {
		return nil, errors.New("desk source keyboard differs")
	}
	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)
	if len(setup.Roots) != 1 || screen.WidthInPixels < 64 || screen.HeightInPixels < 64 || screen.WidthInPixels > 32767 || screen.HeightInPixels > 32767 {
		return nil, errors.New("desk primary display geometry unsupported")
	}
	keys, err := xproto.QueryKeymap(conn).Reply()
	if err != nil {
		return nil, err
	}
	for _, held := range keys.Keys {
		if held != 0 {
			return nil, errors.New("desk primary preheld keys refused")
		}
	}
	pointer, err := xproto.QueryPointer(conn, screen.Root).Reply()
	if err != nil || pointer.Mask&0x1fff != 0 {
		return nil, errors.New("desk primary preheld buttons/modifiers refused")
	}
	if err = xproto.ChangeWindowAttributesChecked(conn, screen.Root, xproto.CwEventMask, []uint32{xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskPointerMotion | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease}).Check(); err != nil {
		return nil, err
	}
	keyGrab, err := xproto.GrabKeyboard(conn, false, screen.Root, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
	if err != nil || keyGrab.Status != xproto.GrabStatusSuccess {
		return nil, errors.New("desk primary keyboard grab refused")
	}
	defer func() {
		if !success {
			xproto.UngrabKeyboardChecked(conn, xproto.TimeCurrentTime).Check()
		}
	}()
	mask := uint16(xproto.EventMaskPointerMotion | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease)
	pointerGrab, err := xproto.GrabPointer(conn, false, screen.Root, mask, xproto.GrabModeAsync, xproto.GrabModeAsync, screen.Root, 0, xproto.TimeCurrentTime).Reply()
	if err != nil || pointerGrab.Status != xproto.GrabStatusSuccess {
		return nil, errors.New("desk primary pointer grab refused")
	}
	c := &deskPrimaryCapture{conn: conn, safety: safety, done: make(chan struct{}), board: board}
	centerX, centerY := int16(screen.WidthInPixels/2), int16(screen.HeightInPixels/2)
	if err = xproto.WarpPointerChecked(conn, 0, screen.Root, 0, 0, 0, 0, centerX, centerY).Check(); err != nil {
		c.disable()
		return nil, err
	}
	go func() {
		defer close(c.done)
		defer board.Stop()
		defer c.disable()
		lastX, lastY := centerX, centerY
		var sequence uint64 = 1
		x, y := board.Position()
		var held [256]bool
		stopKey := xproto.Keycode(attendedStopKey(conn))
		for {
			event, err := conn.WaitForEvent()
			if err != nil || event == nil {
				return
			}
			if c.stopped.Load() || !live() {
				return
			}
			var input desk.Input
			switch e := event.(type) {
			case xproto.KeyPressEvent:
				if e.Detail == stopKey && e.State&attendedStopModifiers == attendedStopModifiers {
					return
				}
				action := "down"
				if held[e.Detail] {
					action = "repeat"
				}
				held[e.Detail] = true
				input = desk.Input{Kind: "key", Action: action, Code: uint16(e.Detail)}
			case xproto.KeyReleaseEvent:
				held[e.Detail] = false
				input = desk.Input{Kind: "key", Action: "up", Code: uint16(e.Detail)}
			case xproto.ButtonPressEvent:
				input = desk.Input{Kind: "button", Action: "down", Code: uint16(e.Detail)}
			case xproto.ButtonReleaseEvent:
				input = desk.Input{Kind: "button", Action: "up", Code: uint16(e.Detail)}
			case xproto.MappingNotifyEvent:
				actual, err := deskKeyboardFingerprint(conn)
				if err == nil && actual == keyboard {
					continue
				}
				c.runErr = errors.New("desk source map changed")
				return
			case xproto.MotionNotifyEvent:
				if e.RootX == centerX && e.RootY == centerY {
					lastX, lastY = centerX, centerY
					continue
				}
				// First slice clamps motion at the destination. Cross-device automatic
				// edge activation and local-return pointer placement remain separate work.
				x = max(0, min(target.PixelWidth-1, x+int64(e.RootX-lastX)))
				y = max(0, min(target.PixelHeight-1, y+int64(e.RootY-lastY)))
				lastX, lastY = e.RootX, e.RootY
				input = desk.Input{Kind: "pointer", Action: "move", X: x, Y: y}
				if xproto.WarpPointerChecked(conn, 0, screen.Root, 0, 0, 0, 0, centerX, centerY).Check() != nil {
					return
				}
			default:
				continue
			}
			sequence++
			input.Sequence = sequence
			if err := board.Apply(input); err != nil {
				c.runErr = err
				return
			}
			c.forwarded.Add(1)
		}
	}()
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-c.done:
				return
			case <-safety.Stopped():
				c.disable()
				return
			case <-tick.C:
				if !live() {
					c.disable()
					return
				}
			}
		}
	}()
	success = true
	return c, nil
}

// disable is non-reentrant with the board: local grabs disappear before any
// remote cleanup is attempted. Close joins both event-loop and owned cleanup.
func (c *deskPrimaryCapture) disable() {
	c.once.Do(func() {
		c.stopped.Store(true)
		c.err = errors.Join(c.err, xproto.UngrabKeyboardChecked(c.conn, xproto.TimeCurrentTime).Check(), xproto.UngrabPointerChecked(c.conn, xproto.TimeCurrentTime).Check())
		c.conn.Close()
		c.err = errors.Join(c.err, c.safety.Close())
	})
}
func (c *deskPrimaryCapture) Close() error {
	c.disable()
	<-c.done
	return errors.Join(c.err, c.runErr, c.board.Stop())
}
