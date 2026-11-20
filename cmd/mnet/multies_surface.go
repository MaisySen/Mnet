package main

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strconv"

	"fintex.au/mnet/internal/multies"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type multiesSurface struct {
	phase, key, origin string
	list               multies.ApprovedDeviceList
	page, focus        int
	enabled            [4]bool
}

// Shared pixel renderer with embedded licensed Go fonts. No host font lookup,
// browser engine, network read or account operation can enter this renderer.
func renderMultiesSurface(v multiesSurface) (*image.RGBA, error) {
	if len(v.origin) > 75 || len(v.key) > 43 || v.page < 0 || v.page*4 > len(v.list.Devices) {
		return nil, errors.New("invalid sign in presentation")
	}
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
	face := func(size int, heavy bool) font.Face {
		key := size
		if heavy {
			key += 100
		}
		if f := faces[key]; f != nil {
			return f
		}
		ttf := regular
		if heavy {
			ttf = bold
		}
		f, e := opentype.NewFace(ttf, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
		if e != nil {
			return nil
		}
		faces[key] = f
		return f
	}
	rgb := func(h uint32) color.RGBA { return color.RGBA{uint8(h >> 16), uint8(h >> 8), uint8(h), 255} }
	im := image.NewRGBA(image.Rect(0, 0, 900, 500))
	rect := func(x, y, w, h int, c uint32) {
		draw.Draw(im, image.Rect(x, y, x+w, y+h), image.NewUniform(rgb(c)), image.Point{}, draw.Src)
	}
	var renderErr error
	text := func(x, y, width, size int, heavy bool, c uint32, s string) {
		if renderErr != nil {
			return
		}
		f := face(size, heavy)
		if f == nil || font.MeasureString(f, s).Ceil() > width {
			renderErr = errors.New("sign in text exceeds presentation bounds")
			return
		}
		d := font.Drawer{Dst: im, Src: image.NewUniform(rgb(c)), Face: f, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	// Brand bar, one selected tool and two separate, unavailable setup routes.
	rect(0, 0, 900, 500, 0xf4f6fa)
	rect(0, 0, 900, 72, 0xffffff)
	rect(0, 71, 900, 1, 0xe4e8f0)
	rect(24, 18, 38, 38, 0x103e89)
	text(31, 46, 27, 28, true, 0xffffff, "M")
	text(74, 43, 180, 24, true, 0x172b4d, "Mnet")
	text(742, 41, 136, 12, true, 0x63758c, "LOCAL PREVIEW")
	text(26, 98, 150, 14, true, 0x124b9a, "Mnet networking")
	rect(25, 107, 134, 3, 0x2563c7)
	text(204, 98, 210, 14, false, 0x758299, "Remote Desktop")
	text(392, 98, 210, 14, false, 0x758299, "Shared Desk")
	// Main content stays plain and short. Detailed grants remain enforced, not
	// expanded into the primary screen's paragraph copy.
	rect(20, 126, 860, 266, 0xffffff)
	heading, subtitle := "Your devices, together", "Sign in with Multies to see the devices you approve."
	switch v.phase {
	case "preparing":
		heading = "Getting this device ready"
		subtitle = "Preparing a private key on this computer."
	case "review":
		heading = "Check this device"
		subtitle = "You'll compare this key with the one shown in Multies."
	case "waiting":
		heading = "Continue in your browser"
		subtitle = "Sign in to Multies, choose a workspace and approve this device."
	case "done":
		heading = "Your approved devices"
		subtitle = "Current approvals for your account. Not a connection status."
	case "stale":
		heading = "Ready for a fresh look?"
		subtitle = "This snapshot expired. Check again to see current approvals."
	case "failed":
		heading = "That sign in didn't finish"
		subtitle = "Nothing was saved. You can start again when you're ready."
	case "start":
	default:
		return nil, errors.New("unknown sign in presentation")
	}
	text(44, 170, 800, 27, true, 0x172b4d, heading)
	text(44, 200, 800, 16, false, 0x63758c, subtitle)
	switch v.phase {
	case "start":
		for i, step := range []struct{ title, detail string }{{"1  This device", "Prepare your local key"}, {"2  Multies sign in", "You choose what to approve"}, {"3  Your devices", "See your current approvals"}} {
			x := 44 + i*275
			rect(x, 234, 256, 104, 0xf4f7fc)
			text(x+16, 266, 225, 16, true, 0x214f90, step.title)
			text(x+16, 296, 225, 14, false, 0x63758c, step.detail)
		}
		text(44, 367, 790, 14, false, 0x63758c, "Start on this computer. You'll review the key before a browser opens.")
	case "preparing":
		rect(44, 238, 790, 4, 0xe9edf5)
		rect(44, 238, 180, 4, 0x2563c7)
		text(44, 287, 790, 16, false, 0x63758c, "Your browser has not opened yet.")
	case "review", "waiting":
		rect(44, 225, 812, 79, 0xf2f6fc)
		text(60, 250, 770, 12, true, 0x63758c, "THIS DEVICE KEY")
		text(60, 281, 770, 17, true, 0x214f90, v.key)
		if v.phase == "review" {
			text(44, 338, 800, 15, false, 0x63758c, "Only approve if you started this request and both keys match.")
			text(44, 365, 800, 15, false, 0x63758c, "Open your browser when you're ready.")
		} else {
			text(44, 340, 800, 15, false, 0x63758c, "This window will update after you decide. Closing it cancels this attempt.")
		}
	case "done":
		start, end := v.page*4, min(v.page*4+4, len(v.list.Devices))
		if start == end {
			text(44, 261, 790, 17, false, 0x63758c, "No active approvals. Start sign in to approve this computer.")
		}
		for i, d := range v.list.Devices[start:end] {
			y := 220 + i*39
			rect(44, y, 812, 34, 0xf4f7fc)
			label := deviceText(d.Platform, 8)
			if d.DeviceKey == v.key {
				label = "This computer"
			}
			text(56, y+23, 105, 14, true, 0x214f90, label)
			text(170, y+23, 630, 14, false, 0x526780, d.DeviceKey)
		}
		note := "Checked just now. This list clears after 30 seconds."
		if len(v.list.Devices) > 4 {
			note = "Page " + strconv.Itoa(v.page+1) + " of " + strconv.Itoa((len(v.list.Devices)+3)/4) + ". Snapshot clears after 30 seconds."
		}
		if v.list.Truncated {
			note += " More records exist."
		}
		text(44, 385, 800, 12, false, 0x63758c, note)
	case "stale", "failed":
		rect(44, 231, 812, 86, 0xf4f7fc)
		text(62, 267, 770, 17, true, 0x526780, "Your previous device list has been cleared.")
		text(62, 296, 770, 15, false, 0x63758c, "Start again for a new check. No background requests.")
	}
	// Exact configured destination is always present, never cropped or hidden.
	text(24, 414, 850, 12, false, 0x63758c, "Multies server: "+v.origin)
	text(24, 435, 850, 12, false, 0x63758c, "Device list only. Network connection isn't enabled in this preview.")
	labels := [4]string{"Start sign in", "Previous", "Next", "Close"}
	if v.phase == "review" {
		labels[0] = "Open browser"
	}
	if v.phase == "done" || v.phase == "stale" || v.phase == "failed" {
		labels[0] = "Check again"
	}
	for i, x := range []int{20, 240, 460, 680} {
		if !v.enabled[i] && i != 0 {
			continue
		}
		bg, fg := uint32(0xffffff), uint32(0x526780)
		if i == 0 {
			bg, fg = 0x2563c7, 0xffffff
		}
		if !v.enabled[i] {
			bg, fg = 0xe7ecf5, 0x7b8ba1
		}
		rect(x, 448, 200, 44, bg)
		if v.focus == i && v.enabled[i] {
			rect(x+3, 451, 194, 2, 0xf0a849)
			rect(x+3, 487, 194, 2, 0xf0a849)
		}
		text(x+20, 476, 165, 16, true, fg, labels[i])
	}
	if renderErr != nil {
		return nil, renderErr
	}
	return im, nil
}
