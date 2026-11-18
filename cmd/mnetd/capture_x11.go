//go:build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"sync"
	"sync/atomic"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"

	"fintex.au/mnet/internal/session"
)

// X11 screen capture and input injection. Wayland sessions refuse
// capture; they need a portal backend, which is later work.

type x11Host struct {
	inputMu   sync.Mutex
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
	pressed   [256]bool
	conn      *xgb.Conn
	root      xproto.Window
	w, h      int
	scale     int // downscale factor: frames are capped on the long edge
	last      []byte
	lastRGBA  *image.RGBA
	jpegQ     atomic.Int32
	textMode  string
	remapKC   byte
}

func newX11Capturer() (session.Capturer, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("X11 connect (set DISPLAY; Wayland unsupported): %w", err)
	}
	setup := xproto.Setup(c)
	scr := setup.DefaultScreen(c)
	root := scr.Root
	w, h := int(scr.WidthInPixels), int(scr.HeightInPixels)
	xtest.Init(c)
	x := &x11Host{conn: c, root: root, w: w, h: h, scale: capScale(w, h)}
	x.jpegQ.Store(70)
	return x, nil
}

func newX11Injector() (session.Injector, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("X11 connect (set DISPLAY): %w", err)
	}
	if err := xtest.Init(c); err != nil {
		c.Close()
		return nil, err
	}
	return &x11Host{conn: c}, nil
}

// capScale picks a downscale factor so the long edge is <= 1280.
func capScale(w, h int) int {
	long := w
	if h > long {
		long = h
	}
	scale := 1
	for long/scale > 1280 {
		scale++
	}
	return scale
}

func (x *x11Host) Geometry() (int, int) { return x.w / x.scale, x.h / x.scale }

// SetQuality adapts the JPEG encode quality to path conditions (the
// session layer drives it from the measured round trip).
func (x *x11Host) SetQuality(q int) {
	if q < 20 {
		q = 20
	}
	if q > 90 {
		q = 90
	}
	x.jpegQ.Store(int32(q))
}

// grab captures and downscales the screen to one RGBA image.
func (x *x11Host) grab() (*image.RGBA, error) {
	if x.closed.Load() {
		return nil, fmt.Errorf("X11 capture closed")
	}
	reply, err := xproto.GetImage(x.conn, xproto.ImageFormatZPixmap, xproto.Drawable(x.root),
		0, 0, uint16(x.w), uint16(x.h), 0xffffffff).Reply()
	if err != nil {
		return nil, err
	}
	// ZPixmap on a 24/32bpp TrueColor screen: BGRX byte order.
	src := reply.Data
	n := x.w * x.h
	if len(src) < n*4 {
		return nil, fmt.Errorf("short image: %d bytes for %dx%d", len(src), x.w, x.h)
	}
	img := image.NewRGBA(image.Rect(0, 0, x.w, x.h))
	j := 0
	for i := 0; i < n; i++ {
		img.Pix[j+0] = src[i*4+2] // R
		img.Pix[j+1] = src[i*4+1] // G
		img.Pix[j+2] = src[i*4+0] // B
		img.Pix[j+3] = 255
		j += 4
	}
	if x.scale > 1 {
		img = downscale(img, x.scale)
	}
	return img, nil
}

func (x *x11Host) Capture() ([]byte, bool, error) {
	img, err := x.grab()
	if err != nil {
		return nil, false, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: int(x.jpegQ.Load())}); err != nil {
		return nil, false, err
	}
	changed := !bytes.Equal(buf.Bytes(), x.last)
	x.last = buf.Bytes()
	x.lastRGBA = img
	return buf.Bytes(), changed, nil
}

// CaptureDelta sends only changed 64x64 tiles when a minority of the
// screen moved; past 60% changed tiles a full frame is cheaper.
func (x *x11Host) CaptureDelta() ([]byte, []session.Tile, error) {
	img, err := x.grab()
	if err != nil {
		return nil, nil, err
	}
	// A new canvas cannot be sent as deltas against the old dimensions.
	if x.lastRGBA != nil && x.lastRGBA.Bounds() != img.Bounds() {
		x.lastRGBA = nil
	}
	rects := session.DiffTiles(x.lastRGBA, img, 64)
	b := img.Bounds()
	total := ((b.Dx() + 63) / 64) * ((b.Dy() + 63) / 64)
	if x.lastRGBA != nil && len(rects) == 0 {
		return nil, nil, nil
	}
	if x.lastRGBA != nil && len(rects) <= total*3/5 {
		tiles, err := session.EncodeTiles(img, rects, int(x.jpegQ.Load()))
		if err != nil {
			return nil, nil, err
		}
		x.lastRGBA = img
		return nil, tiles, nil
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: int(x.jpegQ.Load())}); err != nil {
		return nil, nil, err
	}
	x.lastRGBA = img
	x.last = buf.Bytes()
	return buf.Bytes(), nil, nil
}

