//go:build linux

package main

import (
	"bytes"
	"reflect"
	"testing"

	"fintex.au/mnet/internal/session"
)

// Queue-only model. No X connection, remote session, permission or native
// backend is created. Expected entries are copied independently of slice reuse.
func FuzzDesktopPointerQueueBoundAndWithdrawal(f *testing.F) {
	f.Add([]byte{0, 0, 1, 2, 0, 3, 4, 0, 1, 5})
	f.Add(make([]byte, 80))
	f.Add(bytes.Repeat([]byte{1}, 80))
	f.Add(append(bytes.Repeat([]byte{1}, 64), 3, 0, 5))
	f.Add(append(bytes.Repeat([]byte{2}, 64), 3, 0, 5))
	f.Add([]byte{1, 1, 1, 2, 2, 3, 4, 5})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 512 {
			t.Skip()
		}
		v := &xDesktopView{pointer: true}
		grant := session.ControlStatus{State: "allowed", Scope: session.ControlScopePointer, PointerV1: true, Session: "fixture", Request: "request", Revision: 1}
		var want []desktopPointerAction
		for i, b := range raw {
			if b%6 == 5 {
				got := v.PointerActions()
				if !reflect.DeepEqual(append([]desktopPointerAction(nil), got...), append([]desktopPointerAction(nil), want...)) {
					t.Fatal("drain differs", i, got, want)
				}
				if len(v.PointerActions()) != 0 {
					t.Fatal("drain replayed queued actions")
				}
				want = nil
				continue
			}
			v.pointerGrant = grant
			a := desktopPointerAction{Kind: "input", Grant: grant, Input: session.InputEvent{Kind: "pointer", Action: "move", X: i, Y: int(b)}}
			switch b % 6 {
			case 1:
				a.Input.Action = "click"
				a.Input.Button = "left"
			case 2:
				a.Kind = "request"
				a.Grant = session.ControlStatus{}
			case 3:
				a.Kind = "release"
				a.Grant = session.ControlStatus{}
			case 4:
				grant.Revision++
				v.pointerGrant = grant
				a.Grant = grant
			}
			if a.Kind == "release" {
				keep := make([]desktopPointerAction, 0, len(want))
				for _, old := range want {
					if old.Kind != "input" {
						keep = append(keep, old)
					}
				}
				want = keep
			}
			full := len(want) >= 64
			err := v.queuePointer(a)
			if (err != nil) != full {
				t.Fatal("queue bound differs", i, len(want), err)
			}
			if !full {
				if a.Kind == "request" {
					a.Grant = grant
				}
				replace := a.Kind == "input" && a.Input.Action == "move" && len(want) > 0
				if replace {
					last := want[len(want)-1]
					replace = last.Kind == "input" && last.Input.Action == "move" && last.Grant == a.Grant
				}
				if replace {
					want[len(want)-1] = a
				} else {
					want = append(want, a)
				}
			}
			got := append([]desktopPointerAction(nil), v.pointerActions...)
			expected := append([]desktopPointerAction(nil), want...)
			if !reflect.DeepEqual(got, expected) || len(got) > 64 {
				t.Fatal("queue ordering/grant differs", i, got, expected)
			}
		}
	})
}
