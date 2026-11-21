//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// A local-only drafts window. No daemon, transport, pairing, capture or input
// owner is reachable here. It lists offline arrangement files and opens the
// existing offline inspector or editor as separate owned windows.
func showDeskArrangements(ctx context.Context, v *deskArrangementsView) (int, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return deskArrActClose, errors.New("arrangements require local X11, no fallback")
	}
	if ctx == nil || ctx.Err() != nil || v == nil {
		return deskArrActClose, errors.New("arrangements window unavailable")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return deskArrActClose, errors.New("arrangements require local Linux X11")
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
		return deskArrActClose, errors.New("native controls require supported 24-bit TrueColor X11")
	}
	if !format || setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return deskArrActClose, errors.New("arrangements pixel format unsupported")
	}
	if screen.WidthInPixels < 940 || screen.HeightInPixels < 600 {
		return deskArrActClose, errors.New("arrangements require at least 940x600 pixels")
	}
	if err = nativeControlsWatchRoot(c, screen); err != nil {
		return deskArrActClose, err
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return deskArrActClose, err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, deskArrWinW, deskArrWinH, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x17202e, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return deskArrActClose, err
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return deskArrActClose, err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), 0, nil).Check(); err != nil {
		return deskArrActClose, err
	}
	atom, err := xproto.InternAtom(c, false, 16, "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return deskArrActClose, err
	}
	protocol, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return deskArrActClose, err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(atom.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocol.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return deskArrActClose, err
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

	v.clamp()
	pressed, held := -1, uint32(0)
	clear := func() { pressed = -1; held = 0 }
	rowAt := func(x, y int16) int {
		if int(y) < 108 || int(x) < 28 || int(x) >= 872 {
			return -1
		}
		i := (int(y) - 108) / 36
		if i >= 0 && i < v.rows() && i < deskArrRows {
			return i
		}
		return -1
	}
	buttonAt := func(x, y int16) int {
		if int(y) < 452 || int(y) >= 496 {
			return -1
		}
		for i := 0; i < 5; i++ {
			bx := 20 + i*150
			if int(x) >= bx && int(x) < bx+140 {
				return i // button order: New starter, Inspect, Edit, Delete, Close
			}
		}
		return -1
	}
	buttonAction := []int{deskArrActStarter, deskArrActInspect, deskArrActEdit, deskArrActDelete, deskArrActClose}
	state := func() string {
		return fmt.Sprintf("rows%d sel%d pinned%t focus%d armed%t", v.rows(), v.sel, v.pinned != "", v.focus, v.armed != "")
	}
	draw := func() error {
		picture, er := renderDeskArrangementsSurface(*v)
		if er != nil {
			return er
		}
		pix, er := xproto.NewPixmapId(c)
		if er != nil {
			return er
		}
		if er = xproto.CreatePixmapChecked(c, 24, pix, xproto.Drawable(w), deskArrWinW, deskArrWinH).Check(); er != nil {
			return er
		}
		defer xproto.FreePixmap(c, pix)
		sampler := newViewerPixelSource(picture, picture.Bounds(), deskArrWinW, deskArrWinH)
		uploads := make([]xproto.PutImageCookie, 0, (deskArrWinH+15)/16)
		for y := 0; y < deskArrWinH; y += 16 {
			n := min(16, deskArrWinH-y)
			uploads = append(uploads, xproto.PutImageChecked(c, xproto.ImageFormatZPixmap, xproto.Drawable(pix), gc, deskArrWinW, uint16(n), 0, int16(y), 0, 24, sampler.strip(y, n)))
		}
		for _, upload := range uploads {
			if er = upload.Check(); er != nil {
				return er
			}
		}
		if er = xproto.ChangeWindowAttributesChecked(c, w, xproto.CwBackPixmap, []uint32{uint32(pix)}).Check(); er != nil {
			return er
		}
		if er = xproto.CopyAreaChecked(c, xproto.Drawable(pix), xproto.Drawable(w), gc, 0, 0, 0, 0, deskArrWinW, deskArrWinH).Check(); er != nil {
			return er
		}
		bounded := v.notice
		if len(bounded) > 120 {
			bounded = bounded[:120]
		}
		title := fmt.Sprintf("Mnet Shared Desk: arrangements %s notice[%s]", state(), bounded)
		return xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
	}
	// activate maps a focused control to a journey action. A row activates as
	// Inspect; disabled buttons cannot activate.
	activate := func() (int, bool) {
		if v.focus >= 0 && v.focus < v.rows() {
			v.sel = v.focus
			return deskArrActInspect, true
		}
		b := v.focus - v.rows()
		if b >= 0 && b < 5 && v.buttonEnabled(buttonAction[b]) {
			return buttonAction[b], true
		}
		return deskArrActClose, false
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		return deskArrActClose, err
	}
	if err = draw(); err != nil {
		return deskArrActClose, err
	}
	for {
		before := fmt.Sprintf("%s notice[%s]", state(), v.notice)
		expose := false
		acted, fired := -1, false
		select {
		case <-owner.Done():
			return deskArrActClose, nil
		case ev := <-events:
			if ev.err != nil || ev.event == nil {
				return deskArrActClose, errors.New("arrangements display ended")
			}
			switch ev := ev.event.(type) {
			case xproto.ExposeEvent:
				expose = true
			case xproto.ButtonPressEvent:
				clear()
				if ev.Detail == 1 {
					if r := rowAt(ev.EventX, ev.EventY); r >= 0 {
						v.sel, v.focus, pressed = r, r, r
						v.armed = "" // choosing a row cancels a pending delete
					} else if b := buttonAt(ev.EventX, ev.EventY); b >= 0 && v.buttonEnabled(buttonAction[b]) {
						v.focus, pressed = v.rows()+b, 100+b
					}
				}
			case xproto.ButtonReleaseEvent:
				old, key := pressed, held
				clear()
				if key != 0 || ev.Detail != 1 || old < 0 {
					break
				}
				if old < 100 && rowAt(ev.EventX, ev.EventY) == old {
					v.sel, v.focus = old, old
				} else if old >= 100 && buttonAt(ev.EventX, ev.EventY) == old-100 && v.buttonEnabled(buttonAction[old-100]) {
					acted, fired = buttonAction[old-100], true
				}
			case xproto.KeyPressEvent:
				mapping, er := xproto.GetKeyboardMapping(c, ev.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return deskArrActClose, errors.New("arrangements keyboard unavailable")
				}
				sym := uint32(mapping.Keysyms[0])
				clear()
				switch sym {
				case 0xff1b:
					return deskArrActClose, nil
				case 0xff09, 0xfe20:
					step := 1
					if ev.State&xproto.ModMaskShift != 0 || sym == 0xfe20 {
						step = -1
					}
					n := v.focusables()
					v.focus = (v.focus + step + n) % n
				case 0xff52:
					if v.rows() > 0 {
						v.sel--
						if v.sel < 0 {
							v.sel = v.rows() - 1
						}
						v.focus = v.sel
						v.armed = ""
					}
				case 0xff54:
					if v.rows() > 0 {
						v.sel = (v.sel + 1) % v.rows()
						v.focus = v.sel
						v.armed = ""
					}
				case 0xff0d, 0x20:
					if a, ok := activate(); ok {
						pressed, held = 200, sym
						_ = a
					}
				}
			case xproto.KeyReleaseEvent:
				mapping, er := xproto.GetKeyboardMapping(c, ev.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return deskArrActClose, errors.New("arrangements keyboard unavailable")
				}
				old, key := pressed, held
				clear()
				if key != 0 && uint32(mapping.Keysyms[0]) == key && old == 200 {
					if a, ok := activate(); ok {
						acted, fired = a, true
					}
				}
			case xproto.FocusOutEvent:
				clear()
			case xproto.ConfigureNotifyEvent:
				if nativeControlsRootChanged(screen, ev) {
					return deskArrActClose, errors.New("native controls root geometry changed")
				}
				if ev.Window == w && (ev.Width != deskArrWinW || ev.Height != deskArrWinH) {
					return deskArrActClose, errors.New("arrangements unsupported resize")
				}
			case xproto.DestroyNotifyEvent:
				return deskArrActClose, nil
			case xproto.ClientMessageEvent:
				if ev.Type == protocol.Atom && ev.Format == 32 && xproto.Atom(ev.Data.Data32[0]) == atom.Atom {
					return deskArrActClose, nil
				}
			default:
				continue
			}
		}
		if fired {
			return acted, nil
		}
		if expose || fmt.Sprintf("%s notice[%s]", state(), v.notice) != before {
			if err = draw(); err != nil {
				return deskArrActClose, err
			}
		}
	}
}

