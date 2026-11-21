package main

import (
	"fmt"
	"image"
	"time"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The arrangements window manages offline draft files only. Pairing, input
// and any network or daemon path stay unavailable and unreachable here.
const (
	deskArrWinW = 900
	deskArrWinH = 560
	deskArrRows = 9
)

const (
	deskArrActClose = iota
	deskArrActStarter
	deskArrActInspect
	deskArrActEdit
	deskArrActDelete
	deskArrActDuplicate
)

type deskArrangementsView struct {
	pinned  string // base name of an explicitly opened file, "" when none
	entries []deskStoreEntry
	sel     int // selected row: pinned first, then store entries
	focus   int // rows 0..rows-1, then New starter, Inspect, Edit, Duplicate, Delete, Close
	notice  string
	armed   string // store draft awaiting a second explicit Delete confirmation
	// armedUntil is when the pending delete lapses; the window clears the
	// prompt itself so an expired delete never stays armed-looking.
	armedUntil time.Time
}

func (v deskArrangementsView) rows() int {
	n := len(v.entries)
	if v.pinned != "" {
		n++
	}
	return n
}

// focusables counts every Tab stop: rows, then the six buttons.
func (v deskArrangementsView) focusables() int { return v.rows() + 6 }

func (v deskArrangementsView) buttonEnabled(button int) bool {
	switch button {
	case deskArrActStarter, deskArrActClose:
		return true
	case deskArrActDelete, deskArrActDuplicate:
		// Only an app-owned store draft can be deleted or duplicated, never
		// an explicitly opened external file.
		return v.sel >= boolInt(v.pinned != "") && v.sel < v.rows()
	default:
		return v.rows() > 0 && v.sel >= 0 && v.sel < v.rows()
	}
}

func (v *deskArrangementsView) clamp() {
	if v.rows() == 0 {
		v.sel = -1
	} else if v.sel < 0 {
		v.sel = 0
	} else if v.sel >= v.rows() {
		v.sel = v.rows() - 1
	}
	if v.focus < 0 || v.focus >= v.focusables() {
		v.focus = -1
	}
}

func deskArrButtonLabel(button int) string {
	return []string{"Close", "New starter", "Inspect", "Edit", "Delete", "Duplicate"}[button]
}

func renderDeskArrangementsSurface(v deskArrangementsView) (*image.RGBA, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	faces := map[int]font.Face{}
	defer func() {
		for _, f := range faces {
			_ = f.Close()
		}
	}()
	picture := image.NewRGBA(image.Rect(0, 0, deskArrWinW, deskArrWinH))
	rgb := func(c uint32) color.RGBA { return color.RGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 255} }
	rect := func(x, y, w, h int, c uint32) {
		draw.Draw(picture, image.Rect(x, y, x+w, y+h), image.NewUniform(rgb(c)), image.Point{}, draw.Src)
	}
	var renderErr error
	text := func(x, y, width, size int, heavy bool, c uint32, s string) {
		if renderErr != nil {
			return
		}
		key := size
		if heavy {
			key += 100
		}
		face := faces[key]
		if face == nil {
			ttf := regular
			if heavy {
				ttf = bold
			}
			face, renderErr = opentype.NewFace(ttf, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
			if renderErr != nil {
				return
			}
			faces[key] = face
		}
		d := font.Drawer{Dst: picture, Src: image.NewUniform(rgb(c)), Face: face, Dot: fixed.P(x, y)}
		for _, r := range s {
			if d.MeasureString(string(r)).Ceil()+d.Dot.X.Ceil()-x > width {
				return
			}
			d.DrawString(string(r))
		}
	}

	rect(0, 0, deskArrWinW, deskArrWinH, 0xf4f6fa)
	rect(0, 0, deskArrWinW, 58, 0xffffff)
	rect(20, 12, 32, 32, 0x103e89)
	text(26, 36, 24, 25, true, 0xffffff, "M")
	text(64, 36, 560, 23, true, 0x172b4d, "Shared Desk arrangements")
	text(700, 34, 180, 12, true, 0x63758c, "OFFLINE DRAFTS")
	text(20, 82, 860, 14, false, 0x526780, "Create, inspect and edit local arrangement drafts. Pairing and input stay unavailable.")

	rows := v.rows()
	rect(20, 100, 860, 330, 0xffffff)
	if rows == 0 {
		text(40, 150, 820, 14, true, 0x172b4d, "No arrangement drafts yet")
		text(40, 174, 820, 13, false, 0x63758c, "An arrangement names where each display sits on this desk. It stays a local file.")
		text(40, 194, 820, 13, false, 0x63758c, "Choose New starter to create one. Nothing is shared, paired or connected here.")
	} else {
		for i := 0; i < rows && i < deskArrRows; i++ {
			y := 108 + i*36
			if i == v.sel {
				rect(28, y, 844, 32, 0xe3ecf9)
			}
			var name, detail string
			if v.pinned != "" && i == 0 {
				name = v.pinned
				detail = "opened file"
			} else {
				e := v.entries[i-boolInt(v.pinned != "")]
				name = e.Name
				if e.Invalid {
					detail = "not a valid arrangement file"
				} else {
					detail = fmt.Sprintf("%d displays", e.Displays)
					if e.Placeholders {
						detail += ", placeholder labels"
					}
				}
			}
			text(44, y+21, 400, 13, true, 0x172b4d, name)
			text(460, y+21, 390, 12, false, 0x63758c, detail)
			if v.focus == i {
				rect(28, y, 4, 32, 0x2563c7)
			}
		}
		if rows > deskArrRows {
			text(44, 108+deskArrRows*36+8, 820, 12, false, 0x63758c, fmt.Sprintf("Showing %d of %d drafts.", deskArrRows, rows))
		}
	}

	buttons := []int{deskArrActStarter, deskArrActInspect, deskArrActEdit, deskArrActDuplicate, deskArrActDelete, deskArrActClose}
	for i, b := range buttons {
		x := 20 + i*145
		enabled := v.buttonEnabled(b)
		bg, fg := uint32(0xffffff), uint32(0x214f90)
		if !enabled {
			bg, fg = 0xecf0f6, 0x758299
		}
		rect(x, 452, 130, 44, bg)
		text(x+10, 480, 112, 14, true, fg, deskArrButtonLabel(b))
		if v.focus == rows+i && enabled {
			rect(x, 493, 130, 3, 0x2563c7)
		}
	}
	if v.armed != "" {
		name := v.armed
		if len(name) > 40 {
			name = name[:40]
		}
		text(20, 516, 700, 12, true, 0xb3261e, "Press Delete again to remove "+name+". Any other action cancels.")
	} else if v.notice != "" {
		text(20, 516, 700, 12, true, 0xb3261e, v.notice)
	}
	text(20, 544, 860, 12, false, 0x63758c, "Up/Down chooses a draft. Tab moves between drafts and buttons. Enter opens. Escape returns home.")
	if renderErr != nil {
		return nil, renderErr
	}
	return picture, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
