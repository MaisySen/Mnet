//go:build linux

package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
)

func TestDesktopViewFitAndLabels(t *testing.T) {
	for _, tc := range []struct{ sw, sh, dw, dh, w, h int }{{2000, 1000, 800, 600, 800, 400}, {1000, 2000, 800, 600, 300, 600}, {0, 10, 100, 100, 0, 0}, {8192, 1, 160, 120, 160, 1}} {
		w, h := fitView(tc.sw, tc.sh, tc.dw, tc.dh)
		if w != tc.w || h != tc.h {
			t.Fatalf("fit %+v got %d,%d", tc, w, h)
		}
	}
	if got := safeViewerLabel("peer\n\x1b\u202e"); got != "peer???" {
		t.Fatal(got)
	}
}
func TestDesktopViewMalformedImageRefusesBeforeX(t *testing.T) {
	v := &xDesktopView{}
	if err := v.Frame(session.ScreenFrame{Image: []byte("not a JPEG")}); err == nil {
		t.Fatal("invalid image accepted")
	}
}

// Invoked only on an explicitly disposable viewer display. The other end is
// a real session protocol host, not a frame callback bypass.
func TestDesktopViewX11ProtocolPixelsAndClose(t *testing.T) {
	if os.Getenv("MNET_VIEWER_X11_TEST") != "1" {
		t.Skip("requires disposable X11 display")
	}
	dir := os.Getenv("MNET_VIEWER_ARTIFACTS")
	if dir == "" {
		t.Fatal("artifact directory required")
	}
	display, err := newDesktopView("approved-fixture-host")
	if err != nil {
		t.Fatal(err)
	}
	v := display.(*xDesktopView)
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			c := color.RGBA{R: 220, A: 255}
			if x >= 320 {
				c = color.RGBA{G: 190, A: 255}
			}
			if y >= 240 {
				c.B = 230
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	right, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	hostC := make(chan *session.Mux, 1)
	go func() {
		m, e := session.Listen(ln, "approved-fixture-host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		if e != nil {
			hostC <- nil
			return
		}
		hostC <- m
	}()
	viewer, err := session.Dial(right, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	host := <-hostC
	if host == nil {
		t.Fatal("host hello failed")
	}
	defer host.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runDesktopView(ctx, v, func(context.Context) (*session.Mux, error) { return viewer, nil })
	}()
	h := session.ServeHost(host, &viewerFixtureCapture{data: buf.Bytes()}, nil, 8)
	defer h.Stop()
	if err = os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("driver did not close viewer")
	}
	select {
	case <-viewer.Closed():
	default:
		t.Fatal("window close did not end transport")
	}
	t.Logf("final viewer geometry %dx%d", v.width, v.height)
	if !v.closed {
		t.Fatal("X resources not retired")
	}
}

type viewerFixtureCapture struct{ data []byte }

func (c *viewerFixtureCapture) Geometry() (int, int)           { return 640, 480 }
func (c *viewerFixtureCapture) Capture() ([]byte, bool, error) { return c.data, true, nil }

func TestDesktopViewUnsupportedDisplayRefuses(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "fixture-wayland")
	if view, err := newDesktopView("peer"); err == nil {
		view.Close()
		t.Fatal("unsupported Wayland display fell back")
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XDG_SESSION_TYPE", "")
	t.Setenv("DISPLAY", "")
	if view, err := newDesktopView("peer"); err == nil {
		view.Close()
		t.Fatal("missing local display accepted")
	}
}
func TestDesktopViewX11UnsupportedDepth(t *testing.T) {
	if os.Getenv("MNET_VIEWER_UNSUPPORTED_DEPTH") != "1" {
		t.Skip("requires disposable16bit X display")
	}
	if view, err := newDesktopView("peer"); err == nil {
		view.Close()
		t.Fatal("unsupported native depth accepted")
	}
}
