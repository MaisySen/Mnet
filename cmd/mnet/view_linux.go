//go:build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"os"
	"strings"
	"time"

	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type xDesktopView struct {
	viewport                     viewerViewport
	lastProbe                    time.Time
	conn                         *xgb.Conn
	window                       xproto.Window
	gc                           xproto.Gcontext
	deleteAtom                   xproto.Atom
	width, height                int
	maxRowsBytes                 int
	image                        *image.RGBA
	state                        string
	title                        string
	closed                       bool
	pointer                      bool
	pointerGrant                 session.ControlStatus
	pointerActions               []desktopPointerAction
	pointerPressed               byte
	pointerPressGrant            session.ControlStatus
	pointerPressX, pointerPressY int
}

func safeViewerLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 80 {
			break
		}
		if r >= 32 && r <= 126 {
			b.WriteByte(byte(r))
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

func newDesktopView(peer string) (desktopView, error)    { return newDesktopViewMode(peer, false) }
func newDesktopPointer(peer string) (desktopView, error) { return newDesktopViewMode(peer, true) }
func newDesktopViewMode(peer string, pointer bool) (desktopView, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" || os.Getenv("DISPLAY") == "" {
		return nil, errors.New("native desktop viewing requires a local X11 display; no fallback")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("viewer X11 unavailable: %w", err)
	}
	fail := func(e error) (desktopView, error) { c.Close(); return nil, e }
	setup := xproto.Setup(c)
	screen := setup.DefaultScreen(c)
	supported := false
	for _, depth := range screen.AllowedDepths {
		for _, visual := range depth.Visuals {
			if visual.VisualId == screen.RootVisual && depth.Depth == 24 && visual.Class == xproto.VisualClassTrueColor && visual.RedMask == 0xff0000 && visual.GreenMask == 0xff00 && visual.BlueMask == 0xff {
				supported = true
			}
		}
	}
	format := false
	for _, f := range setup.PixmapFormats {
		if f.Depth == 24 && f.BitsPerPixel == 32 && f.ScanlinePad == 32 {
			format = true
		}
	}
	if !supported || !format || setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return fail(errors.New("viewer requires supported 24-bit TrueColor X11 format"))
	}
	w, h := int(screen.WidthInPixels)-64, int(screen.HeightInPixels)-96
	if w > 1024 {
		w = 1024
	}
	if h > 800 {
		h = 800
	}
	if w < 160 || h < 120 {
		return fail(errors.New("viewer local display is too small"))
	}
	window, err := xproto.NewWindowId(c)
	if err != nil {
		return fail(err)
	}
	v := &xDesktopView{conn: c, window: window, width: w, height: h, maxRowsBytes: int(setup.MaximumRequestLength)*4 - 32, title: "Mnet view only: " + safeViewerLabel(peer), pointer: pointer}
	if pointer {
		v.title = "Mnet pointer only: " + safeViewerLabel(peer)
		v.pointerGrant.State = "unavailable"
	}
	if err = xproto.CreateWindowChecked(c, 24, window, screen.Root, 20, 20, uint16(w), uint16(h), 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x151923, xproto.EventMaskExposure | xproto.EventMaskStructureNotify | xproto.EventMaskKeyPress | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskPointerMotion | xproto.EventMaskFocusChange}).Check(); err != nil {
		return fail(err)
	}
	cleanup := func(e error) (desktopView, error) { v.Close(); return nil, e }
	v.gc, err = xproto.NewGcontextId(c)
	if err != nil {
		return cleanup(err)
	}
	if err = xproto.CreateGCChecked(c, v.gc, xproto.Drawable(window), xproto.GcForeground|xproto.GcBackground, []uint32{0xffffff, 0x151923}).Check(); err != nil {
		return cleanup(err)
	}
	protocols, err := xproto.InternAtom(c, false, uint16(len("WM_PROTOCOLS")), "WM_PROTOCOLS").Reply()
	if err != nil {
		return cleanup(err)
	}
	del, err := xproto.InternAtom(c, false, uint16(len("WM_DELETE_WINDOW")), "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return cleanup(err)
	}
	v.deleteAtom = del.Atom
	data := make([]byte, 4)
	xgb.Put32(data, uint32(del.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, window, protocols.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return cleanup(err)
	}
	if err = v.State("Starting. View only. Escape ends."); err != nil {
		return cleanup(err)
	}
	if err = xproto.MapWindowChecked(c, window).Check(); err != nil {
		return cleanup(err)
	}
	return v, nil
}

