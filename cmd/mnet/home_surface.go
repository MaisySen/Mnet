package main

import (
	"errors"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// renderHomeSurface is presentation only. Existing route selection, local
// capability checks and child ownership do not enter this pixel renderer.
// The info panel is a local explanation of the separate consent each tool
// requires. It grants nothing and changes no route.
func renderHomeSurface(notice string, hasLayout, hasMulties bool, focus int, info bool) (*image.RGBA, error) {
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
			renderErr = errors.New("home text exceeds presentation bounds")
			return
		}
		d := font.Drawer{Dst: picture, Src: image.NewUniform(rgb(c)), Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	rect(0, 0, 900, 560, 0xf4f6fa)
	rect(0, 0, 900, 72, 0xffffff)
	rect(0, 71, 900, 1, 0xe4e8f0)
	rect(24, 18, 38, 38, 0x103e89)
	text(31, 46, 27, 28, true, 0xffffff, "M")
	text(74, 43, 300, 24, true, 0x172b4d, "Your devices, together")
	text(742, 41, 136, 12, true, 0x63758c, "LOCAL PREVIEW")
	// The consent-map explanation is always available and never a route.
	infoBG, infoFG := uint32(0xffffff), uint32(0x214f90)
	rect(520, 18, 200, 40, infoBG)
	text(542, 44, 160, 15, true, infoFG, "How this works")
	if focus == 5 {
		rect(523, 21, 194, 1, 0x2563c7)
		rect(523, 54, 194, 1, 0x2563c7)
	}
	if info {
		rect(20, 84, 860, 384, 0xffffff)
		text(38, 116, 800, 18, true, 0x172b4d, "How these tools work")
		blocks := [][2]string{
			{"Mnet networking", "See which devices are online. Being connected never shares a screen or any input."},
			{"Remote Desktop", "Viewing needs the host's approval every time. Pointer control is a separate question the host answers later. No keyboard, files or recording."},
			{"Shared Desk", "One keyboard and mouse across your displays needs exact pairing and a fresh approval each run. It is not connected in this preview."},
			{"Multies setup", "Sign-in and device setup are not connected here. When configured, Review Multies approvals only reviews access."},
		}
		for i, block := range blocks {
			y := 152 + i*66
			text(38, y, 800, 15, true, 0x124b9a, block[0])
			for j, line := range wrapHomeInfo(block[1], 96) {
				text(38, y+22+j*18, 800, 13, false, 0x526780, line)
			}
		}
		text(38, 448, 800, 12, false, 0x63758c, "Each tool asks on its own. No tool inherits another tool's approval.")
		if renderErr != nil {
			return nil, renderErr
		}
		return picture, nil
	}
	intro := "Three separate tools. Multies setup isn't configured for this home."
	if hasMulties {
		intro = "Three separate tools. You can review Multies approvals. Network setup is not connected."
	}
	text(24, 94, 850, 14, false, 0x63758c, intro)
	cards := [][3]string{
		{"Mnet networking", "Your private network", "See a read-only snapshot of local network devices."},
		{"Remote Desktop", "A screen when you need it", "Choose a device. The host approves viewing and any pointer control separately."},
		{"Shared Desk", "One desk, across your displays", "Create, inspect and edit offline arrangement drafts. Pairing and input stay unavailable."},
	}
	for i, card := range cards {
		y := 110 + i*112
		enabled := true
		heading := uint32(0x172b4d)
		accent := uint32(0x124b9a)
		bg := uint32(0xffffff)
		if !enabled {
			bg = 0xecf0f6
			heading = 0x63758c
			accent = 0x63758c
		}
		rect(20, y, 860, 104, bg)
		text(38, y+29, 735, 21, true, heading, card[0])
		text(38, y+54, 735, 15, true, accent, card[1])
		text(38, y+78, 790, 14, false, 0x63758c, card[2])
		if i == 2 && !hasLayout {
			text(38, y+97, 790, 12, false, 0x63758c, "Drafts stay local files. Saving always creates a new file; nothing is replaced.")
		}
		if enabled {
			text(832, y+54, 27, 24, false, 0x2563c7, ">")
		}
		if focus == i && enabled {
			rect(21, y+1, 4, 102, 0x2563c7)
		}
	}
	text(24, 468, 850, 14, false, 0x526780, notice)
	setupLabel := "Multies not configured"
	setupBG, setupFG := uint32(0xecf0f6), uint32(0x758299)
	if hasMulties {
		setupLabel = "Review Multies approvals"
		setupBG = 0xffffff
		setupFG = 0x214f90
	}
	rect(20, 486, 640, 44, setupBG)
	text(38, 514, 600, 16, true, setupFG, setupLabel)
	if focus == 4 && hasMulties {
		rect(20, 527, 640, 3, 0x2563c7)
	}
	rect(680, 486, 200, 44, 0xffffff)
	text(710, 514, 150, 16, true, 0x526780, "Close")
	if focus == 3 {
		rect(680, 527, 200, 3, 0x2563c7)
	}
	text(24, 549, 850, 12, false, 0x63758c, "Tab to choose. Enter to open. Escape to close. Closing a tool brings you back here.")
	if renderErr != nil {
		return nil, renderErr
	}
	return picture, nil
}

// wrapHomeInfo splits an explanation line at word boundaries, bounded so the
// presentation check still rejects anything unexpected.
func wrapHomeInfo(s string, limit int) []string {
	var lines []string
	for len(s) > limit {
		cut := limit
		for cut > 0 && s[cut-1] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = limit
		}
		lines = append(lines, s[:cut])
		s = s[cut:]
		for len(s) > 0 && s[0] == ' ' {
			s = s[1:]
		}
	}
	return append(lines, s)
}
