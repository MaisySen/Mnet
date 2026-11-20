//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"fintex.au/mnet/internal/daemon"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type deviceResult struct {
	snapshot timedDeviceSnapshot
	err      error
}
type deviceEvent struct {
	event xgb.Event
	err   error
}

func runDeviceWindow(parent context.Context, hc *http.Client, base string) error {
	if parent == nil || parent.Err() != nil {
		return errors.New("local device window owner absent")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return errors.New("device window requires a local Linux X11 display")
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
		return errors.New("device window pixel format unsupported")
	}
	screen := setup.DefaultScreen(c)
	if !nativeControlsRGB(screen) {
		return errors.New("native controls require supported 24-bit TrueColor X11")
	}
	if screen.WidthInPixels < 940 || screen.HeightInPixels < 540 {
		return errors.New("device window requires at least940x540 local pixels")
	}
	if err = nativeControlsWatchRoot(c, screen); err != nil {
		return err
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 900, 500, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0xf5f5f7, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
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
	protocols, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(atom.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocols.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return err
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	events := make(chan deviceEvent, 1)
	eventDone := make(chan struct{})
	go func() {
		defer close(eventDone)
		for {
			ev, er := c.WaitForEvent()
			select {
			case events <- deviceEvent{ev, er}:
			case <-ctx.Done():
				return
			}
			if er != nil || ev == nil {
				return
			}
		}
	}()
	results := make(chan deviceResult, 1)
	var requestCancel context.CancelFunc
	var requestDone chan struct{}
	defer func() {
		cancel()
		if requestCancel != nil {
			requestCancel()
		}
		c.Close()
		<-eventDone
		if requestDone != nil {
			<-requestDone
		}
	}()
	var st *daemon.Status
	var sampled time.Time
	phase := "loading"
	message := "Reading the local daemon. Previous rows cleared."
	page := 0
	generation := 0
	loading := false
	controls := newDeviceControls()
	enabledControls := func() [4]bool {
		state, peers := "", 0
		if st != nil {
			state, peers = st.Status, len(st.Peers)
		}
		return deviceControlEnabled(loading, state, page, peers)
	}
	activate := func(index int) bool {
		controls.cancel()
		if !enabledControls()[index] {
			return false
		}
		switch index {
		case 1:
			page--
		case 2:
			page++
		case 3:
			return true
		}
		return false
	}
	refresh := func() {
		if loading {
			return
		}
		if requestDone != nil {
			<-requestDone
		}
		requestCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		requestCancel = stop
		requestDone = make(chan struct{})
		done := requestDone
		st = nil
		sampled = time.Time{}
		page = 0
		phase = "loading"
		message = "Reading the local daemon. Previous rows cleared."
		loading = true
		generation++
		controls.update(enabledControls(), uint64(generation))
		go func() {
			defer close(done)
			defer stop()
			snapshot, fetchErr := readTimedDeviceSnapshot(requestCtx, hc, base)
			select {
			case results <- deviceResult{snapshot, fetchErr}:
			case <-ctx.Done():
			}
		}()
	}
	draw := func() error {

		controls.update(enabledControls(), uint64(generation))
		picture, p, er := renderDevicesSurface(st, page, sampled, message, enabledControls(), controls.focus)
		if er != nil {
			return er
		}
		page = p
		pix, er := xproto.NewPixmapId(c)
		if er != nil {
			return er
		}
		if er = xproto.CreatePixmapChecked(c, 24, pix, xproto.Drawable(w), 900, 500).Check(); er != nil {
			return er
		}
		defer xproto.FreePixmap(c, pix)
		sampler := newViewerPixelSource(picture, picture.Bounds(), 900, 500)
		for y := 0; y < 500; y += 16 {
			n := min(16, 500-y)
			if er = xproto.PutImageChecked(c, xproto.ImageFormatZPixmap, xproto.Drawable(pix), gc, 900, uint16(n), 0, int16(y), 0, 24, sampler.strip(y, n)).Check(); er != nil {
				return er
			}
		}
		if er = xproto.ChangeWindowAttributesChecked(c, w, xproto.CwBackPixmap, []uint32{uint32(pix)}).Check(); er != nil {
			return er
		}
		if er = xproto.CopyAreaChecked(c, xproto.Drawable(pix), xproto.Drawable(w), gc, 0, 0, 0, 0, 900, 500).Check(); er != nil {
			return er
		}
		title := fmt.Sprintf("Mnet devices: read-only [%s] read%d page%d", phase, generation, page+1)
		return xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
	}
	refresh()
	if err = draw(); err != nil {
		return errors.New("device window drawing failed")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		changed := true
		select {
		case <-ctx.Done():
			return nil
		case result := <-results:
			controls.cancel()
			loading = false
			if result.err != nil {
				st = nil
				phase = "unavailable"
				message = result.err.Error()
			} else {
				st = result.snapshot.status
				sampled = result.snapshot.started
				phase = st.Status
				switch phase {
				case "up", "down", "joining", "preparing", "waiting-approval", "key-expired":
				default:
					phase = "unknown"
				}
				message = "Reported state at the snapshot time only. Refresh before relying on it."
			}
		case <-ticker.C:
			if st == nil || (timedDeviceSnapshot{st, sampled}).current(time.Now()) {
				changed = false
			} else {
				st = nil
				page = 0
				controls.cancel()
				phase = "expired"
				message = "Snapshot expired. Old peer and readiness rows removed; refresh to check again."
			}
		case event := <-events:
			if event.err != nil || event.event == nil {
				return errors.New("device window display ended unexpectedly")
			}
			switch e := event.event.(type) {
			case xproto.ExposeEvent:
			case xproto.FocusOutEvent:
				controls.cancel()
				controls.focus = -1
			case xproto.DestroyNotifyEvent:
				return nil
			case xproto.ClientMessageEvent:
				if e.Type == protocols.Atom && e.Format == 32 && xproto.Atom(e.Data.Data32[0]) == atom.Atom {
					return nil
				}
				changed = false
			case xproto.ConfigureNotifyEvent:
				if nativeControlsRootChanged(screen, e) {
					return errors.New("native controls root geometry changed")
				}
				if e.Window == w && (e.Width != 900 || e.Height != 500) {
					return errors.New("device window resized outside supported layout")
				}
				changed = false
			case xproto.ButtonPressEvent:
				controls.pressMouse(byte(e.Detail), int(e.EventX), int(e.EventY), enabledControls())
			case xproto.ButtonReleaseEvent:
				index := controls.releaseMouse(byte(e.Detail), int(e.EventX), int(e.EventY), enabledControls())
				if index == 0 {
					refresh()
				} else if index > 0 && activate(index) {
					return nil
				}

			case xproto.KeyReleaseEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil {
					return errors.New("device window keyboard unavailable")
				}
				if len(mapping.Keysyms) > 0 {
					index := controls.releaseKey(uint32(mapping.Keysyms[0]), enabledControls())
					if index == 0 {
						refresh()
					} else if index > 0 && activate(index) {
						return nil
					}
				}
			case xproto.KeyPressEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil {
					return errors.New("device window keyboard unavailable")
				}
				if len(mapping.Keysyms) == 0 {
					changed = false
					break
				}
				switch mapping.Keysyms[0] {
				case 0xff1b:
					return nil
				case 0xff09:
					controls.advance(e.State&xproto.ModMaskShift != 0, enabledControls())
				case 0xff0d, 0x20:
					controls.pressKey(uint32(mapping.Keysyms[0]), enabledControls())
				case 'r', 'R':
					refresh()
				case 0xff51, 0xff53, 0xff55, 0xff56, 0xff50, 0xff57:
					controls.cancel()
					state, peers := "", 0
					if st != nil {
						state, peers = st.Status, len(st.Peers)
					}
					page, _ = devicePageForKey(uint32(mapping.Keysyms[0]), state, page, peers)
				default:
					changed = false
				}
			default:
				changed = false
			}
		}
		if changed {
			if err = draw(); err != nil {
				return errors.New("device window drawing failed")
			}
		}
	}
}