// downscale averages scale-by-scale blocks. Cheap and good enough at
// the factors used here.
func downscale(src *image.RGBA, factor int) *image.RGBA {
	w := src.Rect.Dx() / factor
	h := src.Rect.Dy() / factor
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x0 := 0; x0 < w; x0++ {
			var r, g, b, n int
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					i := src.PixOffset(x0*factor+dx, y*factor+dy)
					r += int(src.Pix[i])
					g += int(src.Pix[i+1])
					b += int(src.Pix[i+2])
					n++
				}
			}
			o := out.PixOffset(x0, y)
			out.Pix[o] = uint8(r / n)
			out.Pix[o+1] = uint8(g / n)
			out.Pix[o+2] = uint8(b / n)
			out.Pix[o+3] = 255
		}
	}
	return out
}

// Inject applies one viewer input event via XTEST. Coordinates arrive
// in scaled frame space; multiply back to screen space.
func (x *x11Host) Inject(ev session.InputEvent) error {
	x.inputMu.Lock()
	defer x.inputMu.Unlock()
	if x.conn == nil || x.closed.Load() {
		return fmt.Errorf("injector not connected")
	}
	scale := 1
	if x.w > 0 && x.scale > 0 {
		scale = x.scale
	}
	switch ev.Kind {
	case "pointer":
		switch ev.Action {
		case "move", "down", "up", "click":
			xtest.FakeInput(x.conn, xproto.MotionNotify, 0, 0, x.root,
				int16(ev.X*scale), int16(ev.Y*scale), 0)
			if ev.Action == "move" {
				return nil
			}
			btn := buttonCode(ev.Button)
			if ev.Action == "down" || ev.Action == "click" {
				if err := xtest.FakeInputChecked(x.conn, xproto.ButtonPress, btn, 0, x.root, 0, 0, 0).Check(); err != nil {
					return err
				}
				x.pressed[btn] = true
			}
			if ev.Action == "up" || ev.Action == "click" {
				if err := xtest.FakeInputChecked(x.conn, xproto.ButtonRelease, btn, 0, x.root, 0, 0, 0).Check(); err != nil {
					return err
				}
				x.pressed[btn] = false
			}
		case "scroll":
			// Buttons 4/5/6/7 are the X scroll wheel.
			btn := byte(5)
			if ev.DY < 0 {
				btn = 4
			}
			xtest.FakeInput(x.conn, xproto.ButtonPress, btn, 0, x.root, 0, 0, 0)
			xtest.FakeInput(x.conn, xproto.ButtonRelease, btn, 0, x.root, 0, 0, 0)
		}
	case "key":
		if ev.Text != "" {
			return x.TypeText(ev.Text)
		}
	}
	x.conn.Sync()
	return nil
}

func buttonCode(name string) byte {
	switch name {
	case "right":
		return 3
	case "middle":
		return 2
	default:
		return 1
	}
}

// keycodeFor finds the keycode that produces a keysym in the current
// keyboard mapping.
func keycodeFor(c *xgb.Conn, sym xproto.Keysym) byte {
	setup := xproto.Setup(c)
	min, max := setup.MinKeycode, setup.MaxKeycode
	mapping, err := xproto.GetKeyboardMapping(c, min, byte(max-min+1)).Reply()
	if err != nil {
		return 0
	}
	per := int(mapping.KeysymsPerKeycode)
	for kc := int(min); kc <= int(max); kc++ {
		for i := 0; i < per; i++ {
			if mapping.Keysyms[(kc-int(min))*per+i] == sym {
				return byte(kc)
			}
		}
	}
	return 0
}

// Close rejects further input, releases buttons this injector left down, then
// closes its dedicated X connection. It is safe for repeated host Stop calls.
// A running platform action is serialized, not forcibly interrupted.
func (x *x11Host) Close() error {
	x.closeOnce.Do(func() {
		x.inputMu.Lock()
		defer x.inputMu.Unlock()
		x.closed.Store(true)
		if x.conn == nil {
			return
		}
		for btn, down := range x.pressed {
			if !down {
				continue
			}
			if err := xtest.FakeInputChecked(x.conn, xproto.ButtonRelease, byte(btn), 0, x.root, 0, 0, 0).Check(); err != nil {
				x.closeErr = errors.Join(x.closeErr, err)
			}
			x.pressed[btn] = false
		}
		x.conn.Sync()
		x.conn.Close()
	})
	return x.closeErr
}
