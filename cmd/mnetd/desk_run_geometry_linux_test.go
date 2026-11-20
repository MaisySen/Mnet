//go:build linux

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestDeskX11ReviewedGeometryGuardJoinsAndNeverReactivates(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("requires disposable X server")
	}
	for _, mode := range []string{"close", "cancel", "resize", "transient", "display-loss"} {
		t.Run(mode, func(t *testing.T) {
			observer, err := xgb.NewConn()
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close()
			screen := xproto.Setup(observer).DefaultScreen(observer)
			atom, err := xproto.InternAtom(observer, false, uint16(len(attendedInputOwnerAtom)), attendedInputOwnerAtom).Reply()
			if err != nil {
				t.Fatal(err)
			}
			before, err := xproto.GetSelectionOwner(observer, atom.Atom).Reply()
			if err != nil || before.Owner != 0 {
				t.Fatal("fixture input owner not empty", err)
			}
			spec := desk.RunSpec{Role: "recipient", Binding: desk.Binding{Recipient: "fixture-device", Display: "target"}, Layout: desk.Layout{Displays: []desk.Display{{ID: "target", Device: "fixture-device", PixelWidth: int64(screen.WidthInPixels), PixelHeight: int64(screen.HeightInPixels)}}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			guard, err := newDeskRunGeometryGuard(ctx, spec)
			if err != nil {
				t.Fatal(err)
			}
			defer guard.Close()
			select {
			case <-guard.ctx.Done():
				t.Fatal("exact initial geometry canceled")
			default:
			}
			var restore func()
			switch mode {
			case "close":
				if err = guard.Close(); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancel()
			case "display-loss":
				guard.conn.Close()
			case "resize", "transient":
				restore = deskFixtureResizeSource(t, observer)
				defer restore()
				if mode == "transient" {
					restore()
				}
			}
			select {
			case <-guard.ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("geometry guard did not retire", mode)
			}
			err = guard.Close()
			if (mode == "resize" || mode == "transient" || mode == "display-loss") && (err == nil || !strings.Contains(err.Error(), "reviewed recipient geometry changed")) {
				t.Fatal("geometry loss not preserved", mode, err)
			}
			if (mode == "close" || mode == "cancel") && err != nil {
				t.Fatal("deliberate close relabeled as geometry loss", mode, err)
			}
			if restore != nil {
				restore()
			}
			select {
			case <-guard.done:
			default:
				t.Fatal("closed guard retained event reader")
			}
			after, err := xproto.GetSelectionOwner(observer, atom.Atom).Reply()
			if err != nil || after.Owner != before.Owner {
				t.Fatal("geometry-only guard claimed native input", err)
			}
			// Recheck after restoration: a canceled guard cannot renew its context.
			if guard.ctx.Err() == nil {
				t.Fatal("restored dimensions reactivated old guard")
			}
		})
	}
}
func TestDeskX11ReviewedGeometryGuardRejectsWrongEvidence(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("requires disposable X server")
	}
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	screen := xproto.Setup(observer).DefaultScreen(observer)
	valid := func() desk.RunSpec {
		return desk.RunSpec{Role: "recipient", Binding: desk.Binding{Recipient: "fixture-device", Display: "target"}, Layout: desk.Layout{Displays: []desk.Display{{ID: "target", Device: "fixture-device", PixelWidth: int64(screen.WidthInPixels), PixelHeight: int64(screen.HeightInPixels)}}}}
	}
	for _, change := range []func(*desk.RunSpec){func(s *desk.RunSpec) { s.Role = "primary" }, func(s *desk.RunSpec) { s.Binding.Recipient = "other" }, func(s *desk.RunSpec) { s.Binding.Display = "other" }, func(s *desk.RunSpec) { s.Layout.Displays[0].PixelWidth-- }, func(s *desk.RunSpec) { s.Layout.Displays[0].PixelHeight-- }, func(s *desk.RunSpec) { s.Layout.Displays = nil }} {
		spec := valid()
		change(&spec)
		guard, err := newDeskRunGeometryGuard(context.Background(), spec)
		if err == nil {
			guard.Close()
			t.Fatal("wrong reviewed geometry admitted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, parent := range []context.Context{nil, ctx} {
		if guard, err := newDeskRunGeometryGuard(parent, valid()); err == nil {
			guard.Close()
			t.Fatal("absent owner admitted")
		}
	}
}