func (v *xDesktopView) State(state string) error {
	if v.closed {
		return errors.New("viewer closed")
	}
	v.state = state
	mode := "Fit"
	if v.viewport.one {
		mode = "1:1 received"
	}
	title := v.title + " | " + state + " [" + mode + "]"
	if v.pointer {
		title += " [pointer " + v.pointerGrant.State + "]"
	}
	// Publish matching pixels before advertising the corresponding title state.
	// Native callers/assistive UI must not observe an allowed/revoked title while
	// the visible header still shows the preceding state.
	if err := v.draw(); err != nil {
		return err
	}
	return xproto.ChangePropertyChecked(v.conn, xproto.PropModeReplace, v.window, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
}

func (v *xDesktopView) Frame(fr session.ScreenFrame) error {
	if v.closed {
		return errors.New("viewer closed")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(fr.Image))
	if err != nil {
		return errors.New("viewer invalid screen image")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || cfg.Width > 16*1024*1024/cfg.Height {
		return errors.New("viewer screen exceeds image policy")
	}
	img, err := jpeg.Decode(bytes.NewReader(fr.Image))
	if err != nil {
		return errors.New("viewer invalid screen image")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	if v.image == nil || v.image.Bounds() != rgba.Bounds() {
		v.pointerPressed = 0
	}
	v.image = rgba
	v.state = "Viewing. Last screen " + time.Now().Format("15:04:05") + ". View only. Escape ends."
	if v.pointer {
		v.state = "Screen " + time.Now().Format("15:04:05") + ". Pointer: " + v.pointerGrant.State + ". No keyboard/dragging."
	}
	return v.State(v.state)
}

func fitView(sw, sh, dw, dh int) (int, int) {
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return 0, 0
	}
	if int64(sw)*int64(dh) > int64(sh)*int64(dw) {
		return dw, max(1, int(int64(sh)*int64(dw)/int64(sw)))
	}
	return max(1, int(int64(sw)*int64(dh)/int64(sh))), dh
}

func (v *xDesktopView) draw() error {
	if v.closed {
		return errors.New("viewer closed")
	}
	if v.width < 1 || v.height < 1 || v.width > 8192 || v.height > 8192 || v.width > 16*1024*1024/v.height {
		return errors.New("viewer window exceeds rendering policy")
	}
	// Compose off-screen, then copy once. Clearing the visible window before
	// multiple checked image strips makes valid sessions flash blank/partial.
	pix, err := xproto.NewPixmapId(v.conn)
	if err != nil {
		return err
	}
	if err = xproto.CreatePixmapChecked(v.conn, 24, pix, xproto.Drawable(v.window), uint16(v.width), uint16(v.height)).Check(); err != nil {
		return err
	}
	defer func() { _ = xproto.FreePixmapChecked(v.conn, pix).Check() }()
	target := xproto.Drawable(pix)
	if err = xproto.ChangeGCChecked(v.conn, v.gc, xproto.GcForeground, []uint32{0x151923}).Check(); err != nil {
		return err
	}
	if err = xproto.PolyFillRectangleChecked(v.conn, target, v.gc, []xproto.Rectangle{{Width: uint16(v.width), Height: uint16(v.height)}}).Check(); err != nil {
		return err
	}
	if err = xproto.ChangeGCChecked(v.conn, v.gc, xproto.GcForeground, []uint32{0xffffff}).Check(); err != nil {
		return err
	}
	mode := "Fit. 1: received pixels 1:1; arrows/Home/End: pan; F: fit; Escape: end"
	if v.viewport.one {
		mode = "1:1 received pixels. Arrows/Home/End: pan; F: fit; Escape: end"
	}
	if v.pointer {
		mode = "F8: ask pointer; F9: release; Escape: end. 1/F/arrows: local view."
	}
	for i, text := range []string{safeViewerLabel(v.title), safeViewerLabel(v.state), mode} {
		if err = xproto.ImageText8Checked(v.conn, byte(len(text)), target, v.gc, 10, int16(18+i*20), text).Check(); err != nil {
			return err
		}
	}
	publish := func() error {
		return xproto.CopyAreaChecked(v.conn, target, xproto.Drawable(v.window), v.gc, 0, 0, 0, 0, uint16(v.width), uint16(v.height)).Check()
	}
	if v.image == nil || v.height <= viewerHeader {
		return publish()
	}
	b := v.image.Bounds()
	region := v.viewport.region(b.Dx(), b.Dy(), v.width, v.height-viewerHeader)
	w, h := region.target.Dx(), region.target.Dy()
	if w == 0 || h == 0 {
		return publish()
	}
	rows := v.maxRowsBytes / (w * 4)
	if rows < 1 {
		return errors.New("viewer X11 request limit too small")
	}
	dx, dy := region.target.Min.X, viewerHeader+region.target.Min.Y
	for y := 0; y < h; y += rows {
		n := min(rows, h-y)
		pixels := make([]byte, w*n*4)
		for row := 0; row < n; row++ {
			for x := 0; x < w; x++ {
				src := v.image.PixOffset(b.Min.X+region.source.Min.X+x*region.source.Dx()/w, b.Min.Y+region.source.Min.Y+(y+row)*region.source.Dy()/h)
				i := (row*w + x) * 4
				pixels[i], pixels[i+1], pixels[i+2] = v.image.Pix[src+2], v.image.Pix[src+1], v.image.Pix[src]
			}
		}
		if err := xproto.PutImageChecked(v.conn, xproto.ImageFormatZPixmap, target, v.gc, uint16(w), uint16(n), int16(dx), int16(dy+y), 0, 24, pixels).Check(); err != nil {
			return err
		}
	}
	return publish()
}

func (v *xDesktopView) Poll() (bool, error) {
	// XGB discards EOF in PollForEvent. A checked periodic round-trip catches
	// a dead display even when no further frame or local event arrives.
	if !v.closed && time.Since(v.lastProbe) >= time.Second {
		v.lastProbe = time.Now()
		if _, err := xproto.GetInputFocus(v.conn).Reply(); err != nil {
			return false, fmt.Errorf("viewer display ended: %w", err)
		}
	}
	if v.closed {
		return true, nil
	}
	// Bound event work so resize/expose floods cannot starve cancellation.
	for i := 0; i < 64; i++ {
		ev, err := v.conn.PollForEvent()
		if err != nil {
			return false, fmt.Errorf("viewer X11 event failure: %w", err)
		}
		if ev == nil {
			return false, nil
		}
		switch e := ev.(type) {
		case xproto.ClientMessageEvent:
			if e.Window == v.window && e.Data.Data32[0] == uint32(v.deleteAtom) {
				return true, nil
			}
		case xproto.DestroyNotifyEvent:
			if e.Window == v.window {
				return true, nil
			}
		case xproto.FocusOutEvent:
			if v.pointer {
				v.pointerPressed = 0
				if err := v.queuePointer(desktopPointerAction{Kind: "release"}); err != nil {
					return false, err
				}
			}
		case xproto.ButtonPressEvent:
			if v.pointer {
				if err := v.pointerButton(byte(e.Detail), true, int(e.EventX), int(e.EventY)); err != nil {
					return false, err
				}
			}
		case xproto.ButtonReleaseEvent:
			if v.pointer {
				if err := v.pointerButton(byte(e.Detail), false, int(e.EventX), int(e.EventY)); err != nil {
					return false, err
				}
			}
		case xproto.MotionNotifyEvent:
			if v.pointer {
				if err := v.pointerMotion(int(e.EventX), int(e.EventY)); err != nil {
					return false, err
				}
			}
		case xproto.KeyPressEvent:
			// Resolve Escape from the actual layout, never forward any local key.
			mapping, err := xproto.GetKeyboardMapping(v.conn, e.Detail, 1).Reply()
			if err != nil {
				return false, err
			}
			if len(mapping.Keysyms) > 0 {
				sym := uint32(mapping.Keysyms[0])
				if v.pointer && (sym == 0xffc5 || sym == 0xffc6) { // F8/F9 stay local.
					kind := "request"
					if sym == 0xffc6 {
						kind = "release"
					}
					if err := v.queuePointer(desktopPointerAction{Kind: kind}); err != nil {
						return false, err
					}
					continue
				}
				if sym == 0xff1b {
					return true, nil
				}
				if v.image != nil && v.viewport.key(sym, v.image.Bounds().Dx(), v.image.Bounds().Dy(), v.width, v.height-viewerHeader) {
					v.pointerPressed = 0
					if err := v.State(v.state); err != nil {
						return false, err
					}
				}
			}
		case xproto.ExposeEvent:
			if e.Count == 0 {
				if err := v.draw(); err != nil {
					return false, err
				}
			}
		case xproto.ConfigureNotifyEvent:
			if e.Window == v.window {
				if v.width != int(e.Width) || v.height != int(e.Height) {
					v.pointerPressed = 0
				}
				v.width, v.height = int(e.Width), int(e.Height)
				if err := v.draw(); err != nil {
					return false, err
				}
			}
		}
	}
	return false, nil
}
func (v *xDesktopView) Close() error {
	if v.closed {
		return nil
	}
	v.closed = true
	v.image = nil
	var errs []error
	if v.gc != 0 {
		errs = append(errs, xproto.FreeGCChecked(v.conn, v.gc).Check())
	}
	if v.window != 0 {
		errs = append(errs, xproto.DestroyWindowChecked(v.conn, v.window).Check())
	}
	v.conn.Close()
	// XGB closes asynchronously; checked resource destruction above owns the
	// server-side postcondition. No claim is made for a nonresponsive X server.
	return errors.Join(errs...)
}
