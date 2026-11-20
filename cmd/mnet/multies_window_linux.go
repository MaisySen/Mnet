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
	setup := xproto.Setup(c)
	format := false
	for _, f := range setup.PixmapFormats {
		if f.Depth == 24 && f.BitsPerPixel == 32 && f.ScanlinePad == 32 {
			format = true
		}
	}
	if !format || setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return errors.New("unsupported sign in pixel format")
	}
	screen := setup.DefaultScreen(c)
	if !nativeControlsRGB(screen) || screen.WidthInPixels < 940 || screen.HeightInPixels < 540 {
		return errors.New("sign in window requires supported 24-bit X11 and 940x540 pixels")
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
	draw := func() error {
		picture, e := renderMultiesSurface(multiesSurface{phase: phase, key: key, origin: origin, list: list, page: page, focus: controls.focus, enabled: enabled()})
		if e != nil {
			return e
		}
		pix, e := xproto.NewPixmapId(c)
		if e != nil {
			return e
		}
		if e = xproto.CreatePixmapChecked(c, 24, pix, xproto.Drawable(w), 900, 500).Check(); e != nil {
			return e
		}
		defer xproto.FreePixmap(c, pix)
		// Upload bounded complete strips into an off-screen pixmap. A failed
		// strip cannot expose a half-drawn consent or stale device state.
		sampler := newViewerPixelSource(picture, picture.Bounds(), 900, 500)
		for y := 0; y < 500; y += 16 {
			n := min(16, 500-y)
			if e = xproto.PutImageChecked(c, xproto.ImageFormatZPixmap, xproto.Drawable(pix), gc, 900, uint16(n), 0, int16(y), 0, 24, sampler.strip(y, n)).Check(); e != nil {
				return e
			}
		}
		return xproto.CopyAreaChecked(c, xproto.Drawable(pix), xproto.Drawable(w), gc, 0, 0, 0, 0, 900, 500).Check()
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
