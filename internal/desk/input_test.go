package desk

import (
	"errors"
	"sync/atomic"
	"testing"
)

type ownedFixture struct {
	events     []Input
	released   int
	fail       bool
	releaseErr error
	after      func()
}

func (b *ownedFixture) Apply(ev Input) error {
	b.events = append(b.events, ev)
	if b.after != nil {
		b.after()
	}
	if b.fail {
		return errors.New("native refused")
	}
	return nil
}
func (b *ownedFixture) Release() error { b.released++; return b.releaseErr }
func TestRecipientPressReleaseAndTerminalReplay(t *testing.T) {
	b := &ownedFixture{}
	r, err := NewRecipient(b, 1000, 500, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	for i, ev := range []Input{{Kind: "key", Action: "down", Code: 50}, {Kind: "key", Action: "repeat", Code: 50}, {Kind: "button", Action: "down", Code: 1}, {Kind: "pointer", Action: "move", X: 999, Y: 499}, {Kind: "button", Action: "up", Code: 1}, {Kind: "key", Action: "up", Code: 50}} {
		ev.Sequence = uint64(i + 1)
		if err = r.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}
	if r.Held() {
		t.Fatal("released input remained in ledger")
	}
	if err = r.Apply(Input{Sequence: 6, Kind: "key", Action: "down", Code: 38}); err == nil || b.released != 1 {
		t.Fatal("replay did not retire")
	}
	if err = r.Apply(Input{Sequence: 7, Kind: "key", Action: "down", Code: 38}); err == nil || len(b.events) != 6 {
		t.Fatal("retired recipient reused")
	}
	r.Close()
	if b.released != 1 {
		t.Fatal("owned release repeated")
	}
}
func TestRecipientRejectsUnownedAndInvalidBeforeNative(t *testing.T) {
	for _, ev := range []Input{{1, "key", "up", 38, 0, 0}, {1, "key", "repeat", 38, 0, 0}, {1, "pointer", "move", 0, -1, 0}, {1, "pointer", "move", 0, 1000, 0}, {1, "button", "down", 6, 0, 0}, {1, "key", "down", 7, 0, 0}, {1, "text", "down", 38, 0, 0}, {1, "key", "down", 38, 1, 0}} {
		b := &ownedFixture{}
		r, _ := NewRecipient(b, 1000, 500, func() bool { return true })
		if err := r.Apply(ev); err == nil || len(b.events) != 0 || b.released != 1 {
			t.Fatalf("invalid input reached backend %+v", ev)
		}
	}
}
func TestRecipientAuthorityLossReleasesBeforeReturn(t *testing.T) {
	var live atomic.Bool
	live.Store(true)
	b := &ownedFixture{}
	r, _ := NewRecipient(b, 1000, 500, live.Load)
	b.after = func() { live.Store(false) }
	if err := r.Apply(Input{1, "key", "down", 38, 0, 0}); err == nil || b.released != 1 {
		t.Fatal("late native completion kept lost grant")
	}
}

func TestRecipientRetainsReleaseFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		b := &ownedFixture{}
		if failed {
			b.releaseErr = errors.New("release failed")
		}
		r, _ := NewRecipient(b, 100, 100, func() bool { return true })
		if err := r.Apply(Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := r.Close(); (err != nil) != failed {
				t.Fatal("lost cleanup outcome", err)
			}
		}
		if r.Held() != failed || b.released != 1 {
			t.Fatal("ledger did not reflect confirmed cleanup")
		}
	}
}

func TestRecipientRetiredInputPreservesUnconfirmedRelease(t *testing.T) {
	for _, trigger := range []string{"invalid-input", "authority-loss", "native-error", "explicit-close"} {
		t.Run(trigger, func(t *testing.T) {
			sentinel := errors.New("fixture unconfirmed release")
			b := &ownedFixture{releaseErr: sentinel}
			live := true
			r, err := NewRecipient(b, 100, 100, func() bool { return live })
			if err != nil {
				t.Fatal(err)
			}
			if err = r.Apply(Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}); err != nil {
				t.Fatal(err)
			}
			switch trigger {
			case "invalid-input":
				err = r.Apply(Input{Sequence: 2, Kind: "key", Action: "down", Code: 38})
			case "authority-loss":
				live = false
				err = r.Apply(Input{Sequence: 2, Kind: "pointer", Action: "move"})
			case "native-error":
				b.fail = true
				err = r.Apply(Input{Sequence: 2, Kind: "pointer", Action: "move"})
			case "explicit-close":
				err = r.Close()
			}
			if !errors.Is(err, sentinel) {
				t.Fatal("first retirement lost unconfirmed cleanup", err)
			}
			before := len(b.events)
			for range 3 {
				if err = r.Apply(Input{Sequence: 3, Kind: "key", Action: "up", Code: 38}); !errors.Is(err, sentinel) {
					t.Fatal("later refused input hid unconfirmed cleanup", err)
				}
				if err = r.Close(); !errors.Is(err, sentinel) {
					t.Fatal("close hid unconfirmed cleanup", err)
				}
			}
			if len(b.events) != before || b.released != 1 || !r.Held() {
				t.Fatal("retired input retried native work or forgot held ledger", len(b.events), before, b.released, r.Held())
			}
		})
	}
}
