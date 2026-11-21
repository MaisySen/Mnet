//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"golang.org/x/sys/unix"
)

// Fixed save-failure categories. Raw OS errors and destination paths are not
// rendered into the window.
var (
	errDeskEditSaveExists = errors.New("save destination already exists; nothing replaced")
	errDeskEditSaveParent = errors.New("save parent must be an existing private directory owned by you")
	errDeskEditSaveIO     = errors.New("save failed; a partial new file may remain, remove it before retrying")
)

// The edited arrangement can only ever create one new file: create-only,
// no-follow, anchored to a checked private parent, synced before success.
// The opened source file is never a destination because it already exists.
func writeDeskEditedLayout(outPath string, data []byte) error {
	outPath = filepath.Clean(outPath)
	base := filepath.Base(outPath)
	if base == "." || base == ".." || outPath == "/" {
		return errDeskEditSaveParent
	}
	parentFD, err := unix.Open(filepath.Dir(outPath), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errDeskEditSaveParent
	}
	defer unix.Close(parentFD)
	var parentStat unix.Stat_t
	if err = unix.Fstat(parentFD, &parentStat); err != nil {
		return errDeskEditSaveParent
	}
	if parentStat.Uid != uint32(os.Geteuid()) || parentStat.Mode&0022 != 0 {
		return errDeskEditSaveParent
	}
	fd, err := unix.Openat(parentFD, base, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return errDeskEditSaveExists
		}
		return errDeskEditSaveIO
	}
	f := os.NewFile(uintptr(fd), base)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if errors.Join(writeErr, syncErr, closeErr) != nil {
		return errDeskEditSaveIO
	}
	if err = unix.Fsync(parentFD); err != nil {
		return errDeskEditSaveIO
	}
	return nil
}

// Static offline editing cannot block on a writerless FIFO or an endless
// device. Inspect the opened descriptor, not a prior path stat.
func readDeskLayoutEditorFile(path string) (*deskLayoutEditor, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("layout file unavailable")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("offline layout file must be regular")
	}
	return readDeskLayoutEditor(f)
}

