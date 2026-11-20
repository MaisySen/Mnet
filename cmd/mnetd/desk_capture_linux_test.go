//go:build linux

package main

import (
	"context"
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
	for _, mode := range []string{"local-stop", "stop-chord", "idle-peer-loss", "idle-native-child-loss", "source-map-loss", "source-geometry-loss", "source-geometry-transient", "edge-return", "armed-edge-entry", "armed-left-entry", "armed-top-entry", "armed-bottom-entry", "reviewed-edge-entry"} {
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
	entryEdge := desk.Right
	targetX, targetY := int64(1000), int64(0)
	edgeX, edgeY := int16(sourceScreen.WidthInPixels-1), int16(300)
	moveX, moveY := int16(40), int16(20)
	switch mode {
	case "armed-left-entry":
		entryEdge = desk.Left
		targetX = -1000
		edgeX = 0
		moveX = -40
	case "armed-top-entry":
		entryEdge = desk.Top
		targetX, targetY = 0, -1000
		edgeX, edgeY = 500, 0
		moveY = -20
	case "armed-bottom-entry":
		entryEdge = desk.Bottom
		targetX, targetY = 0, 1000
		edgeX, edgeY = 500, int16(sourceScreen.HeightInPixels-1)
	}
	armed := strings.HasPrefix(mode, "armed-") || mode == "reviewed-edge-entry"
	layout, err := desk.Compile(desk.Layout{Version: 1, Displays: []desk.Display{
		{ID: "source", Device: sourceKey, Width: 1000, Height: 1000, PixelWidth: int64(sourceScreen.WidthInPixels), PixelHeight: int64(sourceScreen.HeightInPixels)},
		{ID: "target", Device: targetKey, X: targetX, Y: targetY, Width: 1000, Height: 1000, PixelWidth: int64(targetScreen.WidthInPixels), PixelHeight: int64(targetScreen.HeightInPixels)},
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
	var nativeProcess *attendedProcessInjector

	createNative := func(current func() bool) (desk.NativeInput, error) {
		process, err := newInputProcessWithInit(command, &attendedProcessCommand{Kind: "desk-init", Keyboard: keyboard})
		if err != nil {
			return nil, err
		}
		nativeProcess = process
		return &deskProcessInput{process, current}, nil
	}
	var approval *desk.Approval
	if mode == "reviewed-edge-entry" {
		ui, uiErr := newDeskReviewUI()
		if uiErr != nil {
			t.Fatal(uiErr)
		}
		recipientUI := ui.(deskZenityReview)
		recipientUI.display = targetDisplay
		approval, err = reviewedDeskApproval(context.Background(), binding, layout, live.Load, recipientUI, createNative)
	} else {
		approval, err = desk.NewApproval(binding, layout, live.Load, createNative)
	}

	if err != nil {
		t.Fatal(err)
	}

	var c *deskPrimaryCapture
	create := func(approved desk.Binding) (desk.NativeInput, error) {
		channel, serverConn = deskTLSPairFixture(t, sourceCert, targetCert)
		go func() { serverDone <- approval.ServePair(serverConn) }()
		return desk.ConnectPair(channel, approved, layout, live.Load)
	}
	if armed {
		xproto.WarpPointerChecked(source, 0, sourceScreen.Root, 0, 0, 0, 0, edgeX, edgeY).Check()
		type result struct {
			capture *deskPrimaryCapture
			err     error
		}
		ready := make(chan result, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			var capture *deskPrimaryCapture
			var err error
			if mode == "reviewed-edge-entry" {
				ui, uiErr := newDeskReviewUI()
				if uiErr != nil {
					ready <- result{nil, uiErr}
					return
				}
				capture, err = reviewedDeskEdge(ctx, board, entryEdge, binding, layout, keyboard, live.Load, ui, create)
			} else {
				capture, err = waitDeskEdge(ctx, board, entryEdge, binding, keyboard, live.Load, create)
			}
			ready <- result{capture, err}
		}()
		if mode == "reviewed-edge-entry" {
			artifacts := os.Getenv("MNET_DESK_ARTIFACTS")
			until := time.Now().Add(10 * time.Second)
			for {
				if _, err = os.Stat(filepath.Join(artifacts, "primary-review.allowed")); err == nil {
					break
				}
				if time.Now().After(until) {
					t.Fatal("source review was not answered")
				}
				time.Sleep(time.Millisecond)
			}
		}
		// Already resting on edge must not enable capture. Visit the interior
		// while holding a button, then return to the edge: still not allowed.
		time.Sleep(30 * time.Millisecond)
		select {
		case <-ready:
			t.Fatal("resting edge activated")
		default:
		}
		if err = xtest.FakeInputChecked(source, xproto.ButtonPress, 1, 0, sourceScreen.Root, 0, 0, 0).Check(); err != nil {
			t.Fatal(err)
		}
		defer func() { xtest.FakeInputChecked(source, xproto.ButtonRelease, 1, 0, sourceScreen.Root, 0, 0, 0).Check() }()
		xproto.WarpPointerChecked(source, 0, sourceScreen.Root, 0, 0, 0, 0, 500, 300).Check()
		time.Sleep(30 * time.Millisecond)
		xproto.WarpPointerChecked(source, 0, sourceScreen.Root, 0, 0, 0, 0, edgeX, edgeY).Check()
		time.Sleep(30 * time.Millisecond)
		select {
		case <-ready:
			t.Fatal("held edge activated")
		default:
		}
		if err = xtest.FakeInputChecked(source, xproto.ButtonRelease, 1, 0, sourceScreen.Root, 0, 0, 0).Check(); err != nil {
			t.Fatal(err)
		}
		xproto.WarpPointerChecked(source, 0, sourceScreen.Root, 0, 0, 0, 0, 500, 300).Check()
		time.Sleep(30 * time.Millisecond)
		xproto.WarpPointerChecked(source, 0, sourceScreen.Root, 0, 0, 0, 0, edgeX, edgeY).Check()
		select {
		case answer := <-ready:
			c, err = answer.capture, answer.err
		case <-time.After(2 * time.Second):
			t.Fatal("approved edge arrival did not enable source capture")
		}
	} else {
		err = board.Cross(desk.Right, 300, binding, create)
		if err == nil {
			c, err = newDeskPrimaryCapture(board, keyboard, live.Load)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	capture.Store(c)
	defer c.Close()
	// Independent same-axis rational mapping, not a call to MapEdge. The actual
	// recipient must already be at this pixel before any source key/button input.
	expectedX, expectedY := int64(0), int64(300)*int64(targetScreen.HeightInPixels)/int64(sourceScreen.HeightInPixels)
	switch entryEdge {
	case desk.Left:
		expectedX = int64(targetScreen.WidthInPixels) - 1
	case desk.Top:
		expectedX = int64(500) * int64(targetScreen.WidthInPixels) / int64(sourceScreen.WidthInPixels)
		expectedY = int64(targetScreen.HeightInPixels) - 1
	case desk.Bottom:
		expectedX = int64(500) * int64(targetScreen.WidthInPixels) / int64(sourceScreen.WidthInPixels)
		expectedY = 0
	}
	positionX, positionY := board.Position()
	initial, initialErr := xproto.QueryPointer(target, targetScreen.Root).Reply()
	if initialErr != nil || positionX != expectedX || positionY != expectedY || int64(initial.RootX) != expectedX || int64(initial.RootY) != expectedY {
		t.Fatal("entry differs from independent native pixel oracle", entryEdge, positionX, positionY, initial, initialErr)
	}
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
	if err = xtest.FakeInputChecked(source, xproto.MotionNotify, 0, 0, sourceScreen.Root, int16(sourceScreen.WidthInPixels/2)+moveX, int16(sourceScreen.HeightInPixels/2)+moveY, 0).Check(); err != nil {
		t.Fatal(err)
	}
	until = time.Now().Add(time.Second)
	for {
		p, e := xproto.QueryPointer(target, targetScreen.Root).Reply()
		if e == nil && int64(p.RootX) == startX+int64(moveX) && int64(p.RootY) == startY+int64(moveY) && c.forwarded.Load() >= 3 {
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
	if artifacts := os.Getenv("MNET_DESK_ARTIFACTS"); artifacts != "" && (mode == "armed-left-entry" || mode == "armed-top-entry" || mode == "armed-bottom-entry") {
		if err = os.WriteFile(filepath.Join(artifacts, mode+".stage"), []byte("native entry, source motion and held ledger checked"), 0600); err != nil {
			t.Fatal(err)
		}
		until = time.Now().Add(5 * time.Second)
		for {
			if _, err = os.Stat(filepath.Join(artifacts, mode+".captured")); err == nil {
				break
			}
			if time.Now().After(until) {
				t.Fatal("directional entry screenshot acknowledgement missing", mode)
			}
			time.Sleep(time.Millisecond)
		}
	}
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
	if mode == "source-geometry-loss" || mode == "source-geometry-transient" {
		restore := deskFixtureResizeSource(t, source)
		defer restore()
		if mode == "source-geometry-transient" {
			restore()
		}
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			t.Fatal("changed source geometry left held-input capture active")
		}
		closeErr := c.Close()
		if closeErr == nil || !strings.Contains(closeErr.Error(), "source geometry changed") {
			t.Fatal("source geometry loss was not preserved as error", closeErr)
		}
		select {
		case err := <-serverDone:
			if err != nil {
				t.Fatal("source geometry loss did not join remote cleanup", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("source geometry loss left pair server live")
		}
		keys, ke := xproto.QueryKeymap(target).Reply()
		pointer, pe := xproto.QueryPointer(target, targetScreen.Root).Reply()
		if ke != nil || pe != nil || keys.Keys[38/8]&(1<<(38%8)) != 0 || pointer.Mask&xproto.ButtonMask1 != 0 {
			t.Fatal("source geometry loss left recipient held", ke, pe)
		}
		grab, err := xproto.GrabKeyboard(source, false, sourceScreen.Root, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
		if err != nil || grab.Status != xproto.GrabStatusSuccess {
			t.Fatal("source geometry change left source grabbed", err)
		}
		xproto.UngrabKeyboardChecked(source, xproto.TimeCurrentTime).Check()
		if err = board.Apply(desk.Input{Sequence: 100, Kind: "key", Action: "down", Code: 38}); err == nil {
			t.Fatal("changed-geometry board reused")
		}
		if mode == "source-geometry-loss" {
			deskFixtureObserved(t, source, sourceWindow, "OBSERVED: root resized to800x600; capture retired; local keyboard reclaimed")
		} else {
			deskFixtureObserved(t, source, sourceWindow, "OBSERVED: transient root resize restored; old capture still retired")
		}
		deskFixtureObserved(t, target, targetWindow, "OBSERVED: source geometry retired; native key38/button1 released; pair cleanup joined")
		if artifacts := os.Getenv("MNET_DESK_ARTIFACTS"); artifacts != "" && mode == "source-geometry-loss" {
			if err = os.WriteFile(filepath.Join(artifacts, "source-geometry.stage"), []byte("actual resized source and native held-input cleanup checked"), 0600); err != nil {
				t.Fatal(err)
			}
			until = time.Now().Add(5 * time.Second)
			for {
				if _, err = os.Stat(filepath.Join(artifacts, "source-geometry.captured")); err == nil {
					break
				}
				if time.Now().After(until) {
					t.Fatal("resized root visual acknowledgement missing")
				}
				time.Sleep(time.Millisecond)
			}
		}
		t.Log("source root geometry change retired capture and joined native held-input cleanup; local close result:", closeErr)
		return
	}
	if mode == "source-map-loss" {
		mapping, err := xproto.GetKeyboardMapping(source, 38, 1).Reply()
		if err != nil {
			t.Fatal(err)
		}
		// An unchanged mapping notification is not an identity/layout change. A
		// real native transition after the event must still reach the recipient.
		if err = xproto.ChangeKeyboardMappingChecked(source, 1, 38, mapping.KeysymsPerKeycode, mapping.Keysyms).Check(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
		if err = xtest.FakeInputChecked(source, xproto.MotionNotify, 0, 0, sourceScreen.Root, int16(sourceScreen.WidthInPixels/2+1), int16(sourceScreen.HeightInPixels/2), 0).Check(); err != nil {
			t.Fatal(err)
		}
		until = time.Now().Add(time.Second)
		for {
			pointer, err := xproto.QueryPointer(target, targetScreen.Root).Reply()
			if err == nil && int64(pointer.RootX) == startX+41 && int64(pointer.RootY) == startY+20 && c.forwarded.Load() >= 4 {
				break
			}
			select {
			case <-c.done:
				t.Fatal("unchanged source map notification retired capture", c.Close())
			default:
			}
			if time.Now().After(until) {
				t.Fatal("unchanged map did not preserve native routing")
			}
			time.Sleep(time.Millisecond)
		}
		restore := deskFixtureChangeSourceMap(t, source)
		defer restore()
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			t.Fatal("source map change left capture active")
		}
		// Cleanup may race the safety watcher and event loop. Either path is terminal;
		// the surviving recipient must release the exact held key/button ledger.
		closeErr := c.Close()
		select {
		case err := <-serverDone:
			if err != nil {
				t.Fatal("source map loss did not join remote cleanup", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("source map loss left pair server live")
		}
		keys, ke := xproto.QueryKeymap(target).Reply()
		pointer, pe := xproto.QueryPointer(target, targetScreen.Root).Reply()
		if ke != nil || pe != nil || keys.Keys[38/8]&(1<<(38%8)) != 0 || pointer.Mask&xproto.ButtonMask1 != 0 {
			t.Fatal("source map loss left recipient held", ke, pe)
		}
		grabbed, err := xproto.GrabKeyboard(source, false, sourceScreen.Root, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
		if err != nil || grabbed.Status != xproto.GrabStatusSuccess {
			t.Fatal("source map loss left capture grabbed", err)
		}
		xproto.UngrabKeyboardChecked(source, xproto.TimeCurrentTime).Check()
		if err = board.Apply(desk.Input{Sequence: 100, Kind: "key", Action: "down", Code: 38}); err == nil {
			t.Fatal("changed-map board reused")
		}
		t.Log("source map change retired capture and joined native held-input cleanup; local close result:", closeErr)
		return
	}
	if mode == "edge-return" {
		// Cross left while key/button held: native routing must remain remote.
		if err = xtest.FakeInputChecked(source, xproto.MotionNotify, 0, 0, sourceScreen.Root, 0, int16(sourceScreen.HeightInPixels/2), 0).Check(); err != nil {
			t.Fatal(err)
		}
		until = time.Now().Add(time.Second)
		for {
			p, e := xproto.QueryPointer(target, targetScreen.Root).Reply()
			if e == nil && p.RootX == 0 && c.forwarded.Load() >= 4 {
				break
			}
			if time.Now().After(until) {
				t.Fatal("held edge fixture did not reach recipient boundary")
			}
			time.Sleep(time.Millisecond)
		}
		select {
		case <-c.done:
			t.Fatal("held edge crossed")
		default:
		}
		press(xproto.KeyRelease, 38)
		press(xproto.ButtonRelease, 1)
		until = time.Now().Add(time.Second)
		for c.forwarded.Load() < 6 {
			if time.Now().After(until) {
				t.Fatal("edge fixture releases not acknowledged")
			}
			time.Sleep(time.Millisecond)
		}
		if err = xtest.FakeInputChecked(source, xproto.MotionNotify, 0, 0, sourceScreen.Root, int16(sourceScreen.WidthInPixels/2-5), int16(sourceScreen.HeightInPixels/2), 0).Check(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			t.Fatal("native motion did not return across approved local edge")
		}
		if err = c.Close(); err != nil {
			t.Fatal(err)
		}
		if err = <-serverDone; err != nil {
			t.Fatal(err)
		}
		p, err := xproto.QueryPointer(source, sourceScreen.Root).Reply()
		if err != nil || p.RootX != int16(sourceScreen.WidthInPixels-1) || int64(p.RootY) != (startY+20)*int64(sourceScreen.HeightInPixels)/int64(targetScreen.HeightInPixels) {
			t.Fatal("local pointer not restored to mapped pixel", p, err)
		}
		deskFixtureObserved(t, source, sourceWindow, fmt.Sprintf("OBSERVED: local edge return at (%d, %d); recipient cleanup acknowledged", p.RootX, p.RootY))
		if artifacts := os.Getenv("MNET_DESK_ARTIFACTS"); artifacts != "" {
			if err = os.WriteFile(filepath.Join(artifacts, "return.stage"), []byte("local pointer mapped after cleanup"), 0600); err != nil {
				t.Fatal(err)
			}
			until = time.Now().Add(10 * time.Second)
			for {
				if _, err = os.Stat(filepath.Join(artifacts, "return.captured")); err == nil {
					break
				}
				if time.Now().After(until) {
					t.Fatal("return screenshot acknowledgement missing")
				}
				time.Sleep(time.Millisecond)
			}
		}
		grab, err := xproto.GrabKeyboard(source, false, sourceScreen.Root, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
		if err != nil || grab.Status != xproto.GrabStatusSuccess {
			t.Fatal("edge return left source keyboard grabbed")
		}
		xproto.UngrabKeyboardChecked(source, xproto.TimeCurrentTime).Check()
		return
	}
	if mode == "idle-peer-loss" || mode == "idle-native-child-loss" {
		press(xproto.KeyRelease, 38)
		until := time.Now().Add(time.Second)
		for c.forwarded.Load() < 4 {
			if time.Now().After(until) {
				t.Fatal("fixture key release not acknowledged")
			}
			time.Sleep(time.Millisecond)
		}
		if mode == "idle-native-child-loss" {
			if err = nativeProcess.cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
		} else {
			serverConn.NetConn().Close()
		}
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
		if mode == "idle-native-child-loss" {
			pointer, err := xproto.QueryPointer(target, targetScreen.Root).Reply()
			if err != nil || pointer.Mask&xproto.ButtonMask1 == 0 {
				t.Fatal("expected XTEST injector-death negative changed", err)
			}
			// This is fixture cleanup only, never a proposed production recovery.
			if err = xtest.Init(target); err != nil {
				t.Fatal(err)
			}
			if err = xtest.FakeInputChecked(target, xproto.ButtonRelease, 1, 0, targetScreen.Root, 0, 0, 0).Check(); err != nil {
				t.Fatal(err)
			}
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
