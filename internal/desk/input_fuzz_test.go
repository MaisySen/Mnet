package desk

import (
	"errors"
	"testing"
)

// Only the in-memory fixture is called. No TLS, native input, capture or grant.
// Stateful mutations exercise exact ownership and irreversible retirement.
func FuzzDeskRecipientOwnership(f *testing.F) {
	for _, seed := range [][]byte{{0, 0, 2, 0, 1, 0, 3, 0, 4, 0}, {0, 0, 0, 0}, {3, 0, 9, 1}, {0, 0, 11, 0}, {0, 0, 10, 0}, {0, 0, 8, 0}, {0, 0, 12, 0}} {
		f.Add(seed, false)
		f.Add(seed, true)
	}
	f.Fuzz(func(t *testing.T, raw []byte, failRelease bool) {
		if len(raw) > 256 {
			t.Skip()
		}
		live := true
		backend := &ownedFixture{}
		sentinel := errors.New("fixture release failed")
		if failRelease {
			backend.releaseErr = sentinel
		}
		recipient, err := NewRecipient(backend, 64, 32, func() bool { return live })
		if err != nil {
			t.Fatal(err)
		}
		keys, buttons := map[uint16]bool{}, map[uint16]bool{}
		var sequence uint64
		terminal := false
		held := func() bool { return len(keys)+len(buttons) > 0 }
		for i := 0; i+1 < len(raw); i += 2 {
			action, code := raw[i]%13, raw[i+1]
			ev := Input{Sequence: sequence + uint64(code>>5) + 1}
			valid := true
			nativeFailure, lateRetirement := false, false
			switch action {
			case 0, 1, 2:
				ev.Kind = "key"
				ev.Code = uint16(code%4) + 38
				ev.Action = []string{"down", "up", "repeat"}[action]
				valid = (action == 0 && !keys[ev.Code]) || (action != 0 && keys[ev.Code])
			case 3, 4:
				ev.Kind = "button"
				ev.Code = uint16(code%5) + 1
				ev.Action = []string{"down", "up"}[action-3]
				valid = (action == 3 && !buttons[ev.Code]) || (action == 4 && buttons[ev.Code])
			case 5:
				ev.Kind = "pointer"
				ev.Action = "move"
				ev.X = int64(code % 64)
				ev.Y = int64(code % 32)
			case 6:
				ev.Kind = "pointer"
				ev.Action = "move"
				ev.X = 64
				valid = false
			case 7:
				ev.Kind = "key"
				ev.Action = "down"
				ev.Code = 7
				valid = false
			case 8:
				ev.Kind = "pointer"
				ev.Action = "move"
				ev.Sequence = sequence
				valid = false
			case 9:
				ev.Kind = "pointer"
				ev.Action = "move"
				live = false
				valid = false
			case 10:
				ev.Kind = "pointer"
				ev.Action = "move"
				nativeFailure = true
			case 11:
				ev.Kind = "pointer"
				ev.Action = "move"
				lateRetirement = true
			case 12:
				closeErr := recipient.Close()
				if (closeErr != nil) != failRelease || failRelease && !errors.Is(closeErr, sentinel) {
					t.Fatal("close lost release result", closeErr)
				}
				terminal = true
				if !failRelease {
					clear(keys)
					clear(buttons)
				}
				ev.Kind = "pointer"
				ev.Action = "move"
			}
			before := len(backend.events)
			backend.fail = nativeFailure
			backend.after = nil
			if lateRetirement {
				backend.after = func() { live = false }
			}
			expectedCall := !terminal && valid
			wantError := terminal || !valid || nativeFailure || lateRetirement
			err := recipient.Apply(ev)
			if (err != nil) != wantError {
				t.Fatal("unexpected result", action, err, wantError)
			}
			if wantError && failRelease && !errors.Is(err, sentinel) {
				t.Fatal("refused input lost unconfirmed cleanup", action, err)
			}
			calls := len(backend.events) - before
			if (calls == 1) != expectedCall || calls > 1 {
				t.Fatal("invalid or retired transition reached backend", action, calls, expectedCall)
			}
			if !terminal && valid && !nativeFailure {
				sequence = ev.Sequence
				ledger := keys
				if ev.Kind == "button" {
					ledger = buttons
				}
				if ev.Kind == "key" || ev.Kind == "button" {
					if ev.Action == "down" {
						ledger[ev.Code] = true
					} else if ev.Action == "up" {
						delete(ledger, ev.Code)
					}
				}
			}
			if wantError {
				terminal = true
				if !failRelease {
					clear(keys)
					clear(buttons)
				}
			}
			if recipient.Held() != held() {
				t.Fatal("ownership ledger differs", action, recipient.Held(), held())
			}
			wantReleased := 0
			if terminal {
				wantReleased = 1
			}
			if backend.released != wantReleased {
				t.Fatal("cleanup repeated or missing", backend.released, wantReleased)
			}
		}
		for range 2 {
			err := recipient.Close()
			if (err != nil) != failRelease || failRelease && !errors.Is(err, sentinel) || backend.released != 1 {
				t.Fatal("final cleanup lost/repeated", err, backend.released)
			}
		}
		if !failRelease && recipient.Held() {
			t.Fatal("successful cleanup kept owned input")
		}
	})
}
