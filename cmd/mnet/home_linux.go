//go:build linux

package main

import (
	"context"
	"errors"
	"os"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// No API client or file reader enters this window. A closed choice joins the
// event reader before the owner opens one selected child journey.
func chooseHome(parent context.Context, notice string, hasLayout bool) (homeChoice, error) {
	if parent == nil || parent.Err() != nil {
		return homeClose, nil
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return homeClose, errors.New("home requires local X11, no fallback")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return homeClose, errors.New("home requires a local Linux X11 display")
	}
	defer c.Close()
	screen := xproto.Setup(c).DefaultScreen(c)
	if !nativeControlsRGB(screen) || screen.WidthInPixels < 940 || screen.HeightInPixels < 600 {
		return homeClose, errors.New("home requires supported24-bit X11 and940x600pixels")
	}
	if err = nativeControlsWatchRoot(c, screen); err != nil {
		return homeClose, err
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return homeClose, err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 900, 560, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x101824, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return homeClose, err
	}
	// Core fonts keep this engineering entry independent of another UI runtime.
	fonts := make(map[string]xproto.Font)
	for _, name := range []string{"6x13", "9x15", "10x20"} {
		font, er := xproto.NewFontId(c)
		if er != nil {
			return homeClose, er
		}
		if er = xproto.OpenFontChecked(c, font, uint16(len(name)), name).Check(); er != nil {
			return homeClose, errors.New("home readable fonts unavailable")
		}
		fonts[name] = font
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return homeClose, err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), xproto.GcFont, []uint32{uint32(fonts["6x13"])}).Check(); err != nil {
		return homeClose, err
	}
	wmDelete, err := xproto.InternAtom(c, false, 16, "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return homeClose, err
	}
	protocols, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return homeClose, err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(wmDelete.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocols.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return homeClose, err
	}
	title := "Mnet: three tools [local engineering]"
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check(); err != nil {
		return homeClose, err
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		return homeClose, err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	events := make(chan deviceEvent, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			ev, er := c.WaitForEvent()
			select {
			case events <- deviceEvent{ev, er}:
			case <-ctx.Done():
				return
			}
			if ev == nil || er != nil {
				return
			}
		}
	}()
	defer func() { cancel(); c.Close(); <-done }()
	enabled := [4]bool{true, true, hasLayout, true}
	controls := newDeviceControls()
	fill := func(x, y, width, height int16, color uint32) error {
		if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground, []uint32{color}).Check(); er != nil {
			return er
		}
		return xproto.PolyFillRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{{X: x, Y: y, Width: uint16(width), Height: uint16(height)}}).Check()
	}
	text := func(x, y int16, color, background uint32, font, line string) error {
		if len(line) > 140 {
			return errors.New("home text exceeds fixed layout")
		}
		if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{color, background, uint32(fonts[font])}).Check(); er != nil {
			return er
		}
		return xproto.ImageText8Checked(c, byte(len(line)), xproto.Drawable(w), gc, x, y, line).Check()
	}
	draw := func() error {
		if er := fill(0, 0, 900, 560, 0x101824); er != nil {
			return er
		}
		if er := text(24, 36, 0xffffff, 0x101824, "10x20", "Your devices. Your control."); er != nil {
			return er
		}
		if er := text(690, 32, 0xe1b772, 0x101824, "6x13", "Engineering build"); er != nil {
			return er
		}
		if er := text(24, 66, 0x9bb4cd, 0x101824, "9x15", "Three tools. Separate permissions."); er != nil {
			return er
		}
		if er := text(24, 94, 0xe1b772, 0x101824, "6x13", "Multies setup isn't connected in this build. These are local engineering tools."); er != nil {
			return er
		}
		cards := [][3]string{
			{"Mnet networking", "See network devices", "Read-only snapshot. No enrollment or connection changes."},
			{"Remote Desktop", "Choose a device to view", "Host approval required. Pointer control is separate."},
			{"Shared Desk", "View your display layout", "Layout only. Keyboard and mouse pairing isn't connected."},
		}
		for i, card := range cards {
			y := int16(110 + i*112)
			bg := uint32(0x1c2a3b)
			heading := uint32(0x70c7ef)
			if !enabled[i] {
				bg = 0x182331
				heading = 0x91a1b3
			}
			if er := fill(20, y, 860, 104, bg); er != nil {
				return er
			}
			if er := text(38, y+28, heading, bg, "10x20", card[0]); er != nil {
				return er
			}
			if er := text(38, y+55, 0xffffff, bg, "9x15", card[1]); er != nil {
				return er
			}
			if er := text(38, y+78, 0xb6c9db, bg, "6x13", card[2]); er != nil {
				return er
			}
			if i == 2 && !hasLayout {
				if er := text(38, y+96, 0xe1b772, bg, "6x13", "Add --layout FILE when opening to use the offline inspector."); er != nil {
					return er
				}
			}
			if enabled[i] {
				if er := text(832, y+52, 0x70c7ef, bg, "10x20", ">"); er != nil {
					return er
				}
			}
			if controls.focus == i && enabled[i] {
				if er := fill(22, y+2, 4, 100, 0xffffff); er != nil {
					return er
				}
			}
		}
		if er := text(24, 468, 0xb6c9db, 0x101824, "6x13", notice); er != nil {
			return er
		}
		if er := fill(680, 486, 200, 44, 0x304761); er != nil {
			return er
		}
		label := "Close"
		if controls.focus == 3 {
			label = "> Close"
		}
		if er := text(710, 514, 0xffffff, 0x304761, "9x15", label); er != nil {
			return er
		}
		return text(24, 549, 0x9bb4cd, 0x101824, "6x13", "Tab to choose. Enter to open. Escape to close. Closing a tool brings you back here.")
	}
	hit := func(button byte, x, y int) int {
		if button != 1 {
			return -1
		}
		for i := 0; i < 3; i++ {
			top := 110 + i*112
			if x >= 20 && x < 880 && y >= top && y < top+104 {
				return i
			}
		}
		if x >= 680 && x < 880 && y >= 486 && y < 530 {
			return 3
		}
		return -1
	}
	route := func(index int) (homeChoice, bool) {
		switch index {
		case 0:
			return homeNetwork, true
		case 1:
			return homeDesktop, true
		case 2:
			if hasLayout {
				return homeLayout, true
			}
		case 3:
			return homeClose, true
		}
		return homeClose, false
	}
	if err = draw(); err != nil {
		return homeClose, err
	}
	for {
		select {
		case <-ctx.Done():
			return homeClose, nil
		case event := <-events:
			if event.err != nil || event.event == nil {
				return homeClose, errors.New("home display ended")
			}
			selected := -1
			switch e := event.event.(type) {
			case xproto.ExposeEvent:
			case xproto.FocusOutEvent:
				controls.cancel()
				controls.focus = -1
			case xproto.DestroyNotifyEvent:
				return homeClose, nil
			case xproto.ClientMessageEvent:
				if e.Type == protocols.Atom && e.Format == 32 && xproto.Atom(e.Data.Data32[0]) == wmDelete.Atom {
					return homeClose, nil
				}
			case xproto.ConfigureNotifyEvent:
				if nativeControlsRootChanged(screen, e) || e.Window == w && (e.Width != 900 || e.Height != 560) {
					return homeClose, errors.New("home geometry changed")
				}
			case xproto.ButtonPressEvent:
				controls.cancel()
				index := hit(byte(e.Detail), int(e.EventX), int(e.EventY))
				if index >= 0 && enabled[index] {
					controls.pressed = index
					controls.mouse = true
					controls.focus = index
				}
			case xproto.ButtonReleaseEvent:
				index := hit(byte(e.Detail), int(e.EventX), int(e.EventY))
				if controls.mouse && index >= 0 && index == controls.pressed && enabled[index] {
					selected = index
				}
				controls.cancel()
			case xproto.KeyPressEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil {
					return homeClose, errors.New("home keyboard unavailable")
				}
				if len(mapping.Keysyms) == 0 {
					controls.cancel()
					break
				}
				key := uint32(mapping.Keysyms[0])
				switch key {
				case 0xff1b:
					return homeClose, nil
				case 0xff09:
					controls.advance(e.State&xproto.ModMaskShift != 0, enabled)
				default:
					controls.pressKey(key, enabled)
				}
			case xproto.KeyReleaseEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil {
					return homeClose, errors.New("home keyboard unavailable")
				}
				if len(mapping.Keysyms) > 0 {
					selected = controls.releaseKey(uint32(mapping.Keysyms[0]), enabled)
				} else {
					controls.cancel()
				}
			}
			if choice, ok := route(selected); ok {
				return choice, nil
			}
			if err = draw(); err != nil {
				return homeClose, err
			}
		}
	}
}
