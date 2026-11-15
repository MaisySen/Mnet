//go:build linux

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os/exec"
	"strconv"

	"fintex.au/mnet/internal/session"
)

// Wayland backend: grim captures the full desktop (wlroots
// compositors: Sway, Hyprland, Wayfire, River, KDE with the wlr
// protocol); ydotool injects input at the evdev level, which works on
// every Wayland compositor (needs the ydotoold socket). GNOME and
// others without the wlr-screencopy protocol need the portal backend,
// which is later work.

type waylandCapturer struct {
	w, h int
	last []byte
}

func newWaylandCapturer() (session.Capturer, error) {
	w, h, err := grimSize()
	if err != nil {
		return nil, err
	}
	return &waylandCapturer{w: w, h: h}, nil
}

// grimSize learns the desktop size from one grim capture's header.
func grimSize() (int, int, error) {
	out, err := exec.Command("grim", "-t", "png", "-").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("grim: %w", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		return 0, 0, fmt.Errorf("grim output: %w", err)
	}
	return cfg.Width, cfg.Height, nil
}

func (c *waylandCapturer) Geometry() (int, int) { return c.w, c.h }

func (c *waylandCapturer) Capture() ([]byte, bool, error) {
	out, err := exec.Command("grim", "-t", "png", "-").Output()
	if err != nil {
		return nil, false, fmt.Errorf("grim: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, false, err
	}
	// Cap the long edge like the X11 backend.
	long := c.w
	if c.h > long {
		long = c.h
	}
	scale := 1
	for long/scale > 1280 {
		scale++
	}
	var enc image.Image = img
	if scale > 1 {
		if rgba, ok := img.(*image.RGBA); ok {
			enc = downscale(rgba, scale)
		} else {
			enc = toRGBADownscale(img, scale)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, enc, &jpeg.Options{Quality: 70}); err != nil {
		return nil, false, err
	}
	changed := !bytes.Equal(buf.Bytes(), c.last)
	c.last = buf.Bytes()
	return buf.Bytes(), changed, nil
}

// toRGBADownscale handles non-RGBA sources by nearest-neighbor sampling.
func toRGBADownscale(src image.Image, factor int) *image.RGBA {
	b := src.Bounds()
	w := b.Dx() / factor
	h := b.Dy() / factor
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.Set(x, y, src.At(b.Min.X+x*factor, b.Min.Y+y*factor))
		}
	}
	return out
}

// waylandInjector drives ydotool. Coordinates arrive in capture-frame
// pixels (downscaled); ydotool talks absolute device pixels, so scale
// back up. Note ydotool's absolute moves use a virtual input device:
// compositors map it to the focused output, matching grim's capture.
type waylandInjector struct {
	scaleX, scaleY float64
	textMode       string
}

func newWaylandInjector() session.Injector {
	w, h, err := grimSize()
	if err != nil || w == 0 {
		return &waylandInjector{scaleX: 1, scaleY: 1}
	}
	long := w
	if h > long {
		long = h
	}
	scale := 1
	for long/scale > 1280 {
		scale++
	}
	return &waylandInjector{scaleX: float64(scale), scaleY: float64(scale)}
}

func (i *waylandInjector) Inject(ev session.InputEvent) error {
	x := int(float64(ev.X) * i.scaleX)
	y := int(float64(ev.Y) * i.scaleY)
	switch ev.Kind {
	case "pointer":
		switch ev.Action {
		case "move":
			return run("ydotool", "mousemove", "--absolute", "-x", strconv.Itoa(x), "-y", strconv.Itoa(y))
		case "click", "down", "up":
			if err := run("ydotool", "mousemove", "--absolute", "-x", strconv.Itoa(x), "-y", strconv.Itoa(y)); err != nil {
				return err
			}
			btn := "0xC0"
			switch ev.Button {
			case "right":
				btn = "0xC1"
			case "middle":
				btn = "0xC2"
			}
			return run("ydotool", "click", btn)
		case "scroll":
			btn := "0xC5" // down
			if ev.DY < 0 {
				btn = "0xC4" // up
			}
			return run("ydotool", "click", btn)
		}
	case "key":
		if ev.Text != "" {
			return i.TypeText(ev.Text)
		}
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, out)
	}
	return nil
}
