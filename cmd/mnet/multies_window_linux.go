//go:build linux

package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"time"

	"fintex.au/mnet/internal/multies"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type multiesWindowResult struct {
	target, key string
	list        multies.ApprovedDeviceList
	err         error
}

// This explicit opt-in window has no default remote origin. Closing joins the
// native sign-in worker and closes its loopback listener before returning.
func runMultiesWindow(parent context.Context, local *http.Client, base, origin string) error {
	if parent == nil || parent.Err() != nil || len(origin) > 75 {
		return errors.New("Multies window parameters unavailable")
	}
	for _, r := range origin {
		if r < 33 || r > 126 {
			return errors.New("bridge origin must be readable ASCII")
		}
	}
	if _, err := multies.NewBridgeClient(origin); err != nil {
		return multies.ErrDenied
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return errors.New("experimental sign in window requires Linux X11")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return errors.New("sign in window needs local X11")
	}
	defer c.Close()
	screen := xproto.Setup(c).DefaultScreen(c)
	if !nativeControlsRGB(screen) || screen.WidthInPixels < 940 || screen.HeightInPixels < 540 {
		return errors.New("sign in window requires24bit940x540X11")
	}
	if err = nativeControlsWatchRoot(c, screen); err != nil {
		return err
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 900, 500, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x101824, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return err
	}
	fonts := map[string]xproto.Font{}
	for _, name := range []string{"6x13", "9x15", "10x20"} {
		f, e := xproto.NewFontId(c)
		if e != nil {
			return e
		}
		if e = xproto.OpenFontChecked(c, f, uint16(len(name)), name).Check(); e != nil {
			return errors.New("readable sign in fonts unavailable")
		}
		fonts[name] = f
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), 0, nil).Check(); err != nil {
		return err
	}
	wmDelete, err := xproto.InternAtom(c, false, 16, "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return err
	}
	protocols, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(wmDelete.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocols.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return err
	}
	title := "Mnet: experimental Multies device approvals"
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check(); err != nil {
		return err
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	events := make(chan deviceEvent, 1)
	eventDone := make(chan struct{})
	go func() {
		defer close(eventDone)
		for {
			e, er := c.WaitForEvent()
			select {
			case events <- deviceEvent{e, er}:
			case <-ctx.Done():
				return
			}
			if e == nil || er != nil {
				return
			}
		}
	}()
	var workerCancel context.CancelFunc
	var workerDone chan struct{}
	defer func() {
		cancel()
		if workerCancel != nil {
			workerCancel()
		}
		c.Close()
		<-eventDone
		if workerDone != nil {
			<-workerDone
		}
	}()
	results := make(chan multiesWindowResult, 1)
	permit := make(chan struct{}, 1)
	phase := "start"
	key := ""
	var list multies.ApprovedDeviceList
	var sampled time.Time
	page := 0
	generation := uint64(0)
	controls := newDeviceControls()
	enabled := func() [4]bool {
		return [4]bool{phase == "start" || phase == "review" || phase == "done" || phase == "failed" || phase == "stale", phase == "done" && page > 0, phase == "done" && (page+1)*4 < len(list.Devices), true}
	}
	begin := func() {
		if workerDone != nil {
			<-workerDone
		}
		for len(permit) > 0 {
			<-permit
		}
		work, stop := context.WithCancel(ctx)
		workerCancel = stop
		workerDone = make(chan struct{})
		done := workerDone
		phase = "preparing"
		key = ""
		list = multies.ApprovedDeviceList{}
		sampled = time.Time{}
		page = 0
		generation++
		controls.update(enabled(), generation)
		go func() {
			defer close(done)
			defer stop()
			devices, e := runMultiesSignIn(work, local, base, origin, func(_ context.Context, target string) error {
				u, er := url.Parse(target)
				if er != nil {
					return multies.ErrDenied
				}
				k := u.Query().Get("deviceKey")
				if len(k) != 43 {
					return multies.ErrDenied
				}
				select {
				case results <- multiesWindowResult{target: target, key: k}:
				case <-work.Done():
					return multies.ErrDenied
				}
				select {
				case <-permit:
				case <-work.Done():
					return multies.ErrDenied
				}
				if work.Err() != nil {
					return multies.ErrDenied
				}
				return openMultiesBrowser(work, target)
			})
			select {
			case results <- multiesWindowResult{list: devices, err: e}:
			case <-work.Done():
			}
		}()
	}
	fill := func(x, y, width, height int16, color uint32) error {
		if e := xproto.ChangeGCChecked(c, gc, xproto.GcForeground, []uint32{color}).Check(); e != nil {
			return e
		}
		return xproto.PolyFillRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{{X: x, Y: y, Width: uint16(width), Height: uint16(height)}}).Check()
	}
	text := func(x, y int16, color uint32, font, line string, backgrounds ...uint32) error {
		background := uint32(0x101824)
		if len(backgrounds) > 0 {
			background = backgrounds[0]
		}
		if len(line) > 130 {
			return errors.New("sign in text exceeds bounded layout")
		}
		if e := xproto.ChangeGCChecked(c, gc, xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{color, background, uint32(fonts[font])}).Check(); e != nil {
			return e
		}
		return xproto.ImageText8Checked(c, byte(len(line)), xproto.Drawable(w), gc, x, y, line).Check()
	}
	draw := func() error {
		if e := fill(0, 0, 900, 500, 0x101824); e != nil {
			return e
		}
		if e := text(22, 32, 0xffffff, "10x20", "Multies device approvals"); e != nil {
			return e
		}
		lines := []string{"Experimental local build. Own device approvals only. Not VPN connectivity.", "Trusted bridge: " + origin, "No mail, other people's devices, screen viewing, keyboard, mouse or file access."}
		switch phase {
		case "start":
			lines = append(lines, "", "Start prepares a private local device key, then asks you before opening the browser.", "It does not start a network, enroll a VPN peer or save a Multies credential.")
		case "preparing":
			lines = append(lines, "", "Preparing local identity. No browser opened yet.")
		case "review":
			lines = append(lines, "", "Compare this exact device key with the key on the Multies approval page:", key, "", "Open browser only if this is the bridge you intended to use.", "The browser will ask you to choose an existing workspace and approve list-only access.")
		case "waiting":
			lines = append(lines, "", "Waiting for your browser decision. Closing this window cancels the native attempt.", "Compare the key before approving:", key)
		case "failed":
			lines = append(lines, "", "Sign in did not finish, or current access was refused. No credential was saved.", "Start again for a fresh request. No previous device rows are kept.")
		case "stale":
			lines = append(lines, "", "The device snapshot expired after30seconds. Previous rows were cleared.", "Start again to check current approvals. No background polling.")
		case "done":
			lines = append(lines, "", "Current own-device snapshot. These records do not prove a device is online.")
			if len(list.Devices) == 0 {
				lines = append(lines, "No current own-device approvals.")
			}
			start := page * 4
			end := start + 4
			if end > len(list.Devices) {
				end = len(list.Devices)
			}
			for _, d := range list.Devices[start:end] {
				lines = append(lines, deviceText(d.Platform, 8)+"  "+d.DeviceKey)
			}
			if list.Truncated {
				lines = append(lines, "Server list is truncated. Other records may exist.")
			}
		}
		for i, line := range lines {
			font := "9x15"
			if i == 1 || len(line) > 92 {
				font = "6x13"
			}
			if e := text(22, int16(66+i*24), 0xb6c9db, font, line); e != nil {
				return e
			}
		}
		labels := [4]string{"Start sign in", "Previous", "Next", "Close"}
		if phase == "review" {
			labels[0] = "Open browser"
		}
		if phase == "done" || phase == "stale" || phase == "failed" {
			labels[0] = "Start again"
		}
		en := enabled()
		for i, x := range []int16{20, 240, 460, 680} {
			bg := uint32(0x304761)
			fg := uint32(0xffffff)
			if !en[i] {
				fg = 0x91a1b3
				bg = 0x1c2a3b
			}
			if e := fill(x, 448, 200, 44, bg); e != nil {
				return e
			}
			label := labels[i]
			if controls.focus == i && en[i] {
				label = "> " + label
			}
			if e := text(x+12, 475, fg, "9x15", label, bg); e != nil {
				return e
			}
		}
		return nil
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	if err = draw(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if phase == "done" && time.Since(sampled) >= 30*time.Second {
				phase = "stale"
				list = multies.ApprovedDeviceList{}
				page = 0
				generation++
				controls.update(enabled(), generation)
				if err = draw(); err != nil {
					return err
				}
			}
		case result := <-results:
			if result.target != "" {
				phase = "review"
				key = result.key
			} else if result.err != nil {
				phase = "failed"
				key = ""
				list = multies.ApprovedDeviceList{}
			} else {
				phase = "done"
				list = result.list
				sampled = result.list.ReadStartedAt
			}
			generation++
			controls.update(enabled(), generation)
			if err = draw(); err != nil {
				return err
			}
		case event := <-events:
			if event.err != nil || event.event == nil {
				return errors.New("sign in display ended")
			}
			selected := -1
			en := enabled()
			switch e := event.event.(type) {
			case xproto.ExposeEvent:
			case xproto.FocusOutEvent:
				controls.cancel()
				controls.focus = -1
			case xproto.DestroyNotifyEvent:
				return nil
			case xproto.ClientMessageEvent:
				if e.Type == protocols.Atom && e.Format == 32 && xproto.Atom(e.Data.Data32[0]) == wmDelete.Atom {
					return nil
				}
			case xproto.ConfigureNotifyEvent:
				if nativeControlsRootChanged(screen, e) || e.Window == w && (e.Width != 900 || e.Height != 500) {
					return errors.New("sign in geometry changed")
				}
			case xproto.ButtonPressEvent:
				controls.pressMouse(byte(e.Detail), int(e.EventX), int(e.EventY), en)
				if controls.pressed >= 0 {
					controls.focus = controls.pressed
				}
			case xproto.ButtonReleaseEvent:
				selected = controls.releaseMouse(byte(e.Detail), int(e.EventX), int(e.EventY), en)
			case xproto.KeyPressEvent:
				m, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil {
					return errors.New("sign in keyboard unavailable")
				}
				if len(m.Keysyms) == 0 {
					controls.cancel()
					break
				}
				k := uint32(m.Keysyms[0])
				if k == 0xff1b {
					return nil
				}
				if k == 0xff09 {
					controls.advance(e.State&xproto.ModMaskShift != 0, en)
				} else {
					controls.pressKey(k, en)
				}
			case xproto.KeyReleaseEvent:
				m, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil {
					return errors.New("sign in keyboard unavailable")
				}
				if len(m.Keysyms) > 0 {
					selected = controls.releaseKey(uint32(m.Keysyms[0]), en)
				} else {
					controls.cancel()
				}
			}
			if selected >= 0 && en[selected] {
				switch selected {
				case 0:
					if phase == "review" {
						phase = "waiting"
						permit <- struct{}{}
					} else {
						begin()
					}
				case 1:
					page--
				case 2:
					page++
				case 3:
					return nil
				}
				generation++
				controls.update(enabled(), generation)
			}
			if err = draw(); err != nil {
				return err
			}
		}
	}
}
