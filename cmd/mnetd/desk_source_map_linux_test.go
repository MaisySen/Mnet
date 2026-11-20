//go:build linux

package main

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Only a disposable server may have its global keyboard map changed. Restore
// with an independent observer after the old owner has completely retired.
func deskFixtureChangeSourceMap(t *testing.T, c *xgb.Conn) func() {
	t.Helper()
	mapping, err := xproto.GetKeyboardMapping(c, 38, 1).Reply()
	if err != nil || len(mapping.Keysyms) == 0 {
		t.Fatal("fixture source map unavailable", err)
	}
	original := append([]xproto.Keysym(nil), mapping.Keysyms...)
	changed := append([]xproto.Keysym(nil), original...)
	changed[0] = 'z'
	if changed[0] == original[0] {
		changed[0] = 'x'
	}
	if err = xproto.ChangeKeyboardMappingChecked(c, 1, 38, mapping.KeysymsPerKeycode, changed).Check(); err != nil {
		t.Fatal(err)
	}
	var restored atomic.Bool
	return func() {
		if restored.Swap(true) {
			return
		}
		if err := xproto.ChangeKeyboardMappingChecked(c, 1, 38, mapping.KeysymsPerKeycode, original).Check(); err != nil {
			t.Error(err)
		}
	}
}
func TestDeskX11ArmedSourceMapChangeCannotCreateRecipient(t *testing.T) {
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
	defer board.Stop()
	binding := desk.Binding{Run: strings.Repeat("a", 64), Primary: "fixture-a", Recipient: "fixture-b", Layout: layout.Fingerprint(), Keyboard: keyboard, Display: "remote"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var creates atomic.Int64
	done := make(chan error, 1)
	go func() {
		capture, err := waitDeskEdge(ctx, board, desk.Right, binding, keyboard, func() bool { return true }, func(desk.Binding) (desk.NativeInput, error) { creates.Add(1); return nil, nil })
		if capture != nil {
			capture.Close()
		}
		done <- err
	}()
	atom, err := xproto.InternAtom(conn, false, uint16(len(attendedInputOwnerAtom)), attendedInputOwnerAtom).Reply()
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	for {
		owner, err := xproto.GetSelectionOwner(conn, atom.Atom).Reply()
		if err == nil && owner.Owner != 0 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("armed safety owner never appeared")
		}
		time.Sleep(time.Millisecond)
	}
	restore := deskFixtureChangeSourceMap(t, conn)
	defer restore()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("changed-map arm reported successful capture")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("changed-map arm failed to retire")
	}
	if creates.Load() != 0 {
		t.Fatal("changed-map arm created recipient")
	}
	select {
	case <-board.Stopped():
	default:
		t.Fatal("changed-map arm retained generation")
	}
	owner, err := xproto.GetSelectionOwner(conn, atom.Atom).Reply()
	if err != nil || owner.Owner != 0 {
		t.Fatal("changed-map arm retained local input owner", err)
	}
	// Restoring the keyboard map is fixture repair only; the retired board must
	// not become reusable merely because the source layout now looks familiar.
	restore()
	if err = board.Cross(desk.Right, 10, binding, func(desk.Binding) (desk.NativeInput, error) { creates.Add(1); return nil, nil }); err == nil || creates.Load() != 0 {
		t.Fatal("restored map reactivated retired arm")
	}
	safety, err := newDeskSafety(keyboard)
	if err != nil {
		t.Fatal("old map arm retained shortcut/ownership", err)
	}
	if err = safety.Close(); err != nil {
		t.Fatal(err)
	}
}
