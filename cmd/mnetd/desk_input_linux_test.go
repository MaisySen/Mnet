//go:build linux

package main

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestDeskX11PhysicalTransitionsAndOwnedRelease(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("requires disposable X display")
	}
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	fingerprint, err := deskKeyboardFingerprint(observer)
	if err != nil {
		t.Fatal(err)
	}
	var current atomic.Bool
	current.Store(true)
	native, err := newDeskNativeInput(fingerprint, current.Load)
	if err != nil {
		t.Fatal(err)
	}
	defer native.Release()
	recipient, err := desk.NewRecipient(native, native.width, native.height, current.Load)
	if err != nil {
		t.Fatal(err)
	}
	setup := xproto.Setup(observer)
	mapping, err := xproto.GetKeyboardMapping(observer, setup.MinKeycode, byte(setup.MaxKeycode-setup.MinKeycode+1)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	var code uint16
	for i, sym := range mapping.Keysyms {
		if sym == 'a' {
			code = uint16(setup.MinKeycode) + uint16(i)/uint16(mapping.KeysymsPerKeycode)
			break
		}
	}
	if code == 0 {
		t.Fatal("fixture physical key unavailable")
	}
	for i, ev := range []desk.Input{{Kind: "pointer", Action: "move", X: 100, Y: 120}, {Kind: "key", Action: "down", Code: code}, {Kind: "button", Action: "down", Code: 1}} {
		ev.Sequence = uint64(i + 1)
		if err = recipient.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := xproto.QueryKeymap(observer).Reply()
	if err != nil || keys.Keys[code/8]&(1<<(code%8)) == 0 {
		t.Fatal("actual native key down not observed")
	}
	root := setup.DefaultScreen(observer).Root
	p, err := xproto.QueryPointer(observer, root).Reply()
	if err != nil || p.Mask&xproto.ButtonMask1 == 0 || p.RootX != 100 || p.RootY != 120 {
		t.Fatal("native pointer transition not observed")
	}
	current.Store(false)
	until := time.Now().Add(time.Second)
	for {
		p, err = xproto.QueryPointer(observer, root).Reply()
		keys, keyErr := xproto.QueryKeymap(observer).Reply()
		if err == nil && keyErr == nil && p.Mask&xproto.ButtonMask1 == 0 && keys.Keys[code/8]&(1<<(code%8)) == 0 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("lost pair left actual held state")
		}
		time.Sleep(time.Millisecond)
	}
	if err = recipient.Close(); err != nil {
		t.Fatal(err)
	}
	if err = recipient.Apply(desk.Input{Sequence: 4, Kind: "key", Action: "down", Code: code}); err == nil {
		t.Fatal("retired native recipient reused")
	}
	if other, err := newDeskNativeInput("wrong-keyboard", func() bool { return true }); err == nil {
		other.Release()
		t.Fatal("mismatched map admitted")
	}
}

func TestDeskX11ChangedMapRetiresAndReleases(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("requires disposable X display")
	}
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	fingerprint, err := deskKeyboardFingerprint(observer)
	if err != nil {
		t.Fatal(err)
	}
	native, err := newDeskNativeInput(fingerprint, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer native.Release()
	if other, err := newAttendedSafety(); err == nil {
		other.Close()
		t.Fatal("Remote Desktop acquired simultaneous input ownership")
	}
	r, err := desk.NewRecipient(native, native.width, native.height, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(desk.Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}); err != nil {
		t.Fatal(err)
	}
	mapping, err := xproto.GetKeyboardMapping(observer, 38, 1).Reply()
	if err != nil {
		t.Fatal(err)
	}
	original := append([]xproto.Keysym(nil), mapping.Keysyms...)
	defer func() {
		if err := xproto.ChangeKeyboardMappingChecked(observer, 1, 38, mapping.KeysymsPerKeycode, original).Check(); err != nil {
			t.Error(err)
		}
	}()
	changed := append([]xproto.Keysym(nil), original...)
	changed[0] = 'z'
	if err = xproto.ChangeKeyboardMappingChecked(observer, 1, 38, mapping.KeysymsPerKeycode, changed).Check(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-native.done:
	case <-time.After(time.Second):
		t.Fatal("changed keyboard map left native recipient active")
	}
	keys, err := xproto.QueryKeymap(observer).Reply()
	if err != nil || keys.Keys[38/8]&(1<<(38%8)) != 0 {
		t.Fatal("changed map left key held")
	}
	if err = r.Apply(desk.Input{Sequence: 2, Kind: "key", Action: "down", Code: 38}); err == nil {
		t.Fatal("changed-map recipient reused")
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
}
