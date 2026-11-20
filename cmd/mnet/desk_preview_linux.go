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
	screen := xproto.Setup(c).DefaultScreen(c)
	if !nativeControlsRGB(screen) {
		return errors.New("native controls require supported 24-bit TrueColor X11")
	}
	if screen.WidthInPixels < 940 || screen.HeightInPixels < 690 {
		return errors.New("layout preview requires at least 940x690 pixels")
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 900, 650, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x17202e, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return err
	}
	font, err := xproto.NewFontId(c)
	if err != nil {
		return err
	}
	if err = xproto.OpenFontChecked(c, font, 5, "fixed").Check(); err != nil {
		return err
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{0xffffff, 0x17202e, uint32(font)}).Check(); err != nil {
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
	labels := []string{"PREVIOUS", "NEXT", "LEFT", "RIGHT", "TOP", "BOTTOM", "-PIXEL", "+PIXEL", "CLOSE"}
	focus, pressed := -1, -1
	var held uint32
	clear := func() { pressed = -1; held = 0 }
	hit := func(x, y int16) int {
		if y < 580 || y >= 624 {
			return -1
		}
		for i := range labels {
			left := int16(20 + i*96)
			if x >= left && x < left+92 {
				return i
			}
		}
		return -1
	}
	draw := func() error {
		fill := func(color uint32, r xproto.Rectangle) error {
			if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground, []uint32{color}).Check(); er != nil {
				return er
			}
			return xproto.PolyFillRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{r}).Check()
		}
		text := func(x, y int16, label string, bg, fg uint32) error {
			if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground|xproto.GcBackground, []uint32{fg, bg}).Check(); er != nil {
				return er
			}
			return xproto.ImageText8Checked(c, byte(len(label)), xproto.Drawable(w), gc, x, y, label).Check()
		}
		if er := fill(0x17202e, xproto.Rectangle{Width: 900, Height: 650}); er != nil {
			return er
		}
		for i, line := range []string{"SHARED DESK / OFFLINE ARRANGEMENT INSPECTOR", "FILE GEOMETRY ONLY. No active connection, pairing, identity, input or approval.", "Numbered rectangles use one common layout scale. Tiny paint pixels can merge; labels stay exact."} {
			if er := text(20, int16(28+i*24), line, 0x17202e, 0xffffff); er != nil {
				return er
			}
		}
		rects, er := p.rectangles(840, 250)
		if er != nil {
			return er
		}
		for i, r := range rects {
			bg := uint32(0x34465d)
			if i == p.selected {
				bg = 0x0066cc
			}
			if er = fill(bg, xproto.Rectangle{X: int16(30 + r.x), Y: int16(100 + r.y), Width: uint16(r.w), Height: uint16(r.h)}); er != nil {
				return er
			}
			if r.w >= 24 && r.h >= 24 {
				if er = text(int16(34+r.x), int16(117+r.y), fmt.Sprint(i+1), bg, 0xffffff); er != nil {
					return er
				}
			}
		}
		d := p.display()
		r := rects[p.selected]
		mx, my := r.x, r.y
		alongDen := max(int64(1), p.extent()-1)
		switch p.edge {
		case desk.Left:
			my += p.along * (r.h - 1) / alongDen
		case desk.Right:
			mx += r.w - 1
			my += p.along * (r.h - 1) / alongDen
		case desk.Top:
			mx += p.along * (r.w - 1) / alongDen
		case desk.Bottom:
			mx += p.along * (r.w - 1) / alongDen
			my += r.h - 1
		}
		// The marker names a geometry query, never a live pointer or activation edge.
		if er = fill(0xffd166, xproto.Rectangle{X: int16(28 + mx), Y: int16(98 + my), Width: 5, Height: 5}); er != nil {
			return er
		}
		candidate, ok, er := p.candidate()
		if er != nil {
			return er
		}
		lines := []string{fmt.Sprintf("Selected %d/%d: display=%s", p.selected+1, len(p.layout.Displays), d.ID), "Device label=" + d.Device, fmt.Sprintf("Layout origin (%d,%d), size %dx%d; native pixels %dx%d", d.X, d.Y, d.Width, d.Height, d.PixelWidth, d.PixelHeight), fmt.Sprintf("Query edge=%s source pixel=%d of 0..%d. Yellow dot is this query, not live input.", p.edge, p.along, p.extent()-1)}
		if ok {
			lines = append(lines, "CANDIDATE display="+candidate.Display, fmt.Sprintf("Device label=%s native candidate (%d,%d)", candidate.Device, candidate.X, candidate.Y))
		} else {
			lines = append(lines, "NO NEIGHBOR at this exact source pixel (gap/dead corner).", "Neighbor tests use exact original geometry, not rounded diagram rectangles.")
		}
		lines = append(lines, "Offline candidate only. No run, connection or grant. This window never writes the file.", "Tab: controls. Enter/Space: exact release. Arrows: source pixel. Home/End: endpoints.", "Page Up/Down: display. Shift+arrows: 10 pixels. Escape/Close: local window only.")
		for i, line := range lines {
			if er = text(20, int16(377+i*22), line, 0x17202e, 0xffffff); er != nil {
				return er
			}
		}
		for i, label := range labels {
			bg := uint32(0x28384e)
			if i == 8 {
				bg = 0x9b343e
			}
			if i >= 2 && i <= 5 && desk.Edge([]string{"left", "right", "top", "bottom"}[i-2]) == p.edge {
				bg = 0x0066cc
			}
			x := int16(20 + i*96)
			if er = fill(bg, xproto.Rectangle{X: x, Y: 580, Width: 92, Height: 44}); er != nil {
				return er
			}
			if er = text(x+8, 606, label, bg, 0xffffff); er != nil {
				return er
			}
			if i == focus {
				if er = xproto.PolyRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{{X: x + 3, Y: 583, Width: 85, Height: 37}}).Check(); er != nil {
					return er
				}
			}
		}
		title := fmt.Sprintf("Mnet Shared Desk: offline layout display%d edge%s pixel%d", p.selected+1, p.edge, p.along)
		if ok {
			title += fmt.Sprintf(" candidate=%s@%d,%d", candidate.Display, candidate.X, candidate.Y)
		} else {
			title += " no-neighbor"
		}
		return xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
	}
	activate := func(i int) bool {
		clear()
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
		}
		return false
	}
	if err = draw(); err != nil {
		return err
	}
	for {
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
				if e.Width != 900 || e.Height != 650 {
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
		if err = draw(); err != nil {
			return err
		}
	}
}
