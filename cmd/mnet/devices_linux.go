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
	status *daemon.Status
	err    error
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
	screen := xproto.Setup(c).DefaultScreen(c)
	if screen.WidthInPixels < 940 || screen.HeightInPixels < 540 {
		return errors.New("device window requires at least940x540 local pixels")
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 900, 500, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0xf5f5f7, xproto.EventMaskExposure | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskStructureNotify}).Check(); err != nil {
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
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{0x1d1d1f, 0xf5f5f7, uint32(font)}).Check(); err != nil {
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
		go func() {
			defer close(done)
			defer stop()
			snapshot, fetchErr := readDeviceSnapshot(requestCtx, hc, base)
			select {
			case results <- deviceResult{snapshot, fetchErr}:
			case <-ctx.Done():
			}
		}()
	}
	draw := func() error {
		if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground, []uint32{0xf5f5f7}).Check(); er != nil {
			return er
		}
		if er := xproto.PolyFillRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{{Width: 900, Height: 500}}).Check(); er != nil {
			return er
		}
		if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground|xproto.GcBackground, []uint32{0x1d1d1f, 0xf5f5f7}).Check(); er != nil {
			return er
		}
		lines, p := deviceSnapshotLines(st, page, sampled)
		page = p
		for n, line := range lines {
			if er := xproto.ImageText8Checked(c, byte(len(line)), xproto.Drawable(w), gc, 20, int16(28+n*24), line).Check(); er != nil {
				return er
			}
		}
		for n, line := range []string{message, "Refresh or R reads a new snapshot. Rows expire after30s. There is no automatic polling.", "No server URL, approval link, key, remote endpoint or raw daemon error is displayed."} {
			if er := xproto.ImageText8Checked(c, byte(len(line)), xproto.Drawable(w), gc, 20, int16(402+n*18), line).Check(); er != nil {
				return er
			}
		}
		for index, button := range []struct {
			x     int16
			label string
		}{{20, "REFRESH (R)"}, {240, "PREVIOUS"}, {460, "NEXT"}, {680, "CLOSE (Escape)"}} {
			enabled := index == 3 || index == 0 && !loading || index == 1 && st != nil && page > 0 || index == 2 && st != nil && st.Status == "up" && (page+1)*devicePageSize < len(st.Peers)
			background, foreground := uint32(0x0066cc), uint32(0xffffff)
			if !enabled {
				background, foreground = 0xe8e8ed, 0x65656b
			}
			if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground, []uint32{background}).Check(); er != nil {
				return er
			}
			if er := xproto.PolyFillRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{{X: button.x, Y: 448, Width: 200, Height: 44}}).Check(); er != nil {
				return er
			}
			if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground|xproto.GcBackground, []uint32{foreground, background}).Check(); er != nil {
				return er
			}
			if er := xproto.ImageText8Checked(c, byte(len(button.label)), xproto.Drawable(w), gc, button.x+16, 476, button.label).Check(); er != nil {
				return er
			}
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
			loading = false
			if result.err != nil {
				st = nil
				phase = "unavailable"
				message = result.err.Error()
			} else {
				st = result.status
				sampled = time.Now()
				phase = st.Status
				switch phase {
				case "up", "down", "joining", "preparing", "waiting-approval", "key-expired":
				default:
					phase = "unknown"
				}
				message = "Reported state at the snapshot time only. Refresh before relying on it."
			}
		case <-ticker.C:
			if st == nil || time.Since(sampled) < 30*time.Second {
				changed = false
			} else {
				st = nil
				page = 0
				phase = "expired"
				message = "Snapshot expired. Old peer and readiness rows removed; refresh to check again."
			}
		case event := <-events:
			if event.err != nil || event.event == nil {
				return errors.New("device window display ended unexpectedly")
			}
			switch e := event.event.(type) {
			case xproto.ExposeEvent:
			case xproto.DestroyNotifyEvent:
				return nil
			case xproto.ClientMessageEvent:
				if e.Type == protocols.Atom && e.Format == 32 && xproto.Atom(e.Data.Data32[0]) == atom.Atom {
					return nil
				}
				changed = false
			case xproto.ConfigureNotifyEvent:
				if e.Window == w && (e.Width != 900 || e.Height != 500) {
					return errors.New("device window resized outside supported layout")
				}
				changed = false
			case xproto.ButtonReleaseEvent:
				if e.Detail != 1 || e.EventY < 448 || e.EventY >= 492 {
					changed = false
					break
				}
				switch {
				case e.EventX >= 20 && e.EventX < 220:
					refresh()
				case e.EventX >= 240 && e.EventX < 440:
					page--
				case e.EventX >= 460 && e.EventX < 660:
					page++
				case e.EventX >= 680 && e.EventX < 880:
					return nil
				default:
					changed = false
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
				case 'r', 'R':
					refresh()
				case 0xff51:
					page--
				case 0xff53:
					page++
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
