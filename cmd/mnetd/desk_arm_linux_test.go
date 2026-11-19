//go:build linux

package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestDeskX11ArmedCancelNeverCreatesRecipient(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("requires disposable X display")
	}
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	keyboard, err := deskKeyboardFingerprint(conn)
	if err != nil {
		t.Fatal(err)
	}
	screen := xproto.Setup(conn).DefaultScreen(conn)
	layout, err := desk.Compile(desk.Layout{Version: 1, Displays: []desk.Display{{ID: "local", Device: "fixture-a", Width: 100, Height: 100, PixelWidth: int64(screen.WidthInPixels), PixelHeight: int64(screen.HeightInPixels)}, {ID: "remote", Device: "fixture-b", X: 100, Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768}}})
	if err != nil {
		t.Fatal(err)
	}
	board, err := desk.NewSwitchboard(layout, "fixture-a", "local", func() bool { return true }, func() {})
	if err != nil {
		t.Fatal(err)
	}
	binding := desk.Binding{Run: strings.Repeat("a", 64), Primary: "fixture-a", Recipient: "fixture-b", Layout: layout.Fingerprint(), Keyboard: keyboard, Display: "remote"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if capture, err := waitDeskEdge(ctx, board, desk.Right, binding, keyboard, func() bool { return true }, func(desk.Binding) (desk.NativeInput, error) {
		t.Fatal("canceled arm created native recipient")
		return nil, nil
	}); err == nil {
		capture.Close()
		t.Fatal("canceled arm activated")
	}
	select {
	case <-board.Stopped():
	default:
		t.Fatal("canceled arm left generation live")
	}
	safety, err := newDeskSafety(keyboard)
	if err != nil {
		t.Fatal("canceled arm retained native owner", err)
	}
	if err = safety.Close(); err != nil {
		t.Fatal(err)
	}
}
