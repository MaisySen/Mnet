//go:build linux

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"

	"fintex.au/mnet/internal/session"
)

// X11 screen capture and input injection. Wayland sessions refuse
// capture; they need a portal backend, which is later work.

type x11Host struct {
	conn  *xgb.Conn
	root  xproto.Window
	w, h  int
	scale int // downscale factor: frames are capped on the long edge
	last  []byte
	jpegQ int
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
	return &x11Host{conn: c, root: root, w: w, h: h, scale: capScale(w, h), jpegQ: 70}, nil
}

func newX11Injector() (session.Injector, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("X11 connect (set DISPLAY): %w", err)
	}
	if err := xtest.Init(c); err != nil {
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

func (x *x11Host) Capture() ([]byte, bool, error) {
	reply, err := xproto.GetImage(x.conn, xproto.ImageFormatZPixmap, xproto.Drawable(x.root),
		0, 0, uint16(x.w), uint16(x.h), 0xffffffff).Reply()
	if err != nil {
		return nil, false, err
	}
	// ZPixmap on a 24/32bpp TrueColor screen: BGRX byte order.
	src := reply.Data
	n := x.w * x.h
	if len(src) < n*4 {
		return nil, false, fmt.Errorf("short image: %d bytes for %dx%d", len(src), x.w, x.h)
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
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: x.jpegQ}); err != nil {
		return nil, false, err
	}
	changed := !bytes.Equal(buf.Bytes(), x.last)
	x.last = buf.Bytes()
	return buf.Bytes(), changed, nil
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
	if x.conn == nil {
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
				xtest.FakeInput(x.conn, xproto.ButtonPress, btn, 0, x.root, 0, 0, 0)
			}
			if ev.Action == "up" || ev.Action == "click" {
				xtest.FakeInput(x.conn, xproto.ButtonRelease, btn, 0, x.root, 0, 0, 0)
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
		// Key text arrives as characters; map via the keyboard's own
		// layout using keysyms for ASCII, the common case.
		for _, r := range ev.Text {
			code, ok := keysymFor(r)
			if !ok {
				continue
			}
			kc := keycodeFor(x.conn, code)
			if kc == 0 {
				continue
			}
			xtest.FakeInput(x.conn, xproto.KeyPress, kc, 0, x.root, 0, 0, 0)
			xtest.FakeInput(x.conn, xproto.KeyRelease, kc, 0, x.root, 0, 0, 0)
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

// keysymFor maps ASCII runes to X keysyms. ASCII keysyms equal their
// rune values; space and a few punctuation marks are covered by the
// same rule.
func keysymFor(r rune) (xproto.Keysym, bool) {
	if r >= 0x20 && r < 0x7f {
		return xproto.Keysym(r), true
	}
	return 0, false
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
