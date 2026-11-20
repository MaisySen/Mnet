//go:build linux

package main

import (
	"os"
	"strings"
	"testing"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// Independent XTEST state represents another local input owner. This cannot
// distinguish real physical input from synthetic input on the same key.
func TestDeskX11ForeignStateIsNotClaimedOrReleased(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("disposable X11 only")
	}
	c, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err = xtest.Init(c); err != nil {
		t.Fatal(err)
	}
	root := xproto.Setup(c).DefaultScreen(c).Root
	keyboard, err := deskKeyboardFingerprint(c)
	if err != nil {
		t.Fatal(err)
	}
	emit := func(kind, code byte) {
		t.Helper()
		if err := xtest.FakeInputChecked(c, kind, code, 0, root, 0, 0, 0).Check(); err != nil {
			t.Fatal(err)
		}
	}
	state := func(key byte, button uint16) (bool, bool) {
		t.Helper()
		keys, err := xproto.QueryKeymap(c).Reply()
		if err != nil {
			t.Fatal(err)
		}
		pointer, err := xproto.QueryPointer(c, root).Reply()
		if err != nil {
			t.Fatal(err)
		}
		return keys.Keys[key/8]&(1<<(key%8)) != 0, pointer.Mask&button != 0
	}
	for _, kind := range []string{"key", "button"} {
		t.Run("preheld-"+kind, func(t *testing.T) {
			press, release, code := byte(xproto.KeyPress), byte(xproto.KeyRelease), byte(39)
			if kind == "button" {
				press, release, code = xproto.ButtonPress, xproto.ButtonRelease, 3
			}
			emit(press, code)
			defer emit(release, code)
			native, err := newDeskNativeInput(keyboard, func() bool { return true })
			if err == nil {
				native.Release()
				t.Fatal("preheld foreign state admitted")
			}
			key, button := state(39, xproto.ButtonMask3)
			if key != (kind == "key") || button != (kind == "button") {
				t.Fatal("failed setup changed foreign state", key, button)
			}
		})
	}
	for _, kind := range []string{"key", "button"} {
		t.Run("late-foreign-"+kind, func(t *testing.T) {
			native, err := newDeskNativeInput(keyboard, func() bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			defer native.Release()
			r, err := desk.NewRecipient(native, native.width, native.height, func() bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			for i, ev := range []desk.Input{{Kind: "key", Action: "down", Code: 38}, {Kind: "button", Action: "down", Code: 1}} {
				ev.Sequence = uint64(i + 1)
				if err = r.Apply(ev); err != nil {
					t.Fatal(err)
				}
			}
			if key, button := state(38, xproto.ButtonMask1); !key || !button {
				t.Fatal("owned fixture state absent")
			}
			emit(xproto.KeyPress, 39)
			defer emit(xproto.KeyRelease, 39)
			emit(xproto.ButtonPress, 3)
			defer emit(xproto.ButtonRelease, 3)
			ev := desk.Input{Sequence: 3, Kind: kind, Action: "down", Code: 39}
			if kind == "button" {
				ev.Code = 3
			}
			err = r.Apply(ev)
			if err == nil || !strings.Contains(err.Error(), "foreign preheld "+kind) {
				t.Fatal("foreign state not refused specifically", err)
			}
			if key, button := state(38, xproto.ButtonMask1); key || button {
				t.Fatal("own ledger not released", key, button)
			}
			if key, button := state(39, xproto.ButtonMask3); !key || !button {
				t.Fatal("foreign ledger incorrectly released", key, button)
			}
			if err = r.Apply(desk.Input{Sequence: 4, Kind: "pointer", Action: "move"}); err == nil {
				t.Fatal("foreign collision allowed successor input")
			}
			if err = r.Close(); err != nil {
				t.Fatal(err)
			}
			if key, button := state(39, xproto.ButtonMask3); !key || !button {
				t.Fatal("repeated cleanup changed foreign state")
			}
		})
	}
}
