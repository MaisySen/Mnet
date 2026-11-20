//go:build linux

package main

import (
	"context"
	"errors"
	"strings"
	"sync"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// This local window can withdraw one already-reviewed run, never approve,
// reconnect, select a peer or change a layout. It owns no input grabs.
type deskRunStatus struct {
	ctx                    context.Context
	cancel                 context.CancelFunc
	conn                   *xgb.Conn
	window                 xproto.Window
	gc                     xproto.Gcontext
	deleteAtom             xproto.Atom
	escapeKey              xproto.Keycode
	mu                     sync.Mutex
	role, run, peer, state string
	err                    error
	done, watchDone        chan struct{}
}

func deskStatusLine(role, state string) (string, error) {
	switch role + ":" + state {
	case "primary:armed":
		return "ARMED: Input stays local until a fresh unheld edge arrival.", nil
	case "primary:connecting":
		return "CONNECTING: Checking the exact recipient. No input yet.", nil
	case "primary:forwarding":
		return "SENDING: Raw keyboard and pointer go to the approved recipient.", nil
	case "recipient:waiting":
		return "WAITING: One exact approved primary may connect.", nil
	case "recipient:ready":
		return "READY: Native input owner prepared; no received input yet.", nil
	case "recipient:receiving":
		return "RECEIVING: Raw input accepted in this one-run pair.", nil
	default:
		return "", errors.New("invalid local Desk status")
	}
}
func deskStatusTransition(role, from, to string) bool {
	if _, err := deskStatusLine(role, to); err != nil {
		return false
	}
	return from == to || role == "primary" && (from == "armed" && to == "connecting" || from == "connecting" && to == "forwarding") || role == "recipient" && (from == "waiting" && to == "ready" || from == "ready" && to == "receiving")
}
func startDeskRunStatus(parent context.Context, spec desk.RunSpec, state string) (*deskRunStatus, error) {
	if parent == nil || parent.Err() != nil {
		return nil, errors.New("desk status owner absent")
	}
	if (spec.Role != "primary" || state != "armed") && (spec.Role != "recipient" || state != "waiting") {
		return nil, errors.New("desk status initial phase invalid")
	}
	// Only exact validated digests are shown, not remote names or markup.
	peer := spec.Binding.Recipient
	if spec.Role == "recipient" {
		peer = spec.Binding.Primary
	}
	for _, value := range []string{spec.Binding.Run, peer} {
		if len(value) != 64 || strings.Trim(value, "0123456789abcdef") != "" {
			return nil, errors.New("desk status exact binding absent")
		}
	}
	c, err := xgb.NewConn()
	if err != nil {
		return nil, errors.New("desk local status unavailable")
	}
	ok := false
	defer func() {
		if !ok {
			c.Close()
		}
	}()
	screen := xproto.Setup(c).DefaultScreen(c)
	if screen.WidthInPixels < 760 || screen.HeightInPixels < 330 {
		return nil, errors.New("desk status requires at least760x330 local pixels")
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return nil, err
	}
	mask := uint32(xproto.CwBackPixel | xproto.CwEventMask)
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 720, 300, 0, xproto.WindowClassInputOutput, screen.RootVisual, mask, []uint32{0x17202e, xproto.EventMaskExposure | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return nil, err
	}
	font, err := xproto.NewFontId(c)
	if err != nil {
		return nil, err
	}
	if err = xproto.OpenFontChecked(c, font, 5, "fixed").Check(); err != nil {
		return nil, err
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return nil, err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{0xecf2fc, 0x17202e, uint32(font)}).Check(); err != nil {
		return nil, err
	}
	atom, err := xproto.InternAtom(c, false, 16, "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return nil, err
	}
	protocols, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return nil, err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(atom.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocols.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return nil, err
	}
	title := "Mnet Shared Desk: live " + spec.Role
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	escape := keycodeFor(c, 0xff1b)
	if escape == 0 {
		cancel()
		return nil, errors.New("desk status Escape key unavailable")
	}
	u := &deskRunStatus{escapeKey: xproto.Keycode(escape), ctx: ctx, cancel: cancel, conn: c, window: w, gc: gc, deleteAtom: atom.Atom, role: spec.Role, run: spec.Binding.Run, peer: peer, state: state, done: make(chan struct{}), watchDone: make(chan struct{})}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		cancel()
		return nil, err
	}
	if err = u.drawLocked(); err != nil {
		cancel()
		return nil, err
	}
	go func() { defer close(u.watchDone); <-ctx.Done(); c.Close() }()
	go u.events()
	ok = true
	return u, nil
}
func (u *deskRunStatus) drawLocked() error {
	state, err := deskStatusLine(u.role, u.state)
	if err != nil {
		return err
	}
	if err = xproto.ChangeGCChecked(u.conn, u.gc, xproto.GcForeground, []uint32{0x17202e}).Check(); err != nil {
		return err
	}
	if err = xproto.PolyFillRectangleChecked(u.conn, xproto.Drawable(u.window), u.gc, []xproto.Rectangle{{Width: 720, Height: 300}}).Check(); err != nil {
		return err
	}
	if err = xproto.ChangeGCChecked(u.conn, u.gc, xproto.GcForeground, []uint32{0xecf2fc}).Check(); err != nil {
		return err
	}
	stopHint := "Source grabs can block this button while forwarding; use the shortcut."
	if u.role == "recipient" {
		stopHint = "This local Stop ends this run; it cannot grant more input access."
	}
	lines := []string{"SHARED DESK / " + strings.ToUpper(u.role), state, "", "One-use run: " + u.run, "Exact other-device key: " + u.peer, "", "Only this already-reviewed run. No auto-reconnect or saved grant.", "Ctrl+Alt+Shift+F12 requests local stop while X11 is responsive.", stopHint, "Stop requests joined cleanup. A failed release remains an error.", "No video, clipboard, files or recording. Engineering X11 only."}
	for n, line := range lines {
		if err = xproto.ImageText8Checked(u.conn, byte(len(line)), xproto.Drawable(u.window), u.gc, 18, int16(25+n*19), line).Check(); err != nil {
			return err
		}
	}
	if err = xproto.ChangeGCChecked(u.conn, u.gc, xproto.GcForeground, []uint32{0x9b343e}).Check(); err != nil {
		return err
	}
	if err = xproto.PolyFillRectangleChecked(u.conn, xproto.Drawable(u.window), u.gc, []xproto.Rectangle{{X: 18, Y: 250, Width: 684, Height: 34}}).Check(); err != nil {
		return err
	}
	if err = xproto.ChangeGCChecked(u.conn, u.gc, xproto.GcForeground|xproto.GcBackground, []uint32{0xffffff, 0x9b343e}).Check(); err != nil {
		return err
	}
	label := "STOP THIS RUN  (or Escape when this window has focus)"
	if err = xproto.ImageText8Checked(u.conn, byte(len(label)), xproto.Drawable(u.window), u.gc, 205, 272, label).Check(); err != nil {
		return err
	}
	if err = xproto.ChangeGCChecked(u.conn, u.gc, xproto.GcBackground, []uint32{0x17202e}).Check(); err != nil {
		return err
	}
	// Publish the label only after its pixels are complete.
	title := "Mnet Shared Desk: live " + u.role + " [" + u.state + "]"
	return xproto.ChangePropertyChecked(u.conn, xproto.PropModeReplace, u.window, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
}
func (u *deskRunStatus) Set(state string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.ctx.Err() != nil {
		return u.ctx.Err()
	}
	if !deskStatusTransition(u.role, u.state, state) {
		err := errors.New("desk status invalid phase transition")
		u.err = err
		u.cancel()
		return err
	}
	if u.state == state {
		return nil
	}
	u.state = state
	if err := u.drawLocked(); err != nil {
		u.err = errors.New("desk status display failed")
		u.cancel()
		return u.err
	}
	return nil
}
func (u *deskRunStatus) events() {
	defer close(u.done)
	defer u.cancel()
	for {
		event, err := u.conn.WaitForEvent()
		if err != nil || event == nil {
			u.mu.Lock()
			if u.ctx.Err() == nil {
				u.err = errors.New("desk status connection ended unexpectedly")
			}
			u.mu.Unlock()
			return
		}
		switch e := event.(type) {
		case xproto.ExposeEvent:
			u.mu.Lock()
			if u.ctx.Err() == nil {
				if err := u.drawLocked(); err != nil {
					u.err = errors.New("desk status repaint failed")
					u.cancel()
				}
			}
			u.mu.Unlock()
		case xproto.ButtonReleaseEvent:
			if e.Detail == 1 && e.EventX >= 18 && e.EventX < 702 && e.EventY >= 250 && e.EventY < 284 {
				return
			}
		case xproto.KeyPressEvent:
			if e.Detail == u.escapeKey {
				return
			} // Local X11 Escape; no remote input command.
		case xproto.ClientMessageEvent:
			if e.Format == 32 && xproto.Atom(e.Data.Data32[0]) == u.deleteAtom {
				return
			}
		case xproto.ConfigureNotifyEvent:
			if e.Window == u.window && (e.Width != 720 || e.Height != 300) {
				return
			}
		case xproto.DestroyNotifyEvent:
			return
		}
	}
}
func (u *deskRunStatus) Close() error {
	u.cancel()
	<-u.watchDone
	<-u.done
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.err
}

type deskStatusInput struct {
	*deskProcessInput
	status *deskRunStatus
}

func (i *deskStatusInput) Apply(ev desk.Input) error {
	if err := i.deskProcessInput.Apply(ev); err != nil {
		return err
	}
	return i.status.Set("receiving")
}
