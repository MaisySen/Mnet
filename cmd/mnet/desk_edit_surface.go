package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"fintex.au/mnet/internal/desk"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Offline editor pixels only. Native ownership, hit targets and the save path
// live in the window owner. Geometry numbers come from the exact layout, never
// from rounded diagram pixels.
const (
	deskEditWinW    = 900
	deskEditWinH    = 730
	deskEditDiagram = 240
	deskEditRow1Y   = 486
	deskEditRow2Y   = 540
	deskEditRow3Y   = 594
	deskEditCellW   = 92
	deskEditCellH   = 44
	deskEditCellX0  = 20
	deskEditCellGap = 96
)

// Focusable controls, in Tab order. Rows one and two are dense grids; row
// three uses only the listed cells.
var deskEditLabels = []string{
	"Previous", "Next", "Left", "Right", "Top", "Bottom", "Add", "Remove", "Undo",
	"Move left", "Move right", "Move up", "Move down", "- width", "+ width", "- height", "+ height",
	"- px wide", "+ px wide", "- px high", "+ px high",
	"Save", "Close",
}

const (
	deskEditSave  = 21
	deskEditClose = 22
)

// deskEditCell returns the top-left cell position of a focusable control.
func deskEditCell(i int) (int, int) {
	switch {
	case i < 9:
		return deskEditCellX0 + i*deskEditCellGap, deskEditRow1Y
	case i < 17:
		return deskEditCellX0 + (i-9)*deskEditCellGap, deskEditRow2Y
	case i < 21:
		return deskEditCellX0 + (i-17)*deskEditCellGap, deskEditRow3Y
	default:
		return deskEditCellX0 + (i-21+7)*deskEditCellGap, deskEditRow3Y
	}
}

// deskEditControlAt maps window pixels to a focusable control, or -1. Empty
// row-three cells are not targets.
func deskEditControlAt(x, y int16) int {
	for i := range deskEditLabels {
		cx, cy := deskEditCell(i)
		if int(x) >= cx && int(x) < cx+deskEditCellW && int(y) >= cy && int(y) < cy+deskEditCellH {
			return i
		}
	}
	return -1
}