// A local-only editor window. No daemon, transport, pairing, capture or input
// owner is reachable here. Native events edit only this in-memory arrangement;
// saving creates one explicitly named new file and ends the window.
func showDeskLayoutEditor(ctx context.Context, e *deskLayoutEditor, outPath string) (bool, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return false, errors.New("layout editor requires local X11, no fallback")
	}
	if ctx == nil || ctx.Err() != nil || e == nil {
		return false, errors.New("offline editor unavailable")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return false, errors.New("layout editor requires local Linux X11")
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
		return false, errors.New("native controls require supported 24-bit TrueColor X11")
	}
	if !format || setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return false, errors.New("offline editor pixel format unsupported")
	}
	if screen.WidthInPixels < 940 || screen.HeightInPixels < 760 {
		return false, errors.New("layout editor requires at least 940x760 pixels")
	}
	if err = nativeControlsWatchRoot(c, screen); err != nil {
		return false, err
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return false, err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, deskEditWinW, deskEditWinH, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x17202e, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return false, err
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return false, err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), 0, nil).Check(); err != nil {
		return false, err
	}
	atom, err := xproto.InternAtom(c, false, 16, "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return false, err
	}
	protocol, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return false, err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(atom.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocol.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return false, err
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		return false, err
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

	focus, pressed := -1, -1
	confirm := false
	confirmFocus := -1
	notice := ""
	var held uint32
	clear := func() { pressed = -1; held = 0 }
	hit := func(x, y int16) int {
		if index := e.hitDisplay(840, deskEditDiagram-10, int64(x)-30, int64(y)-100); index >= 0 {
			return len(deskEditLabels) + index
		}
		return deskEditControlAt(x, y)
	}
	confirmHit := func(x, y int16) int {
		for i := 0; i < 2; i++ {
			cx, cy := 20+i*170, 300
			if int(x) >= cx && int(x) < cx+160 && int(y) >= cy && int(y) < cy+44 {
				return i
			}
		}
		return -1
	}
	disabled := func(i int) bool {
		switch i {
		case 6:
			return len(e.layout.Displays) >= 32
		case 7:
			return len(e.layout.Displays) <= 2
		case 8:
			return len(e.undo) == 0
		case deskEditSave:
			return e.validate() != nil || !e.dirty()
		}
		return false
	}
	state := func() string {
		d := e.display()
		return fmt.Sprintf("sel%d/%d edge%s x%d y%d w%d h%d pw%d ph%d undo%d", e.selected, len(e.layout.Displays), e.edge, d.X, d.Y, d.Width, d.Height, d.PixelWidth, d.PixelHeight, len(e.undo))
	}
	draw := func() error {
		picture, er := renderDeskEditSurface(e, focus, confirm, notice)
		if er != nil {
			return er
		}
		pix, er := xproto.NewPixmapId(c)
		if er != nil {
			return er
		}
		if er = xproto.CreatePixmapChecked(c, 24, pix, xproto.Drawable(w), deskEditWinW, deskEditWinH).Check(); er != nil {
			return er
		}
		defer xproto.FreePixmap(c, pix)
		sampler := newViewerPixelSource(picture, picture.Bounds(), deskEditWinW, deskEditWinH)
		uploads := make([]xproto.PutImageCookie, 0, (deskEditWinH+15)/16)
		for y := 0; y < deskEditWinH; y += 16 {
			n := min(16, deskEditWinH-y)
			uploads = append(uploads, xproto.PutImageChecked(c, xproto.ImageFormatZPixmap, xproto.Drawable(pix), gc, deskEditWinW, uint16(n), 0, int16(y), 0, 24, sampler.strip(y, n)))
		}
		for _, upload := range uploads {
			if er = upload.Check(); er != nil {
				return er
			}
		}
		if er = xproto.ChangeWindowAttributesChecked(c, w, xproto.CwBackPixmap, []uint32{uint32(pix)}).Check(); er != nil {
			return er
		}
		if er = xproto.CopyAreaChecked(c, xproto.Drawable(pix), xproto.Drawable(w), gc, 0, 0, 0, 0, deskEditWinW, deskEditWinH).Check(); er != nil {
			return er
		}
		bounded := notice
		if len(bounded) > 80 {
			bounded = bounded[:80]
		}
		title := fmt.Sprintf("Mnet Shared Desk: offline layout editor %s dirty%t valid%t confirm%t focus%d notice[%s]", state(), e.dirty(), e.validate() == nil, confirm, focus, bounded)
		return xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
	}
	// activate returns true only when the window should close (saved, or an
	// explicit discard). A refused control sets a bounded plain-language notice.
	activate := func(i int) bool {
		clear()
		if i >= len(deskEditLabels) {
			index := i - len(deskEditLabels)
			if index < len(e.layout.Displays) {
				e.selected = index
				notice = ""
				focus = -1
			}
			return false
		}
		if disabled(i) {
			switch i {
			case deskEditSave:
				notice = "Save needs unsaved changes and a valid arrangement"
			case 6:
				notice = "Arrangement already has the maximum 32 displays"
			case 7:
				notice = "An arrangement keeps at least 2 displays"
			case 8:
				notice = "Nothing to undo"
			}
			return false
		}
		notice = ""
		switch i {
		case 0:
			e.selectDisplay(-1)
		case 1:
			e.selectDisplay(1)
		case 2, 3, 4, 5:
			e.selectEdge([]desk.Edge{desk.Left, desk.Right, desk.Top, desk.Bottom}[i-2])
		case 6:
			if er := e.addDisplay(); er != nil {
				notice = er.Error()
			}
		case 7:
			if er := e.removeDisplay(); er != nil {
				notice = er.Error()
			}
		case 8:
			if !e.undoEdit() {
				notice = "Nothing to undo"
			}
		case 9:
			e.moveDisplay(-1, 0)
		case 10:
			e.moveDisplay(1, 0)
		case 11:
			e.moveDisplay(0, -1)
		case 12:
			e.moveDisplay(0, 1)
		case 13:
			e.resizeDisplay(-1, 0)
		case 14:
			e.resizeDisplay(1, 0)
		case 15:
			e.resizeDisplay(0, -1)
		case 16:
			e.resizeDisplay(0, 1)
		case 17:
			e.resizePixels(-1, 0)
		case 18:
			e.resizePixels(1, 0)
		case 19:
			e.resizePixels(0, -1)
		case 20:
			e.resizePixels(0, 1)
		case deskEditSave:
			data, er := e.saveBytes()
			if er != nil {
				notice = er.Error()
				return false
			}
			if er = writeDeskEditedLayout(outPath, data); er != nil {
				notice = er.Error()
				return false
			}
			return true
		case deskEditClose:
			if e.dirty() {
				confirm = true
				confirmFocus = -1
				focus = -1
				return false
			}
			return true
		}
		return false
	}
	if err = draw(); err != nil {
		return false, err
	}
	for {
		before := fmt.Sprintf("%s focus%d confirm%t notice[%s] dirty%t valid%t", state(), focus, confirm, notice, e.dirty(), e.validate() == nil)
		expose := false
		index := -1
		saved := false
		discard := false
		select {
		case <-owner.Done():
			return false, nil
		case ev := <-events:
			if ev.err != nil || ev.event == nil {
				return false, errors.New("offline editor display ended")
			}
			if confirm {
				switch ev := ev.event.(type) {
				case xproto.ExposeEvent:
					expose = true
				case xproto.ButtonPressEvent:
					clear()
					if ev.Detail == 1 {
						if h := confirmHit(ev.EventX, ev.EventY); h >= 0 {
							pressed = h
							confirmFocus = h
						} else {
							confirm = false
						}
					}
				case xproto.ButtonReleaseEvent:
					old, key := pressed, held
					clear()
					if key == 0 && ev.Detail == 1 && old >= 0 && confirmHit(ev.EventX, ev.EventY) == old {
						if old == 0 {
							discard = true
						} else {
							confirm = false
						}
					}
				case xproto.KeyPressEvent:
					mapping, er := xproto.GetKeyboardMapping(c, ev.Detail, 1).Reply()
					if er != nil || len(mapping.Keysyms) == 0 {
						return false, errors.New("offline editor keyboard unavailable")
					}
					sym := uint32(mapping.Keysyms[0])
					clear()
					switch sym {
					case 0xff1b:
						confirm = false
					case 0xff09, 0xfe20:
						step := 1
						if ev.State&xproto.ModMaskShift != 0 || sym == 0xfe20 {
							step = -1
						}
						confirmFocus = (confirmFocus + step + 2) % 2
					case 0xff0d, 0x20:
						if confirmFocus >= 0 {
							pressed = confirmFocus
							held = sym
						}
					default:
						confirm = false
					}
				case xproto.KeyReleaseEvent:
					mapping, er := xproto.GetKeyboardMapping(c, ev.Detail, 1).Reply()
					if er != nil || len(mapping.Keysyms) == 0 {
						return false, errors.New("offline editor keyboard unavailable")
					}
					old, key := pressed, held
					clear()
					if key != 0 && uint32(mapping.Keysyms[0]) == key && old == confirmFocus && old >= 0 {
						if old == 0 {
							discard = true
						} else {
							confirm = false
						}
					}
				case xproto.FocusOutEvent:
					clear()
				case xproto.ConfigureNotifyEvent:
					if nativeControlsRootChanged(screen, ev) {
						return false, errors.New("native controls root geometry changed")
					}
					if ev.Window == w && (ev.Width != deskEditWinW || ev.Height != deskEditWinH) {
						return false, errors.New("offline editor unsupported resize")
					}
				case xproto.DestroyNotifyEvent:
					return false, nil
				case xproto.ClientMessageEvent:
					if ev.Type == protocol.Atom && ev.Format == 32 && xproto.Atom(ev.Data.Data32[0]) == atom.Atom {
						confirm = false
					}
				default:
					continue
				}
				if discard {
					return false, nil
				}
				if expose || fmt.Sprintf("%s focus%d confirm%t notice[%s] dirty%t valid%t", state(), focus, confirm, notice, e.dirty(), e.validate() == nil) != before {
					if confirm {
						focus = confirmFocus
					}
					if err = draw(); err != nil {
						return false, err
					}
				}
				continue
			}
			switch xev := ev.event.(type) {
			case xproto.ExposeEvent:
				expose = true
			case xproto.FocusOutEvent:
				clear()
				focus = -1
			case xproto.ButtonPressEvent:
				clear()
				if xev.Detail == 1 {
					pressed = hit(xev.EventX, xev.EventY)
				}
			case xproto.ButtonReleaseEvent:
				old, key := pressed, held
				clear()
				if key == 0 && xev.Detail == 1 && old >= 0 && hit(xev.EventX, xev.EventY) == old {
					index = old
				}
			case xproto.KeyPressEvent:
				mapping, er := xproto.GetKeyboardMapping(c, xev.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return false, errors.New("offline editor keyboard unavailable")
				}
				sym := uint32(mapping.Keysyms[0])
				clear()
				switch sym {
				case 0xff1b:
					if e.dirty() {
						confirm = true
						confirmFocus = -1
						focus = -1
					} else {
						return false, nil
					}
				case 0xff09, 0xfe20:
					step := 1
					if xev.State&xproto.ModMaskShift != 0 || sym == 0xfe20 {
						step = -1
						if focus < 0 {
							focus = 0
						}
					}
					focus = (focus + step + len(deskEditLabels)) % len(deskEditLabels)
				case 0xff0d, 0x20:
					if focus >= 0 {
						pressed = focus
						held = sym
					}
				case 0xff51, 0xff52, 0xff53, 0xff54:
					step := int64(1)
					if xev.State&xproto.ModMaskShift != 0 {
						step = 10
					}
					switch sym {
					case 0xff51:
						if e.moveDisplay(-step, 0) {
							notice = ""
						}
					case 0xff53:
						if e.moveDisplay(step, 0) {
							notice = ""
						}
					case 0xff52:
						if e.moveDisplay(0, -step) {
							notice = ""
						}
					case 0xff54:
						if e.moveDisplay(0, step) {
							notice = ""
						}
					}
				case 0xff50:
					e.selected = 0
				case 0xff57:
					e.selected = len(e.layout.Displays) - 1
				case 0xff55:
					e.selectDisplay(-1)
				case 0xff56:
					e.selectDisplay(1)
				}
			case xproto.KeyReleaseEvent:
				mapping, er := xproto.GetKeyboardMapping(c, xev.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return false, errors.New("offline editor keyboard unavailable")
				}
				old, key := pressed, held
				clear()
				if key != 0 && uint32(mapping.Keysyms[0]) == key && old == focus && old >= 0 {
					index = old
				}
			case xproto.ConfigureNotifyEvent:
				if nativeControlsRootChanged(screen, xev) {
					return false, errors.New("native controls root geometry changed")
				}
				if xev.Window == w && (xev.Width != deskEditWinW || xev.Height != deskEditWinH) {
					return false, errors.New("offline editor unsupported resize")
				}
			case xproto.DestroyNotifyEvent:
				return false, nil
			case xproto.ClientMessageEvent:
				if xev.Type == protocol.Atom && xev.Format == 32 && xproto.Atom(xev.Data.Data32[0]) == atom.Atom {
					if e.dirty() {
						confirm = true
						confirmFocus = -1
						focus = -1
					} else {
						return false, nil
					}
				}
			default:
				continue
			}
		}
		if index >= 0 && activate(index) {
			saved = index == deskEditSave
			return saved, nil
		}
		if expose || fmt.Sprintf("%s focus%d confirm%t notice[%s] dirty%t valid%t", state(), focus, confirm, notice, e.dirty(), e.validate() == nil) != before {
			if err = draw(); err != nil {
				return false, err
			}
		}
	}
}
