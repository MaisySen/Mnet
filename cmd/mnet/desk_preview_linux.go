//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// A static file-only inspector. No daemon, transport, capture or input owner is
// reachable here. Native events control only this local geometry diagram.
func showDeskLayoutPreview(ctx context.Context, p *deskLayoutPreview) error {
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return errors.New("layout preview requires local X11, no fallback")
	}
	if ctx == nil || ctx.Err() != nil || p == nil {
		return errors.New("offline diagram unavailable")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return errors.New("layout preview requires local Linux X11")
	}
	defer c.Close()
	setup := xproto.Setup(c)
	format := false
	for _, f := range setup.PixmapFormats {
		if f.Depth == 24 && f.BitsPerPixel == 32 && f.ScanlinePad == 32 {
			format = true
		}
	}
	screen := setup.DefaultScreen(c)
	if !nativeControlsRGB(screen) {
		return errors.New("native controls require supported 24-bit TrueColor X11")
	}
	if !format || setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return errors.New("offline diagram pixel format unsupported")
	}
	if screen.WidthInPixels < 940 || screen.HeightInPixels < 690 {
		return errors.New("layout preview requires at least 940x690 pixels")
	}
	if err = nativeControlsWatchRoot(c, screen); err != nil {
		return err
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 900, 650, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x17202e, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return err
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), 0, nil).Check(); err != nil {
		return err
	}
	atom, err := xproto.InternAtom(c, false, 16, "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return err
	}
	protocol, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(atom.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocol.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return err
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		return err
	}
	owner, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan deviceEvent, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			ev, er := c.WaitForEvent()
			select {
			case events <- deviceEvent{ev, er}:
			case <-owner.Done():
				return
			}
			if er != nil || ev == nil {
				return
			}
		}
	}()
	defer func() { cancel(); c.Close(); <-done }()
	labels := []string{"PREVIOUS", "NEXT", "LEFT", "RIGHT", "TOP", "BOTTOM", "-PIXEL", "+PIXEL", "CLOSE", "DETAILS"}
	focus, pressed := -1, -1
	details := false
	var held uint32
	clear := func() { pressed = -1; held = 0 }
	hit := func(x, y int16) int {
		if index := p.hitDisplay(840, 250, int64(x)-30, int64(y)-100); index >= 0 {
			return len(labels) + index
		}
		if x >= 730 && x < 880 && y >= 64 && y < 92 {
			return 9
		}
		if y < 580 || y >= 624 {
			return -1
		}
		for i := 0; i < 9; i++ {
			left := int16(20 + i*96)
			if x >= left && x < left+92 {
				return i
			}
		}
		return -1
	}
	draw := func() error {
		picture, er := renderDeskSurface(p, focus, details)
		if er != nil {
			return er
		}
		pix, er := xproto.NewPixmapId(c)
		if er != nil {
			return er
		}
		if er = xproto.CreatePixmapChecked(c, 24, pix, xproto.Drawable(w), 900, 650).Check(); er != nil {
			return er
		}
		defer xproto.FreePixmap(c, pix)
		sampler := newViewerPixelSource(picture, picture.Bounds(), 900, 650)
		uploads := make([]xproto.PutImageCookie, 0, 41)
		for y := 0; y < 650; y += 16 {
			n := min(16, 650-y)
			uploads = append(uploads, xproto.PutImageChecked(c, xproto.ImageFormatZPixmap, xproto.Drawable(pix), gc, 900, uint16(n), 0, int16(y), 0, 24, sampler.strip(y, n)))
		}
		for _, upload := range uploads {
			if er = upload.Check(); er != nil {
				return er
			}
		}
		if er = xproto.ChangeWindowAttributesChecked(c, w, xproto.CwBackPixmap, []uint32{uint32(pix)}).Check(); er != nil {
			return er
		}
		if er = xproto.CopyAreaChecked(c, xproto.Drawable(pix), xproto.Drawable(w), gc, 0, 0, 0, 0, 900, 650).Check(); er != nil {
			return er
		}
		candidate, ok, er := p.candidate()
		if er != nil {
			return er
		}
		title := fmt.Sprintf("Mnet Shared Desk: offline layout details%t display%d edge%s pixel%d", details, p.selected+1, p.edge, p.along)
		if ok {
			title += fmt.Sprintf(" candidate=%s@%d,%d", candidate.Display, candidate.X, candidate.Y)
		} else {
			title += " no-neighbor"
		}
		return xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
	}
	activate := func(i int) bool {
		clear()
		if i >= len(labels) {
			index := i - len(labels)
			if index < len(p.layout.Displays) {
				p.selected = index
				p.along = 0
				focus = -1
			}
			return false
		}
		switch i {
		case 0:
			p.selectDisplay(-1)
		case 1:
			p.selectDisplay(1)
		case 2, 3, 4, 5:
			p.selectEdge([]desk.Edge{desk.Left, desk.Right, desk.Top, desk.Bottom}[i-2])
		case 6:
			p.move(-1)
		case 7:
			p.move(1)
		case 8:
			return true
		case 9:
			details = !details
		}
		return false
	}
	if err = draw(); err != nil {
		return err
	}
	for {
		beforeSelected, beforeAlong, beforeEdge, beforeFocus, beforeDetails := p.selected, p.along, p.edge, focus, details
		expose := false
		index := -1
		select {
		case <-owner.Done():
			return nil
		case ev := <-events:
			if ev.err != nil || ev.event == nil {
				return errors.New("offline diagram display ended")
			}
			switch e := ev.event.(type) {
			case xproto.ExposeEvent:
				expose = true
			case xproto.FocusOutEvent:
				clear()
				focus = -1
			case xproto.ButtonPressEvent:
				clear()
				if e.Detail == 1 {
					pressed = hit(e.EventX, e.EventY)
				}
			case xproto.ButtonReleaseEvent:
				old, key := pressed, held
				clear()
				if key == 0 && e.Detail == 1 && old >= 0 && hit(e.EventX, e.EventY) == old {
					index = old
				}
			case xproto.KeyPressEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return errors.New("offline diagram keyboard unavailable")
				}
				sym := uint32(mapping.Keysyms[0])
				clear()
				switch sym {
				case 0xff1b:
					return nil
				case 0xff09, 0xfe20:
					step := 1
					if e.State&xproto.ModMaskShift != 0 || sym == 0xfe20 {
						step = -1
						if focus < 0 {
							focus = 0
						}
					}
					focus = (focus + step + len(labels)) % len(labels)
				case 0xff0d, 0x20:
					if focus >= 0 {
						pressed = focus
						held = sym
					}
				case 0xff51, 0xff53:
					step := int64(1)
					if e.State&xproto.ModMaskShift != 0 {
						step = 10
					}
					if sym == 0xff51 {
						step = -step
					}
					p.move(step)
				case 0xff50:
					p.along = 0
				case 0xff57:
					p.along = p.extent() - 1
				case 0xff55:
					p.selectDisplay(-1)
				case 0xff56:
					p.selectDisplay(1)
				}
			case xproto.KeyReleaseEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return errors.New("offline diagram keyboard unavailable")
				}
				old, key := pressed, held
				clear()
				if key != 0 && uint32(mapping.Keysyms[0]) == key && old == focus {
					index = old
				}
			case xproto.ConfigureNotifyEvent:
				if nativeControlsRootChanged(screen, e) {
					return errors.New("native controls root geometry changed")
				}
				if e.Window == w && (e.Width != 900 || e.Height != 650) {
					return errors.New("offline diagram unsupported resize")
				}
			case xproto.DestroyNotifyEvent:
				return nil
			case xproto.ClientMessageEvent:
				if e.Type == protocol.Atom && e.Format == 32 && xproto.Atom(e.Data.Data32[0]) == atom.Atom {
					return nil
				}
			default:
				continue
			}
		}
		if index >= 0 && activate(index) {
			return nil
		}
		if expose || p.selected != beforeSelected || p.along != beforeAlong || p.edge != beforeEdge || focus != beforeFocus || details != beforeDetails {
			if err = draw(); err != nil {
				return err
			}
		}
	}
}
