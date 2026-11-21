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
func chooseHome(parent context.Context, notice string, hasLayout, hasMulties bool) (homeChoice, error) {
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
	setup := xproto.Setup(c)
	format := false
	for _, f := range setup.PixmapFormats {
		if f.Depth == 24 && f.BitsPerPixel == 32 && f.ScanlinePad == 32 {
			format = true
		}
	}
	if !format || setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return homeClose, errors.New("home pixel format unsupported")
	}
	screen := setup.DefaultScreen(c)
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
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return homeClose, err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), 0, nil).Check(); err != nil {
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
	title := "Mnet: three tools [local engineering] info0"
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check(); err != nil {
		return homeClose, err
	}
	// Title updates from the event loop are unchecked: a checked round trip
	// here stalls behind the event read and shows the previous state.
	titleInfo := false
	setTitle := func(info bool) {
		if info == titleInfo {
			return
		}
		name := "Mnet: three tools [local engineering] info0"
		if info {
			name = "Mnet: three tools [local engineering] info1"
		}
		_ = xproto.ChangeProperty(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(name)), []byte(name))
		titleInfo = info
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
	enabledAll := [6]bool{true, true, hasLayout, true, hasMulties, true}
	enabledInfo := [6]bool{false, false, false, false, false, true}
	info := false
	enabled := func() []bool {
		if info {
			return enabledInfo[:]
		}
		return enabledAll[:]
	}
	controls := newDeviceControls()

	draw := func() error {
		picture, er := renderHomeSurface(notice, hasLayout, hasMulties, controls.focus, info)
		if er != nil {
			return er
		}
		pix, er := xproto.NewPixmapId(c)
		if er != nil {
			return er
		}
		if er = xproto.CreatePixmapChecked(c, 24, pix, xproto.Drawable(w), 900, 560).Check(); er != nil {
			return er
		}
		defer xproto.FreePixmap(c, pix)
		sampler := newViewerPixelSource(picture, picture.Bounds(), 900, 560)
		for y := 0; y < 560; y += 16 {
			n := min(16, 560-y)
			if er = xproto.PutImageChecked(c, xproto.ImageFormatZPixmap, xproto.Drawable(pix), gc, 900, uint16(n), 0, int16(y), 0, 24, sampler.strip(y, n)).Check(); er != nil {
				return er
			}
		}
		if er = xproto.ChangeWindowAttributesChecked(c, w, xproto.CwBackPixmap, []uint32{uint32(pix)}).Check(); er != nil {
			return er
		}
		if er = xproto.CopyAreaChecked(c, xproto.Drawable(pix), xproto.Drawable(w), gc, 0, 0, 0, 0, 900, 560).Check(); er != nil {
			return er
		}
		// The title flips only after the new pixels are on screen, so a
		// screenshot keyed on the title never captures stale content.
		setTitle(info)
		return nil
	}
	hit := func(button byte, x, y int) int {
		if button != 1 {
			return -1
		}
		if x >= 520 && x < 720 && y >= 18 && y < 58 {
			return 5
		}
		if info {
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
		if x >= 20 && x < 660 && y >= 486 && y < 530 {
			return 4
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
		case 4:
			if hasMulties {
				return homeMulties, true
			}
		}
		return homeClose, false
	}
	if err = draw(); err != nil {
		return homeClose, err
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
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
				if index >= 0 && enabled()[index] {
					controls.pressed = index
					controls.mouse = true
					controls.focus = index
				}
			case xproto.ButtonReleaseEvent:
				index := hit(byte(e.Detail), int(e.EventX), int(e.EventY))
				if controls.mouse && index >= 0 && index == controls.pressed && enabled()[index] {
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
					if info {
						info = false
						break
					}
					return homeClose, nil
				case 0xff09:
					controls.advanceAmong(e.State&xproto.ModMaskShift != 0, enabled())
				default:
					controls.pressKeyAmong(key, enabled())
				}
			case xproto.KeyReleaseEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil {
					return homeClose, errors.New("home keyboard unavailable")
				}
				if len(mapping.Keysyms) > 0 {
					selected = controls.releaseKeyAmong(uint32(mapping.Keysyms[0]), enabled())
				} else {
					controls.cancel()
				}
			}
			if selected == 5 {
				info = !info
				controls.focus = 5
				selected = -1
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
