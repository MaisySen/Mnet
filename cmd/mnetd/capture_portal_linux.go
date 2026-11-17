//go:build linux

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"fintex.au/mnet/internal/portal"
	"fintex.au/mnet/internal/session"
)

// Portal backend: capture through the XDG ScreenCast portal and a
// gst-launch PipeWire pump (package internal/portal). This is the
// GNOME path: compositors without wlr-screencopy refuse grim, and
// the portal is their only sanctioned capture API. The user clicks
// through the compositor's share dialog once per daemon start.
// Input injection stays with ydotool: the portal's RemoteDesktop
// interface needs a second consent flow and libei, which ydotool
// already covers at evdev level on every compositor.
//
// Live verification on a real GNOME session is pending; every piece
// below the D-Bus socket is unit tested.

type portalCapturer struct {
	stream    *portal.Stream
	pwfd      *os.File
	pump      *portal.Pump
	raw       []byte
	last      []byte
	lastRGBA  *image.RGBA
	jpegQ     atomic.Int32
	closeOnce sync.Once
}

// newPortalCapturerFor negotiates a portal stream, persisting the
// compositor's restore token in stateDir (empty: no persistence) so
// later daemon starts skip the consent dialog while the grant lasts.
func newPortalCapturerFor(stateDir string) (session.Capturer, error) {
	bus, err := portal.DialSession()
	if err != nil {
		return nil, err
	}
	client := portal.NewClient(bus)
	client.RestoreToken = loadPortalToken(stateDir)
	stream, err := client.OpenMonitor()
	if err != nil {
		_ = bus.Close()
		return nil, fmt.Errorf("portal: %w", err)
	}
	if stream.RestoreToken != "" {
		savePortalToken(stateDir, stream.RestoreToken)
	}
	pwfd := os.NewFile(uintptr(stream.FD), "mnet-pipewire")
	if pwfd == nil {
		_ = stream.Close()
		return nil, fmt.Errorf("portal: bad PipeWire fd")
	}
	if stream.Width <= 0 || stream.Height <= 0 || stream.Width > 16384 || stream.Height > 16384 {
		_ = pwfd.Close()
		_ = stream.Close()
		return nil, fmt.Errorf("portal: compositor reported no stream size; cannot pace raw frames")
	}
	scale := capScale(stream.Width, stream.Height)
	w, h := stream.Width/scale, stream.Height/scale
	pump, err := portal.StartPump(stream, pwfd, w, h)
	if err != nil {
		_ = pwfd.Close()
		_ = stream.Close()
		return nil, err
	}
	c := &portalCapturer{stream: stream, pwfd: pwfd, pump: pump}
	c.jpegQ.Store(70)
	return c, nil
}

// portalTokenPath is where the compositor's persist token lives. The
// token lets the holder capture the screen without the consent
// dialog, so it is 0600 in the (0700) state directory like the
// node's keys.
func portalTokenPath(stateDir string) string {
	return filepath.Join(stateDir, "portal-restore-token")
}

func loadPortalToken(stateDir string) string {
	if stateDir == "" {
		return ""
	}
	b, err := os.ReadFile(portalTokenPath(stateDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func savePortalToken(stateDir, token string) {
	if stateDir == "" || token == "" {
		return
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		log.Printf("mnetd: portal token: %v", err)
		return
	}
	f, err := os.CreateTemp(stateDir, ".portal-token-*")
	if err != nil {
		log.Printf("mnetd: portal token: %v", err)
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.WriteString(token + "\n")
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, portalTokenPath(stateDir))
	}
	if err != nil {
		log.Printf("mnetd: portal token: %v", err)
	}
}

// SetQuality adapts the JPEG encode quality to path conditions.
func (c *portalCapturer) SetQuality(q int) {
	if q < 20 {
		q = 20
	}
	if q > 90 {
		q = 90
	}
	c.jpegQ.Store(int32(q))
}

func (c *portalCapturer) Geometry() (int, int) { return c.pump.Width, c.pump.Height }

// bgraToRGBA converts one raw BGRA pump frame (the gst caps fix the
// size, so stride is width*4) into an RGBA image for the encoder.
func bgraToRGBA(raw []byte, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	n := w * h
	for i := 0; i < n; i++ {
		s := i * 4
		img.Pix[s+0] = raw[s+2] // R
		img.Pix[s+1] = raw[s+1] // G
		img.Pix[s+2] = raw[s+0] // B
		img.Pix[s+3] = 255
	}
	return img
}

func (c *portalCapturer) Capture() ([]byte, bool, error) {
	raw, err := c.pump.ReadFrame(c.raw)
	if err != nil {
		return nil, false, err
	}
	c.raw = raw
	img := bgraToRGBA(raw, c.pump.Width, c.pump.Height)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: int(c.jpegQ.Load())}); err != nil {
		return nil, false, err
	}
	changed := !bytes.Equal(buf.Bytes(), c.last)
	c.last = buf.Bytes()
	c.lastRGBA = img
	return c.last, changed, nil
}

// CaptureDelta gives portal sessions the same dirty-rect savings as
// X11: only changed 64x64 tiles cross the wire while a minority of
// the screen moved; past 60% changed tiles a full frame is cheaper.
func (c *portalCapturer) CaptureDelta() ([]byte, []session.Tile, error) {
	raw, err := c.pump.ReadFrame(c.raw)
	if err != nil {
		return nil, nil, err
	}
	c.raw = raw
	img := bgraToRGBA(raw, c.pump.Width, c.pump.Height)
	// A new canvas cannot be sent as deltas against the old dimensions.
	if c.lastRGBA != nil && c.lastRGBA.Bounds() != img.Bounds() {
		c.lastRGBA = nil
	}
	rects := session.DiffTiles(c.lastRGBA, img, 64)
	b := img.Bounds()
	total := ((b.Dx() + 63) / 64) * ((b.Dy() + 63) / 64)
	if c.lastRGBA != nil && len(rects) == 0 {
		return nil, nil, nil
	}
	if c.lastRGBA != nil && len(rects) <= total*3/5 {
		tiles, err := session.EncodeTiles(img, rects, int(c.jpegQ.Load()))
		if err != nil {
			return nil, nil, err
		}
		c.lastRGBA = img
		return nil, tiles, nil
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: int(c.jpegQ.Load())}); err != nil {
		return nil, nil, err
	}
	c.lastRGBA = img
	c.last = buf.Bytes()
	return buf.Bytes(), nil, nil
}

// Close interrupts a blocked frame read before the host waits for its
// screen goroutine. Each session owns its helper, descriptor and portal.
func (c *portalCapturer) Close() error {
	c.closeOnce.Do(func() {
		if c.pump != nil {
			_ = c.pump.Close()
		}
		if c.pwfd != nil {
			_ = c.pwfd.Close()
		}
		if c.stream != nil {
			_ = c.stream.Close()
		}
	})
	return nil
}
