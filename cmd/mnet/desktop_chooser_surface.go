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
// chooserSetupGuidance names the honest current state in the no-selection
// details panel. It never implies an enrollment, sign-in or connection flow
// this preview does not have.
func chooserSetupGuidance(message, phase string, peers int) (string, string) {
	switch {
	case message == errLocalDaemon.Error():
		return "Mnet is not running", "Start the local background service, then refresh. This preview only reads local evidence."
	case message == errLocalNetworkDown.Error():
		return "This computer is not connected", "Joining a private network is a separate explicit setup. This preview cannot enroll or connect it."
	case message == errLocalSetup.Error():
		return "Local setup is incomplete", "This preview reads the same local state directory as the background service."
	case message == errLocalAccess.Error():
		return "Local access was refused", "This preview can only read the local evidence it is allowed to read."
	case phase == "ready" && peers == 0:
		return "No devices yet", "Devices appear after they join your private network. Each host still approves viewing separately."
	default:
		return "Local setup details", "This preview does not connect Multies sign-in to network enrollment."
	}
}

func renderDesktopChooser(selection desktopPeerSelection, page, focus int, phase, message string, details bool, enabled []bool, now time.Time) (*image.RGBA, error) {
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
	text(20, 78, 860, 14, true, 0x526780, "Choose a device. Its host decides what to share.")
	current := selection.current(now)
	status := "Devices aren't available. Open Setup details, then refresh."
	if message == errLocalDaemon.Error() {
		status = "Mnet isn't running. Start it, then refresh."
	} else if message == errLocalNetworkDown.Error() {
		status = "This computer isn't connected. Multies setup isn't available in this preview."
	}
	switch phase {
	case "loading":
		status = "Checking your devices..."
	case "expired":
		status = "Device list expired. Refresh to check again."
	case "ready":
		if current {
			status = fmt.Sprintf("%d available device labels. Page %d of %d.", len(selection.snapshot.Peers), page+1, max(1, (len(selection.snapshot.Peers)+5)/6))
		}
	}
	text(20, 96, 860, 12, false, 0x63758c, status)
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
		text(20, 394, 690, 16, true, 0x214f90, "Selected: "+deviceText(p.Name, 30))
	} else {
		text(20, 394, 690, 16, true, 0x63758c, "Select a device to continue")
	}
	if details && phase != "loading" {
		rect(20, 404, 860, 76, 0xffffff)
		if selected {
			p := selection.snapshot.Peers[selection.selected]
			text(30, 423, 840, 13, true, 0x172b4d, "Exact device key: "+p.PublicKey)
			text(30, 442, 840, 12, false, 0x63758c, "Address: "+deviceText(p.IPv4, 40)+". Device labels are not verified Multies identities.")
		} else {
			title, line := chooserSetupGuidance(message, phase, len(selection.snapshot.Peers))
			text(30, 423, 840, 13, true, 0x172b4d, title)
			text(30, 442, 840, 12, false, 0x63758c, line)
		}
		text(30, 460, 840, 11, false, 0x63758c, message)
		text(30, 476, 840, 11, false, 0x63758c, "30-second snapshot. Manual reads only. Files, recording and keyboard are not offered.")
	} else {
		text(20, 427, 860, 14, false, 0x526780, "View only asks the host to share their screen.")
		text(20, 451, 860, 14, false, 0x526780, "Pointer view adds a separate pointer request, which the host must also accept.")
		text(20, 476, 860, 12, false, 0x63758c, "Names are device labels. No connection is made until you choose a viewing mode.")
	}
	// Deliberate disclosure is separate from mode authority. No key is shortened.
	bg, fg := uint32(0xffffff), uint32(0x526780)
	available := len(enabled) > 12 && enabled[12]
	if !available {
		bg, fg = 0xe8edf4, 0x758299
	}
	rect(730, 370, 150, 32, bg)
	label := "Setup details"
	if selected {
		label = "Device details"
	}
	if details && phase != "loading" {
		label = "Hide details"
	}
	text(741, 392, 128, 13, true, fg, label)
	if focus == 12 && available {
		rect(733, 373, 144, 1, 0x2563c7)
		rect(733, 398, 144, 1, 0x2563c7)
	}
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