func renderDeskEditSurface(e *deskLayoutEditor, focus int, confirm bool, notice string) (*image.RGBA, error) {
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
	picture := image.NewRGBA(image.Rect(0, 0, deskEditWinW, deskEditWinH))
	rgb := func(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }
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
		if font.MeasureString(face, s).Ceil() > width {
			renderErr = errors.New("editor text exceeds presentation bounds")
			return
		}
		d := font.Drawer{Dst: picture, Src: image.NewUniform(rgb(c)), Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	rect(0, 0, deskEditWinW, deskEditWinH, 0xf4f6fa)
	rect(0, 0, deskEditWinW, 58, 0xffffff)
	rect(20, 12, 32, 32, 0x103e89)
	text(26, 36, 24, 25, true, 0xffffff, "M")
	text(64, 36, 600, 23, true, 0x172b4d, "Shared Desk arrangement editor")
	text(742, 34, 140, 12, true, 0x63758c, "OFFLINE EDITOR")
	text(20, 82, 860, 14, false, 0x526780, "Change this arrangement locally. Nothing is paired, enabled or shared.")

	if confirm {
		rect(20, 96, 860, deskEditDiagram, 0xffffff)
		text(40, 200, 820, 22, true, 0x172b4d, "Discard your unsaved changes?")
		text(40, 234, 820, 14, false, 0x526780, "The opened layout file is never modified. Only unsaved in-memory edits are lost.")
		text(40, 258, 820, 14, false, 0x526780, "Escape or an outside press keeps editing.")
		for i, label := range []string{"Discard changes", "Keep editing"} {
			x := 20 + i*170
			bg, fg := uint32(0xffffff), uint32(0x526780)
			if i == 0 {
				bg, fg = 0xb3261e, 0xffffff
			}
			rect(x, 300, 160, 44, bg)
			text(x+10, 327, 140, 13, true, fg, label)
			if focus == i {
				ring := uint32(0xffffff)
				if bg == 0xffffff {
					ring = 0x2563c7
				}
				rect(x+4, 303, 152, 1, ring)
				rect(x+4, 340, 152, 1, ring)
			}
		}
		if renderErr != nil {
			return nil, renderErr
		}
		return picture, nil
	}

	rects, err := e.rectangles(840, deskEditDiagram-10)
	if err != nil {
		return nil, err
	}
	rect(20, 96, 860, deskEditDiagram, 0xffffff)
	for i, r := range rects {
		bg, fg := uint32(0xdce5f2), uint32(0x214f90)
		if i == e.selected {
			bg, fg = 0x0066cc, 0xffffff
		}
		rect(int(30+r.x), int(100+r.y), int(r.w), int(r.h), bg)
		if r.w >= 24 && r.h >= 24 {
			text(int(34+r.x), int(117+r.y), 20, 13, true, fg, fmt.Sprint(i+1))
		}
	}
	// A yellow strip marks the edge a new display would attach to. It is a
	// placement hint, never live input.
	sel := rects[e.selected]
	switch e.edge {
	case desk.Left:
		rect(int(30+sel.x), int(100+sel.y), 4, int(sel.h), 0xffd166)
	case desk.Right:
		rect(int(30+sel.x+sel.w-4), int(100+sel.y), 4, int(sel.h), 0xffd166)
	case desk.Top:
		rect(int(30+sel.x), int(100+sel.y), int(sel.w), 4, 0xffd166)
	case desk.Bottom:
		rect(int(30+sel.x), int(100+sel.y+sel.h-4), int(sel.w), 4, 0xffd166)
	}

	d := e.display()
	text(20, 366, 860, 13, true, 0x214f90, fmt.Sprintf("Display %d of %d: %s", e.selected+1, len(e.layout.Displays), d.ID))
	text(20, 388, 860, 13, false, 0x526780, "Device label: "+d.Device)
	text(20, 410, 860, 12, false, 0x526780, fmt.Sprintf("Position (%d, %d), size %dx%d, native pixels %dx%d", d.X, d.Y, d.Width, d.Height, d.PixelWidth, d.PixelHeight))
	text(20, 432, 860, 12, false, 0x63758c, fmt.Sprintf("New displays attach to the %s edge of the selected display.", e.edge))
	if verr := e.validate(); verr != nil {
		text(20, 456, 860, 13, true, 0xb3261e, "Arrangement not valid: "+verr.Error())
	} else if e.dirty() {
		text(20, 456, 860, 13, true, 0x1a7f37, "Arrangement valid. Save writes a new file; the opened file is never replaced.")
	} else {
		text(20, 456, 860, 13, false, 0x63758c, "No unsaved changes.")
	}
	if notice != "" {
		text(20, 476, 860, 12, true, 0xb3261e, notice)
	}

	valid := e.validate() == nil
	disabled := func(i int) bool {
		switch i {
		case 6:
			return len(e.layout.Displays) >= 32
		case 7:
			return len(e.layout.Displays) <= 2
		case 8:
			return len(e.undo) == 0
		case deskEditSave:
			return !valid || !e.dirty()
		}
		return false
	}
	for i, label := range deskEditLabels {
		x, y := deskEditCell(i)
		bg, fg := uint32(0xffffff), uint32(0x526780)
		if i >= 2 && i <= 5 && desk.Edge([]string{"left", "right", "top", "bottom"}[i-2]) == e.edge {
			bg, fg = 0x0066cc, 0xffffff
		}
		if i == deskEditSave && valid && e.dirty() {
			bg, fg = 0x0066cc, 0xffffff
		}
		if disabled(i) {
			bg, fg = 0xe2e8f0, 0x9aa7b8
		}
		rect(x, y, deskEditCellW, deskEditCellH, bg)
		text(x+8, y+27, deskEditCellW-16, 13, true, fg, label)
		if i == focus {
			rect(x+3, y+3, deskEditCellW-6, 1, 0xffffff)
			rect(x+3, y+deskEditCellH-4, deskEditCellW-6, 1, 0xffffff)
			if bg == 0xffffff {
				rect(x+3, y+3, deskEditCellW-6, 1, 0x2563c7)
				rect(x+3, y+deskEditCellH-4, deskEditCellW-6, 1, 0x2563c7)
			}
		}
	}
	text(20, 668, 860, 12, false, 0x63758c, "Arrows move the selected display (Shift = 10). Page Up/Down chooses a display. Tab selects controls.")
	text(20, 716, 860, 12, false, 0x63758c, "Closing without saving discards changes. The source file is never modified.")
	if renderErr != nil {
		return nil, renderErr
	}
	return picture, nil
}
