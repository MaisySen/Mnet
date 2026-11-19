//go:build linux

package main

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

func TestDeskX11TwoDisplayCaptureAndReclaim(t *testing.T) {
	for _, mode := range []string{"local-stop", "stop-chord", "idle-peer-loss"} {
		t.Run(mode, func(t *testing.T) { deskTwoDisplayCaptureFixture(t, mode) })
	}
}
func deskTwoDisplayCaptureFixture(t *testing.T, mode string) {
	targetDisplay := os.Getenv("MNET_DESK_TARGET_DISPLAY")
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" || targetDisplay == "" {
		t.Skip("requires two disposable X displays")
	}
	source, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := xgb.NewConnDisplay(targetDisplay)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err = xtest.Init(source); err != nil {
		t.Fatal(err)
	}
	keyboard, err := deskKeyboardFingerprint(source)
	if err != nil {
		t.Fatal(err)
	}
	targetKeyboard, err := deskKeyboardFingerprint(target)
	if err != nil || keyboard != targetKeyboard {
		t.Fatal("fixture map mismatch")
	}
	sourceCert, sourceKey := deskCertificateFixture(t)
	targetCert, targetKey := deskCertificateFixture(t)
	sourceWindow := deskVisualFixture(t, source, "PRIMARY / disposable X11 fixture", []string{"Shared Desk source capture", "Raw keys and buttons route only to the exact approved Desk pair.", "No Remote Desktop screen stream, clipboard or files.", "Ctrl+Alt+Shift+F12 reclaims this keyboard locally."})
	defer xproto.DestroyWindow(source, sourceWindow)
	targetWindow := deskVisualFixture(t, target, "RECIPIENT / disposable X11 fixture", []string{"Shared Desk recipient input", "Native key A and left button are held during the checked stage.", "Pointer position is verified against this X server.", "These are two virtual displays, not two physical computers."})
	defer xproto.DestroyWindow(target, targetWindow)
	sourceScreen := xproto.Setup(source).DefaultScreen(source)
	targetScreen := xproto.Setup(target).DefaultScreen(target)
	layout, err := desk.Compile(desk.Layout{Version: 1, Displays: []desk.Display{
		{ID: "source", Device: sourceKey, Width: 1000, Height: 1000, PixelWidth: int64(sourceScreen.WidthInPixels), PixelHeight: int64(sourceScreen.HeightInPixels)},
		{ID: "target", Device: targetKey, X: 1000, Width: 1000, Height: 1000, PixelWidth: int64(targetScreen.WidthInPixels), PixelHeight: int64(targetScreen.HeightInPixels)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	binding := desk.Binding{Run: strings.Repeat("a", 64), Primary: sourceKey, Recipient: targetKey, Layout: layout.Fingerprint(), Keyboard: keyboard, Display: "target"}
	var capture atomic.Pointer[deskPrimaryCapture]
	var live atomic.Bool
	live.Store(true)
	board, err := desk.NewSwitchboard(layout, binding.Primary, "source", live.Load, func() {
		if c := capture.Load(); c != nil {
			c.disable()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	command := deskFixtureCommand()
	command.Env = append(command.Env, "DISPLAY="+targetDisplay)
	serverDone := make(chan error, 1)
	var channel *tls.Conn
	var serverConn *tls.Conn
	channel, serverConn = deskTLSPairFixture(t, sourceCert, targetCert)
	approval, err := desk.NewApproval(binding, layout, live.Load, func(current func() bool) (desk.NativeInput, error) {
		process, err := newInputProcessWithInit(command, &attendedProcessCommand{Kind: "desk-init", Keyboard: keyboard})
		if err != nil {
			return nil, err
		}
		return &deskProcessInput{process, current}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { serverDone <- approval.ServePair(serverConn) }()
	err = board.Cross(desk.Right, 300, binding, func(approved desk.Binding) (desk.NativeInput, error) {
		return desk.ConnectPair(channel, approved, layout, live.Load)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer board.Stop()
	c, err := newDeskPrimaryCapture(board, keyboard, live.Load)
	if err != nil {
		t.Fatal(err)
	}
	capture.Store(c)
	defer c.Close()
	press := func(kind, code byte) {
		t.Helper()
		if err := xtest.FakeInputChecked(source, kind, code, 0, sourceScreen.Root, 0, 0, 0).Check(); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { xtest.FakeInputChecked(source, xproto.KeyRelease, 38, 0, sourceScreen.Root, 0, 0, 0).Check() }()
	defer func() { xtest.FakeInputChecked(source, xproto.ButtonRelease, 1, 0, sourceScreen.Root, 0, 0, 0).Check() }()
	press(xproto.KeyPress, 38)
	press(xproto.ButtonPress, 1)
	sk, se := xproto.QueryKeymap(source).Reply()
	if se != nil || sk.Keys[38/8]&(1<<(38%8)) == 0 {
		t.Fatal("source fixture did not press key")
	}
	until := time.Now().Add(time.Second)
	for {
		keys, e := xproto.QueryKeymap(target).Reply()
		pointer, pe := xproto.QueryPointer(target, targetScreen.Root).Reply()
		if e == nil && pe == nil && keys.Keys[38/8]&(1<<(38%8)) != 0 && pointer.Mask&xproto.ButtonMask1 != 0 && c.forwarded.Load() >= 2 {
			break
		}
		select {
		case <-c.done:
			t.Fatal("source loop ended early", c.Close())
		default:
		}
		if time.Now().After(until) {
			t.Fatal("captured source input never reached independent target display", c.forwarded.Load(), keys.Keys, pointer.Mask, c.Close())
		}
		time.Sleep(time.Millisecond)
	}

	// Test source motion as actual capture, not a direct backend command.
	startX, startY := board.Position()
	if err = xtest.FakeInputChecked(source, xproto.MotionNotify, 0, 0, sourceScreen.Root, int16(sourceScreen.WidthInPixels/2+40), int16(sourceScreen.HeightInPixels/2+20), 0).Check(); err != nil {
		t.Fatal(err)
	}
	until = time.Now().Add(time.Second)
	for {
		p, e := xproto.QueryPointer(target, targetScreen.Root).Reply()
		if e == nil && int64(p.RootX) == startX+40 && int64(p.RootY) == startY+20 && c.forwarded.Load() >= 3 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("captured pointer did not map to exact recipient pixels")
		}
		time.Sleep(time.Millisecond)
	}
	nativePointer, err := xproto.QueryPointer(target, targetScreen.Root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	deskFixtureObserved(t, target, targetWindow, fmt.Sprintf("OBSERVED: key 38 DOWN; button 1 DOWN; native pointer (%d, %d)", nativePointer.RootX, nativePointer.RootY))
	deskFixtureObserved(t, source, sourceWindow, fmt.Sprintf("OBSERVED: %d acknowledged Desk input transitions", c.forwarded.Load()))
	if artifacts := os.Getenv("MNET_DESK_ARTIFACTS"); artifacts != "" && mode == "local-stop" {
		if err = os.WriteFile(filepath.Join(artifacts, "held.stage"), []byte("native states and pointer checked"), 0600); err != nil {
			t.Fatal(err)
		}
		until = time.Now().Add(10 * time.Second)
		for {
			if _, err = os.Stat(filepath.Join(artifacts, "held.captured")); err == nil {
				break
			}
			if time.Now().After(until) {
				t.Fatal("visual capture acknowledgement missing")
			}
			time.Sleep(time.Millisecond)
		}
	}
	if mode == "idle-peer-loss" {
		press(xproto.KeyRelease, 38)
		until := time.Now().Add(time.Second)
		for c.forwarded.Load() < 4 {
			if time.Now().After(until) {
				t.Fatal("fixture key release not acknowledged")
			}
			time.Sleep(time.Millisecond)
		}
		serverConn.NetConn().Close()
		until = time.Now().Add(time.Second)
		for {
			grabbed, err := xproto.GrabKeyboard(source, false, sourceScreen.Root, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
			if err == nil && grabbed.Status == xproto.GrabStatusSuccess {
				xproto.UngrabKeyboardChecked(source, xproto.TimeCurrentTime).Check()
				break
			}
			if time.Now().After(until) {
				t.Fatal("idle peer loss left source capture grabbed")
			}
			time.Sleep(time.Millisecond)
		}
		if err = c.Close(); err == nil {
			t.Fatal("lost peer cleanup incorrectly confirmed")
		}
		if err = <-serverDone; err == nil {
			t.Fatal("lost peer reported clean stop")
		}
		return
	}
	// Explicit local stop releases capture before joined remote cleanup.
	if mode == "stop-chord" {
		for _, symbol := range []xproto.Keysym{0xffe3, 0xffe9, 0xffe1, attendedStopKeysym} {
			code := keycodeFor(source, symbol)
			press(xproto.KeyPress, code)
			defer func(code byte) {
				xtest.FakeInputChecked(source, xproto.KeyRelease, code, 0, sourceScreen.Root, 0, 0, 0).Check()
			}(code)
		}

		until := time.Now().Add(time.Second)
		for {
			grabbed, err := xproto.GrabKeyboard(source, false, sourceScreen.Root, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
			if err == nil && grabbed.Status == xproto.GrabStatusSuccess {
				xproto.UngrabKeyboardChecked(source, xproto.TimeCurrentTime).Check()
				break
			}
			if time.Now().After(until) {
				t.Fatal("local stop chord left source keyboard grabbed")
			}
			time.Sleep(time.Millisecond)
		}

	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-serverDone; err != nil {
		t.Fatal(err)
	}
	until = time.Now().Add(time.Second)
	for {
		keys, e := xproto.QueryKeymap(target).Reply()
		pointer, pe := xproto.QueryPointer(target, targetScreen.Root).Reply()
		if e == nil && pe == nil && keys.Keys[38/8]&(1<<(38%8)) == 0 && pointer.Mask&xproto.ButtonMask1 == 0 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("pair loss left recipient state held")
		}
		time.Sleep(time.Millisecond)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	// A new local client must actually acquire the keyboard, not trust a label.
	grab, err := xproto.GrabKeyboard(source, false, sourceScreen.Root, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
	if err != nil || grab.Status != xproto.GrabStatusSuccess {
		t.Fatal("source keyboard not reclaimed")
	}
	if err = xproto.UngrabKeyboardChecked(source, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
}

func deskVisualFixture(t *testing.T, c *xgb.Conn, title string, lines []string) xproto.Window {
	t.Helper()
	screen := xproto.Setup(c).DefaultScreen(c)
	window, err := xproto.NewWindowId(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, window, screen.Root, 0, 0, screen.WidthInPixels, screen.HeightInPixels, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel, []uint32{0x152b44}).Check(); err != nil {
		t.Fatal(err)
	}
	xproto.ChangeProperty(c, xproto.PropModeReplace, window, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title))
	font, err := xproto.NewFontId(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.OpenFontChecked(c, font, 5, "fixed").Check(); err != nil {
		t.Fatal(err)
	}
	defer xproto.CloseFont(c, font)
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(window), xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{0xffffff, 0x152b44, uint32(font)}).Check(); err != nil {
		t.Fatal(err)
	}
	defer xproto.FreeGC(c, gc)
	if err = xproto.MapWindowChecked(c, window).Check(); err != nil {
		t.Fatal(err)
	}
	for i, line := range append([]string{title}, lines...) {
		if err = xproto.ImageText8Checked(c, byte(len(line)), xproto.Drawable(window), gc, 32, int16(60+i*36), line).Check(); err != nil {
			t.Fatal(err)
		}
	}
	return window
}

func deskFixtureObserved(t *testing.T, c *xgb.Conn, window xproto.Window, line string) {
	t.Helper()
	gc, err := xproto.NewGcontextId(c)
	if err != nil {
		t.Fatal(err)
	}
	font, err := xproto.NewFontId(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.OpenFontChecked(c, font, 5, "fixed").Check(); err != nil {
		t.Fatal(err)
	}
	defer xproto.CloseFont(c, font)
	if err = xproto.CreateGCChecked(c, gc, xproto.Drawable(window), xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{0x9bffb2, 0x152b44, uint32(font)}).Check(); err != nil {
		t.Fatal(err)
	}
	defer xproto.FreeGC(c, gc)
	if err = xproto.ImageText8Checked(c, byte(len(line)), xproto.Drawable(window), gc, 32, 300, line).Check(); err != nil {
		t.Fatal(err)
	}
}
