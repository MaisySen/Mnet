package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Pure rendering. Hit targets and selection/mode authority remain in the owner.
func renderDesktopChooser(selection desktopPeerSelection, page, focus int, message string, enabled []bool, now time.Time) (*image.RGBA, error) {
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
	picture := image.NewRGBA(image.Rect(0, 0, 900, 560))
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
			renderErr = errors.New("chooser text exceeds presentation bounds")
			return
		}
		d := font.Drawer{Dst: picture, Src: image.NewUniform(rgb(c)), Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}

	rect(0, 0, 900, 560, 0xf4f6fa)
	rect(0, 0, 900, 58, 0xffffff)
	rect(20, 12, 32, 32, 0x103e89)
	text(26, 36, 24, 25, true, 0xffffff, "M")
	text(64, 36, 550, 23, true, 0x172b4d, "Remote Desktop")
	text(742, 34, 140, 12, true, 0x63758c, "LOCAL PREVIEW")
	text(20, 78, 860, 14, false, 0x526780, "Choose a device. The host must approve viewing; pointer control needs another approval.")
	text(20, 96, 860, 12, false, 0x63758c, message)
	current := selection.current(now)
	for i := 0; i < 6; i++ {
		y := 100 + i*46
		index := page*6 + i
		active := current && index < len(selection.snapshot.Peers)
		bg, fg := uint32(0xffffff), uint32(0x172b4d)
		if !active {
			bg = 0xe8edf4
			fg = 0x758299
		}
		if active && index == selection.selected {
			bg = 0x0066cc
			fg = 0xffffff
		}
		rect(20, y, 860, 44, bg)
		if active {
			p := selection.snapshot.Peers[index]
			text(32, y+28, 530, 16, true, fg, deviceText(p.Name, 30))
			text(600, y+28, 260, 15, false, fg, deviceText(p.IPv4, 40))
		}
		if focus == i && active {
			rect(23, y+3, 854, 1, 0xffffff)
			rect(23, y+40, 854, 1, 0xffffff)
		}
	}
	selected := current && selection.selected >= 0 && selection.selected < len(selection.snapshot.Peers)
	if selected {
		p := selection.snapshot.Peers[selection.selected]
		text(20, 390, 860, 13, true, 0x214f90, fmt.Sprintf("Selected %d/%d: %s  |  %s", selection.selected+1, len(selection.snapshot.Peers), deviceText(p.Name, 28), deviceText(p.IPv4, 40)))
		text(20, 412, 860, 14, true, 0x172b4d, "Exact device key: "+p.PublicKey)
	} else {
		text(20, 390, 860, 14, false, 0x63758c, "Select a current row, then choose View only or Pointer view.")
		text(20, 412, 860, 13, false, 0x63758c, "Device labels are not verified people or Multies identities.")
	}
	text(20, 435, 860, 12, false, 0x63758c, "Pointer view requires host viewing approval, then a separate Ask Pointer and host answer.")
	text(20, 456, 860, 12, false, 0x63758c, "Snapshot clears after 30 seconds. Manual reads only. Files, recording and keyboard are not offered.")
	text(20, 478, 860, 12, false, 0x63758c, "Tab to focus. Arrows to select. PageUp/Down to page. Enter or Space to act. Escape to close.")
	for i, x := range []int{20, 170, 290, 410, 560, 730} {
		widths := []int{140, 110, 110, 140, 160, 150}
		active := len(enabled) > i+6 && enabled[i+6]
		bg, fg := uint32(0x2563c7), uint32(0xffffff)
		if !active {
			bg = 0xe8edf4
			fg = 0x758299
		}
		if i == 5 {
			bg = 0xffffff
			fg = 0x526780
		}
		rect(x, 490, widths[i], 44, bg)
		text(x+12, 518, widths[i]-24, 14, true, fg, []string{"Refresh", "Previous", "Next", "View only", "Pointer view", "Close"}[i])
		if focus == i+6 && active {
			rect(x+3, 493, widths[i]-6, 1, 0xffffff)
			rect(x+3, 530, widths[i]-6, 1, 0xffffff)
		}
	}
	if renderErr != nil {
		return nil, renderErr
	}
	return picture, nil
}
