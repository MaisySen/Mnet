package main

import (
	"errors"
	"fintex.au/mnet/internal/desk"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Offline pixels only. Native ownership, query geometry and hit targets remain
// in the window owner. Labels are local file data, never device identity.
func renderDeskSurface(p *deskLayoutPreview, focus int, details bool) (*image.RGBA, error) {
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
	picture := image.NewRGBA(image.Rect(0, 0, 900, 650))
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
			renderErr = errors.New("layout text exceeds presentation bounds")
			return
		}
		d := font.Drawer{Dst: picture, Src: image.NewUniform(rgb(c)), Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	rect(0, 0, 900, 650, 0xf4f6fa)
	rect(0, 0, 900, 58, 0xffffff)
	rect(20, 12, 32, 32, 0x103e89)
	text(26, 36, 24, 25, true, 0xffffff, "M")
	text(64, 36, 600, 23, true, 0x172b4d, "Shared Desk arrangement")
	text(742, 34, 140, 12, true, 0x63758c, "OFFLINE PREVIEW")
	text(20, 82, 700, 14, false, 0x526780, "Explore this saved arrangement. No keyboard or mouse sharing is active.")
	rects, err := p.rectangles(840, 250)
	if err != nil {
		return nil, err
	}
	rect(20, 96, 860, 262, 0xffffff)
	for i, r := range rects {
		bg, fg := uint32(0xdce5f2), uint32(0x214f90)
		if i == p.selected {
			bg, fg = 0x0066cc, 0xffffff
		}
		rect(int(30+r.x), int(100+r.y), int(r.w), int(r.h), bg)
		if r.w >= 24 && r.h >= 24 {
			text(int(34+r.x), int(117+r.y), 20, 13, true, fg, fmt.Sprint(i+1))
		}
		if starterPlaceholder(p.layout.Displays[i]) && r.w >= 110 && r.h >= 44 {
			text(int(34+r.x), int(134+r.y), int(r.w-14), 11, false, fg, "placeholder, not a device")
		}
	}
	d := p.display()
	r := rects[p.selected]
	mx, my := r.x, r.y
	den := max(int64(1), p.extent()-1)
	switch p.edge {
	case desk.Left:
		my += p.along * (r.h - 1) / den
	case desk.Right:
		mx += r.w - 1
		my += p.along * (r.h - 1) / den
	case desk.Top:
		mx += p.along * (r.w - 1) / den
	case desk.Bottom:
		mx += p.along * (r.w - 1) / den
		my += r.h - 1
	}
	rect(int(28+mx), int(98+my), 5, 5, 0xffd166)
	candidate, ok, err := p.candidate()
	if err != nil {
		return nil, err
	}
	text(20, 380, 860, 13, true, 0x214f90, fmt.Sprintf("Display %d of %d: %s", p.selected+1, len(p.layout.Displays), d.ID))
	deviceLine := "Device label: " + d.Device
	if starterPlaceholder(d) {
		deviceLine += " (placeholder, not a verified device)"
	}
	text(20, 402, 860, 13, false, 0x526780, deviceLine)
	if details {
		lines := []string{
			fmt.Sprintf("Layout origin (%d,%d), size %dx%d; native pixels %dx%d", d.X, d.Y, d.Width, d.Height, d.PixelWidth, d.PixelHeight),
			fmt.Sprintf("Query edge=%s, source pixel=%d of 0..%d", p.edge, p.along, p.extent()-1),
		}
		if ok {
			lines = append(lines, "Candidate display: "+candidate.Display, "Candidate device: "+candidate.Device, fmt.Sprintf("Native candidate pixel: (%d,%d)", candidate.X, candidate.Y))
		} else {
			lines = append(lines, "No neighbor at this exact source pixel.", "Exact original geometry is used, not the rounded diagram.")
		}
		lines = append(lines, "Yellow marker is a geometry query, never live input.")
		for i, line := range lines {
			text(20, 424+i*20, 860, 12, false, 0x526780, line)
		}
	} else {
		if ok {
			text(20, 438, 860, 14, true, 0x172b4d, "This edge reaches: "+candidate.Display)
			text(20, 464, 860, 14, false, 0x526780, fmt.Sprintf("The selected pixel would map to (%d, %d) on that display.", candidate.X, candidate.Y))
		} else {
			text(20, 438, 860, 18, true, 0x172b4d, "No neighboring display at this point")
			text(20, 464, 860, 14, false, 0x526780, "Choose another edge or move the yellow marker to explore the arrangement.")
		}
		text(20, 508, 860, 14, false, 0x63758c, "This is a read-only file preview. It does not pair or enable devices.")
	}
	text(20, 566, 860, 12, false, 0x63758c, "Arrows move the marker. Page Up/Down chooses a display. Tab selects controls.")
	labels := []string{"Previous", "Next", "Left", "Right", "Top", "Bottom", "- pixel", "+ pixel", "Close"}
	for i, label := range labels {
		x := 20 + i*96
		bg, fg := uint32(0xffffff), uint32(0x526780)
		if i == focus || i >= 2 && i <= 5 && desk.Edge([]string{"left", "right", "top", "bottom"}[i-2]) == p.edge {
			bg = 0x0066cc
			fg = 0xffffff
		}
		rect(x, 580, 92, 44, bg)
		text(x+8, 607, 76, 13, true, fg, label)
		if i == focus {
			rect(x+3, 583, 86, 1, 0xffffff)
			rect(x+3, 620, 86, 1, 0xffffff)
		}
	}
	rect(730, 64, 150, 28, 0xffffff)
	label := "Layout details"
	if details {
		label = "Hide details"
	}
	text(741, 83, 128, 13, true, 0x526780, label)
	if focus == 9 {
		rect(733, 67, 144, 1, 0x2563c7)
		rect(733, 88, 144, 1, 0x2563c7)
	}
	text(20, 643, 860, 12, false, 0x63758c, "Nothing is written to the layout file. Close returns to the previous screen.")
	if renderErr != nil {
		return nil, renderErr
	}
	return picture, nil
}
