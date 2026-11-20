//go:build linux

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"

	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestDesktopViewX11FrameCacheOwnership(t *testing.T) {
	if os.Getenv("MNET_VIEWER_X11_TEST") != "1" {
		t.Skip("requires disposable X11")
	}
	display, err := newDesktopView("cache-fixture-only")
	if err != nil {
		t.Fatal(err)
	}
	v := display.(*xDesktopView)
	defer v.Close()
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	frame := image.NewRGBA(image.Rect(0, 0, 1280, 960))
	for y := 0; y < 960; y++ {
		for x := 0; x < 1280; x++ {
			frame.SetRGBA(x, y, color.RGBA{R: 100, G: 180, B: 200, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, frame, nil); err != nil {
		t.Fatal(err)
	}
	fr := session.ScreenFrame{Image: encoded.Bytes()}
	if err = v.Frame(fr); err != nil {
		t.Fatal(err)
	}
	first, imageOwner := v.imagePixmap, v.image
	if first == 0 {
		t.Fatal("no native image cache")
	}
	for i := 0; i < 12; i++ {
		if err = v.Frame(session.ScreenFrame{Image: append([]byte(nil), fr.Image...)}); err != nil {
			t.Fatal(err)
		}
		if v.imagePixmap != first || v.image != imageOwner {
			t.Fatal("identical completed frame decoded/reuploaded")
		}
	}
	if err = v.State("Local header repaint only. No authority change."); err != nil {
		t.Fatal(err)
	}
	if v.imagePixmap != first {
		t.Fatal("header-only change uploaded image")
	}
	v.viewport.one = true
	if err = v.State(v.state); err != nil {
		t.Fatal(err)
	}
	second := v.imagePixmap
	if second == first || second == 0 {
		t.Fatal("viewport reused wrong crop")
	}
	if _, err = xproto.GetGeometry(observer, xproto.Drawable(first)).Reply(); err == nil {
		t.Fatal("old viewport cache leaked")
	}
	v.width, v.height = 700, 520
	if err = v.State(v.state); err != nil {
		t.Fatal(err)
	}
	third := v.imagePixmap
	if third == second || third == 0 {
		t.Fatal("resize reused old geometry")
	}
	if _, err = xproto.GetGeometry(observer, xproto.Drawable(second)).Reply(); err == nil {
		t.Fatal("old resize cache leaked")
	}
	frame.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	encoded.Reset()
	if err = jpeg.Encode(&encoded, frame, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	if err = v.Frame(session.ScreenFrame{Image: encoded.Bytes()}); err != nil {
		t.Fatal(err)
	}
	last := v.imagePixmap
	if last == third || v.image == imageOwner {
		t.Fatal("changed frame reused old pixels")
	}
	if _, err = xproto.GetGeometry(observer, xproto.Drawable(third)).Reply(); err == nil {
		t.Fatal("old frame cache leaked")
	}
	if err = v.Close(); err != nil {
		t.Fatal(err)
	}
	if v.imagePixmap != 0 || v.cachedImage != nil || v.image != nil || v.lastJPEGValid {
		t.Fatal("closed view retained frame/cache ownership")
	}
	if _, err = xproto.GetGeometry(observer, xproto.Drawable(last)).Reply(); err == nil {
		t.Fatal("native cache survived Close")
	}
	t.Log("same complete frame/header reuse actual server pixmap; viewport/resize/changed frame replace and free old cache; Close frees final cache")
}
