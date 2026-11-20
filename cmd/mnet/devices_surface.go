package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"

	"fintex.au/mnet/internal/daemon"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Read-only presentation. No input here selects a device or grants a session.
func renderDevicesSurface(st *daemon.Status, page int, sampled time.Time, message string, enabled [4]bool, focus int) (*image.RGBA, int, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, 0, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, 0, err
	}
	faces := map[int]font.Face{}
	defer func() {
		for _, f := range faces {
			_ = f.Close()
		}
	}()
	im := image.NewRGBA(image.Rect(0, 0, 900, 500))
	rgb := func(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }
	rect := func(x, y, w, h int, c uint32) {
		draw.Draw(im, image.Rect(x, y, x+w, y+h), image.NewUniform(rgb(c)), image.Point{}, draw.Src)
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
		f := faces[key]
		if f == nil {
			ttf := regular
			if heavy {
				ttf = bold
			}
			f, renderErr = opentype.NewFace(ttf, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
			if renderErr != nil {
				return
			}
			faces[key] = f
		}
		if font.MeasureString(f, s).Ceil() > width {
			renderErr = errors.New("device text exceeds presentation bounds")
			return
		}
		d := font.Drawer{Dst: im, Src: image.NewUniform(rgb(c)), Face: f, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	rect(0, 0, 900, 500, 0xf4f6fa)
	rect(0, 0, 900, 72, 0xffffff)
	rect(24, 18, 38, 38, 0x103e89)
	text(31, 46, 27, 28, true, 0xffffff, "M")
	text(74, 43, 600, 24, true, 0x172b4d, "Mnet networking")
	text(742, 41, 136, 12, true, 0x63758c, "LOCAL SNAPSHOT")
	_, page = deviceSnapshotLines(st, page, sampled)
	text(24, 96, 850, 14, false, 0x63758c, "Read-only local status. Device names are labels, not verified Multies identities.")
	rect(20, 110, 860, 278, 0xffffff)
	if st == nil || st.Status != "up" {
		heading, detail := "No current snapshot", "Refresh reads this computer's daemon. No connection changes are made."
		if st != nil {
			heading = "This computer is not connected"
			detail = "Multies setup isn't connected here. No peer rows are shown."
			if st.Status == "waiting-approval" {
				heading = "This computer needs approval"
				detail = "Use private mnet status output for approval. No approval link is shown here."
			}
		}
		text(40, 150, 820, 22, true, 0x172b4d, heading)
		text(40, 182, 820, 14, false, 0x63758c, detail)
		if st != nil {
			text(40, 220, 820, 14, false, 0x526780, "This device: "+deviceText(st.NodeName, 32))
			text(40, 248, 820, 14, false, 0x526780, "Daemon: "+deviceText(st.Status, 24))
		}
	} else {
		text(40, 138, 820, 16, true, 0x172b4d, deviceText(st.NodeName, 32)+"  |  "+deviceText(st.IPv4, 20))
		text(40, 162, 820, 12, false, 0x63758c, "Up is local readiness, not traffic proof. Connection status below is reported telemetry.")
		for i, h := range []string{"Device label", "Private IP", "Reported", "Path", "Handshake"} {
			text([]int{40, 290, 470, 580, 680}[i], 188, []int{235, 165, 100, 90, 170}[i], 12, true, 0x63758c, h)
		}
		start, end := page*devicePageSize, min((page+1)*devicePageSize, len(st.Peers))
		if start == end {
			text(40, 230, 820, 16, false, 0x63758c, "No peers reported in this snapshot.")
		}
		for i, p := range st.Peers[start:end] {
			y := 200 + i*22
			rect(32, y, 836, 21, 0xf4f7fc)
			path := "unknown"
			if p.Endpoint != "" {
				path = "direct"
				if p.Relay {
					path = "relay"
				}
			}
			handshake := "unavailable"
			switch p.HandshakeState {
			case "never":
				handshake = "never"
			case "clock-skew":
				handshake = "clock skew"
			case "recent", "stale", "":
				if p.LastHandshake >= 0 {
					handshake = fmt.Sprintf("%ds ago", p.LastHandshake)
					if p.LastHandshake > 999999 {
						handshake = "999999+s ago"
					}
				}
			}
			text(40, y+16, 235, 13, true, 0x214f90, deviceText(p.Name, 18))
			text(290, y+16, 165, 13, false, 0x526780, deviceText(p.IPv4, 20))
			text(470, y+16, 100, 13, false, 0x526780, devicePeerState(p))
			text(580, y+16, 90, 13, false, 0x526780, path)
			text(680, y+16, 170, 13, false, 0x526780, handshake)
		}
	}
	if st != nil && st.Status == "up" {
		text(40, 384, 820, 11, false, 0x63758c, fmt.Sprintf("Page %d of %d. Read started %s. Device labels may be shortened.", page+1, max(1, (len(st.Peers)+devicePageSize-1)/devicePageSize), sampled.Format("15:04:05 MST")))
	}
	text(24, 410, 850, 12, false, 0x526780, message)
	text(24, 432, 850, 12, false, 0x63758c, "Refresh is read only. Snapshot clears after 30 seconds. No background polling.")
	for i, x := range []int{20, 240, 460, 680} {
		bg, fg := uint32(0xffffff), uint32(0x526780)
		if i == 0 {
			bg = 0x2563c7
			fg = 0xffffff
		}
		if !enabled[i] {
			bg = 0xe8edf4
			fg = 0x758299
		}
		rect(x, 448, 200, 44, bg)
		text(x+16, 477, 170, 16, true, fg, []string{"Refresh", "Previous", "Next", "Close"}[i])
		if enabled[i] && focus == i {
			rect(x, 489, 200, 3, 0x103e89)
		}
	}
	if renderErr != nil {
		return nil, page, renderErr
	}
	return im, page, nil
}
