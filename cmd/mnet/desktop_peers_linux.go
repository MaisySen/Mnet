//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"net/http"
	"os"
	"time"
)

type desktopPeerRead struct {
	snapshot desktopPeerSnapshot
	err      error
}
type peerPickerTarget struct {
	x, y, width, height int16
	label               string
	enabled             bool
}

// One local chooser. It neither dials nor grants input. Returning a choice
// requires a current selected row and a matching focused/button gesture.
func chooseDesktopPeer(parent context.Context, hc *http.Client, base string) (*desktopPeerChoice, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return nil, errors.New("peer chooser requires local X11, no fallback")
	}
	if parent == nil || parent.Err() != nil {
		return nil, errors.New("peer chooser owner unavailable")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return nil, errors.New("peer chooser requires local Linux X11")
	}
	defer c.Close()
	screen := xproto.Setup(c).DefaultScreen(c)
	if !nativeControlsRGB(screen) {
		return nil, errors.New("native controls require supported 24-bit TrueColor X11")
	}
	if screen.WidthInPixels < 940 || screen.HeightInPixels < 600 {
		return nil, errors.New("peer chooser requires at least 940x600 pixels")
	}
	w, err := xproto.NewWindowId(c)
	if err != nil {
		return nil, err
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 20, 20, 900, 560, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{0x17202e, xproto.EventMaskExposure | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskFocusChange | xproto.EventMaskStructureNotify}).Check(); err != nil {
		return nil, err
	}
	font, err := xproto.NewFontId(c)
	if err != nil {
		return nil, err
	}
	if err = xproto.OpenFontChecked(c, font, 5, "fixed").Check(); err != nil {
		return nil, err
	}
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		return nil, err
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{0xffffff, 0x17202e, uint32(font)}).Check(); err != nil {
		return nil, err
	}
	atom, err := xproto.InternAtom(c, false, 16, "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return nil, err
	}
	protocol, err := xproto.InternAtom(c, false, 12, "WM_PROTOCOLS").Reply()
	if err != nil {
		return nil, err
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(atom.Atom))
	if err = xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, protocol.Atom, xproto.AtomAtom, 32, 1, data).Check(); err != nil {
		return nil, err
	}
	if err = xproto.MapWindowChecked(c, w).Check(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
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
	result := make(chan desktopPeerRead, 1)
	selection := newDesktopPeerSelection()
	loading := false
	page := 0
	generation := 0
	message := "Reading current local authorization, no connection made."
	phase := "loading"
	focus, pressed := -1, -1
	var held uint32
	clear := func() { pressed = -1; held = 0 }
	targets := func() []peerPickerTarget {
		current := selection.current(time.Now())
		chosen := current && selection.selected >= 0
		list := make([]peerPickerTarget, 0, 12)
		for i := 0; i < 6; i++ {
			index := page*6 + i
			label := ""
			if current && index < len(selection.snapshot.Peers) {
				p := selection.snapshot.Peers[index]
				label = fmt.Sprintf("%-36s %s", deviceText(p.Name, 36), deviceText(p.IPv4, 40))
			}
			list = append(list, peerPickerTarget{20, int16(100 + i*46), 860, 44, label, current && index < len(selection.snapshot.Peers)})
		}
		return append(list, peerPickerTarget{20, 490, 140, 44, "REFRESH", !loading}, peerPickerTarget{170, 490, 110, 44, "PREVIOUS", current && page > 0}, peerPickerTarget{290, 490, 110, 44, "NEXT", current && (page+1)*6 < len(selection.snapshot.Peers)}, peerPickerTarget{410, 490, 140, 44, "VIEW ONLY", chosen}, peerPickerTarget{560, 490, 160, 44, "POINTER VIEW", chosen}, peerPickerTarget{730, 490, 150, 44, "CLOSE", true})
	}
	refresh := func() {
		if loading {
			return
		}
		if requestDone != nil {
			<-requestDone
		}
		selection.clear()
		clear()
		focus = -1
		page = 0
		generation++
		phase = "loading"
		message = "Reading current local authorization. Previous rows/selection cleared."
		loading = true
		readCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		requestCancel = stop
		requestDone = make(chan struct{})
		done := requestDone
		go func() {
			defer close(done)
			defer stop()
			snapshot, er := readDesktopPeerSnapshot(readCtx, hc, base)
			select {
			case result <- desktopPeerRead{snapshot, er}:
			case <-ctx.Done():
			}
		}()
	}
	draw := func() error {
		fill := func(color uint32, r xproto.Rectangle) error {
			if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground, []uint32{color}).Check(); er != nil {
				return er
			}
			return xproto.PolyFillRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{r}).Check()
		}
		text := func(x, y int16, label string, background, foreground uint32) error {
			if er := xproto.ChangeGCChecked(c, gc, xproto.GcForeground|xproto.GcBackground, []uint32{foreground, background}).Check(); er != nil {
				return er
			}
			return xproto.ImageText8Checked(c, byte(len(label)), xproto.Drawable(w), gc, x, y, label).Check()
		}
		if er := fill(0x17202e, xproto.Rectangle{Width: 900, Height: 560}); er != nil {
			return er
		}
		lines := []string{"REMOTE DESKTOP / CURRENT LOCAL PEERS", "Selection is not view/control approval or verified Multies identity. No auto-connect.", message}
		for i, line := range lines {
			if er := text(20, int16(28+i*24), line, 0x17202e, 0xffffff); er != nil {
				return er
			}
		}
		list := targets()
		for i, t := range list {
			bg := uint32(0x28384e)
			if !t.enabled {
				bg = 0x323744
			}
			if i < 6 && page*6+i == selection.selected {
				bg = 0x0066cc
			}
			if i == 11 {
				bg = 0x9b343e
			}
			if er := fill(bg, xproto.Rectangle{X: t.x, Y: t.y, Width: uint16(t.width), Height: uint16(t.height)}); er != nil {
				return er
			}
			foreground := uint32(0xffffff)
			if !t.enabled {
				foreground = 0x9ca3af
			}
			if er := text(t.x+12, t.y+25, t.label, bg, foreground); er != nil {
				return er
			}
			if focus == i && t.enabled {
				if er := xproto.PolyRectangleChecked(c, xproto.Drawable(w), gc, []xproto.Rectangle{{X: t.x + 3, Y: t.y + 3, Width: uint16(t.width - 7), Height: uint16(t.height - 7)}}).Check(); er != nil {
					return er
				}
			}
		}
		for i, line := range selection.detailLines(time.Now()) {
			if er := text(20, int16(390+i*22), line, 0x17202e, 0xffffff); er != nil {
				return er
			}
		}
		title := fmt.Sprintf("Mnet Remote Desktop: choose [%s] read%d page%d", phase, generation, page+1)
		return xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check()
	}
	activate := func(index int) (*desktopPeerChoice, bool, error) {
		clear()
		list := targets()
		if index < 0 || index >= len(list) || !list[index].enabled {
			return nil, false, nil
		}
		if index < 6 {
			selection.choose(page*6+index, time.Now())
			return nil, false, nil
		}
		switch index {
		case 6:
			refresh()
		case 7:
			page--
		case 8:
			page++
		case 9, 10:
			choice, er := selection.choice(index == 10, time.Now())
			if er != nil {
				return nil, false, er
			}
			return &choice, true, nil
		case 11:
			return nil, true, nil
		}
		return nil, false, nil
	}
	hit := func(x, y int16) int {
		for i, t := range targets() {
			if t.enabled && x >= t.x && x < t.x+t.width && y >= t.y && y < t.y+t.height {
				return i
			}
		}
		return -1
	}
	refresh()
	if err = draw(); err != nil {
		return nil, err
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		changed := true
		index := -1
		select {
		case <-ctx.Done():
			return nil, nil
		case r := <-result:
			loading = false
			clear()
			focus = -1
			selection.clear()
			page = 0
			if r.err != nil {
				phase = "unavailable"
				message = "Current local evidence unavailable. Refresh to try again."
			} else {
				selection.replace(r.snapshot)
				phase = "ready"
				message = fmt.Sprintf("%d current authorized session peers. Labels are not a person's identity.", len(r.snapshot.Peers))
			}
		case <-tick.C:
			if !loading && !selection.snapshot.Sampled.IsZero() && !selection.current(time.Now()) {
				selection.clear()
				clear()
				focus = -1
				page = 0
				phase = "expired"
				message = "Snapshot expired. Old peers and selection cleared. Refresh before choosing."
			} else {
				changed = false
			}
		case ev := <-events:
			if ev.err != nil || ev.event == nil {
				return nil, errors.New("peer chooser display ended")
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
				p := pressed
				key := held
				clear()
				if key == 0 && e.Detail == 1 && p >= 0 && hit(e.EventX, e.EventY) == p {
					index = p
				}
			case xproto.KeyPressEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return nil, errors.New("peer chooser keyboard unavailable")
				}
				sym := uint32(mapping.Keysyms[0])
				clear()
				switch sym {
				case 0xff1b:
					return nil, nil
				case 'r', 'R':
					refresh()
				case 0xff09, 0xfe20:
					step := 1
					if e.State&xproto.ModMaskShift != 0 || sym == 0xfe20 {
						step = -1
						if focus < 0 {
							focus = 0
						}
					}
					list := targets()
					for n := 0; n < len(list); n++ {
						focus = (focus + step + len(list)) % len(list)
						if list[focus].enabled {
							break
						}
					}
				case 0xff55, 0xff56:
					page, _ = selection.pageForKey(sym, page, time.Now())
					focus = -1
				case 0xff50, 0xff57:
					if selection.endpoint(sym == 0xff57, time.Now()) {
						page = selection.selected / 6
						focus = selection.selected % 6
					}
				case 0xff52, 0xff54:
					next := selection.selected
					if sym == 0xff52 {
						next--
					} else {
						next++
					}
					if selection.choose(next, time.Now()) {
						page = next / 6
						focus = next % 6
					}
				case 0xff0d, 0x20:
					if focus >= 0 && targets()[focus].enabled {
						pressed = focus
						held = sym
					}
				}
			case xproto.KeyReleaseEvent:
				mapping, er := xproto.GetKeyboardMapping(c, e.Detail, 1).Reply()
				if er != nil || len(mapping.Keysyms) == 0 {
					return nil, errors.New("peer chooser keyboard unavailable")
				}
				p, key := pressed, held
				clear()
				if key != 0 && uint32(mapping.Keysyms[0]) == key && p == focus {
					index = p
				}
			case xproto.ClientMessageEvent:
				if e.Type == protocol.Atom && e.Format == 32 && xproto.Atom(e.Data.Data32[0]) == atom.Atom {
					return nil, nil
				}
			case xproto.ConfigureNotifyEvent:
				if e.Width != 900 || e.Height != 560 {
					return nil, errors.New("peer chooser unsupported resize")
				}
			case xproto.DestroyNotifyEvent:
				return nil, nil
			default:
				changed = false
			}
		}
		if index >= 0 {
			choice, done, er := activate(index)
			if er != nil || done {
				return choice, er
			}
		}
		if changed {
			if err = draw(); err != nil {
				return nil, err
			}
		}
	}
}
