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

// Waiting pixels contain no received screen or authority. The owner alone
// decides when admission/first-frame state changes and when the attempt ends.
func renderViewerWaiting(width int, waitingHost bool) (*image.RGBA, error) {
	if width < 288 || width > 520 {
		return nil, errors.New("waiting panel size unsupported")
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
	picture := image.NewRGBA(image.Rect(0, 0, width, 120))
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
			renderErr = errors.New("waiting text exceeds presentation bounds")
			return
		}
		d := font.Drawer{Dst: picture, Src: image.NewUniform(rgb(c)), Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	rect(0, 0, width, 120, 0x1c2a3b)
	heading, detail := "Waiting for approval", "The host decides what to share."
	if !waitingHost {
		heading = "Waiting for screen"
		detail = "No screen in 15 seconds ends this session."
	}
	text(20, 36, width-40, 20, true, 0xffffff, heading)
	text(20, 66, width-40, 12, false, 0xb6c9db, detail)
	text(20, 93, width-40, 12, false, 0xb6c9db, "End or Escape leaves this session.")
	if renderErr != nil {
		return nil, renderErr
	}
	return picture, nil
}