// The Shared Desk home journey: browse drafts, create a starter, inspect or
// edit existing drafts, each as its own owned window, then return. Saving
// always creates one new file in the private store; nothing is replaced.
// The reopened list shows each save, so home needs no separate save notice.
func runDeskHomeJourney(ctx context.Context, deskDir, layoutPath string) error {
	if deskDir == "" {
		return errors.New("arrangement store unavailable")
	}
	notice := ""
	pinned := ""
	armed := ""
	var armedID deskDraftID
	armedAt := time.Time{}
	selKeep := -1
	if layoutPath != "" {
		pinned = filepath.Base(layoutPath)
	}
	for ctx.Err() == nil {
		entries, err := listDeskStore(deskDir)
		if err != nil {
			return err
		}
		v := &deskArrangementsView{pinned: pinned, entries: entries, sel: selKeep, notice: notice, armed: armed}
		notice = ""
		action, err := showDeskArrangements(ctx, v)
		if err != nil || action == deskArrActClose || ctx.Err() != nil {
			return err
		}
		selKeep = v.sel
		armed = v.armed // selection changes inside the window cancel a pending delete
		rowPath := ""
		if v.sel >= 0 && v.sel < v.rows() {
			if pinned != "" && v.sel == 0 {
				rowPath = layoutPath
			} else {
				rowPath = filepath.Join(deskDir, entries[v.sel-boolInt(pinned != "")].Name)
			}
		}
		if action != deskArrActDelete {
			armed = ""
		}
		switch action {
		case deskArrActStarter:
			out, er := deskStoreOutPath(deskDir, entries)
			if er != nil {
				notice = er.Error()
				continue
			}
			if _, er = showDeskLayoutStarter(ctx, out); er != nil {
				return er
			}
		case deskArrActInspect:
			if rowPath == "" {
				notice = "Choose a draft to inspect."
				continue
			}
			preview, er := readDeskLayoutPreviewFile(rowPath)
			if er != nil {
				notice = "That draft could not be opened."
				continue
			}
			if er = showDeskLayoutPreview(ctx, preview); er != nil {
				return er
			}
		case deskArrActEdit:
			if rowPath == "" {
				notice = "Choose a draft to edit."
				continue
			}
			editor, er := readDeskLayoutEditorFile(rowPath)
			if er != nil {
				notice = "That draft could not be opened for editing."
				continue
			}
			out, er := deskStoreOutPath(deskDir, entries, pinned)
			if er != nil {
				notice = er.Error()
				continue
			}
			if _, er = showDeskLayoutEditor(ctx, editor, out); er != nil {
				return er
			}
		case deskArrActDelete:
			rowName := ""
			if rowPath != "" && !(pinned != "" && v.sel == 0) {
				rowName = filepath.Base(rowPath)
			}
			if rowName == "" {
				notice = "Choose a saved draft to delete."
				continue
			}
			if armed != rowName || time.Since(armedAt) > 15*time.Second {
				id, er := statDeskDraft(deskDir, rowName)
				if er != nil {
					notice = "That draft could not be deleted."
					continue
				}
				armed, armedID, armedAt = rowName, id, time.Now()
				continue
			}
			armed = ""
			if er := deleteDeskStoreEntry(deskDir, rowName, armedID); errors.Is(er, errDeskDraftChanged) {
				notice = "That draft changed; nothing was removed."
				continue
			} else if er != nil {
				notice = "That draft could not be deleted. Nothing was removed."
				continue
			}
			notice = "Deleted " + rowName + "."
		}
	}
	return nil
}
