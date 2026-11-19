package desk

import (
	"errors"
	"reflect"
	"sync"
)

// Input is an explicit physical-key/button transition, never logical text,
// clipboard data or a Remote Desktop InputEvent. NativeKey is valid only with
// an independently matched keyboard-map fingerprint in a fresh paired run.
type Input struct {
	Sequence uint64 `json:"sequence"`
	Kind     string `json:"kind"`
	Action   string `json:"action"`
	Code     uint16 `json:"code,omitempty"`
	X        int64  `json:"x,omitempty"`
	Y        int64  `json:"y,omitempty"`
}

// NativeInput owns only the transitions it accepted. Release must join exact
// owned cleanup; an error is terminal, never permission to activate a successor.
type NativeInput interface {
	Apply(Input) error
	Release() error
}

// Recipient is one fresh explicitly accepted pair/layout generation. Creation
// does not authenticate a peer or grant a native backend. The caller must first
// verify exact keys/current authority, local pairing and the keyboard map.
type Recipient struct {
	mu            sync.Mutex
	backend       NativeInput
	live          func() bool
	width, height int64
	sequence      uint64
	retired       bool
	releaseErr    error
	keys          map[uint16]bool
	buttons       map[uint16]bool
}

func NewRecipient(backend NativeInput, width, height int64, live func() bool) (*Recipient, error) {
	if !nativePresent(backend) || live == nil || !live() || width < 1 || height < 1 || width > 65535 || height > 65535 {
		return nil, errors.New("recipient prerequisites unavailable")
	}
	return &Recipient{backend: backend, live: live, width: width, height: height, keys: make(map[uint16]bool), buttons: make(map[uint16]bool)}, nil
}
func (r *Recipient) retireLocked() error {
	if r.retired {
		return r.releaseErr
	}
	r.retired = true
	r.releaseErr = r.backend.Release()
	if r.releaseErr == nil {
		clear(r.keys)
		clear(r.buttons)
	}
	return r.releaseErr
}
func (r *Recipient) Close() error { r.mu.Lock(); defer r.mu.Unlock(); return r.retireLocked() }
func (r *Recipient) Apply(ev Input) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	fail := func(why string) error { return errors.Join(errors.New(why), r.retireLocked()) }
	if r.retired {
		return errors.New("recipient retired")
	}
	if !r.live() {
		return fail("pair authority retired")
	}
	if ev.Sequence == 0 || ev.Sequence <= r.sequence {
		return fail("stale input sequence")
	}
	if ev.Kind == "pointer" {
		if ev.X < 0 || ev.Y < 0 || ev.X >= r.width || ev.Y >= r.height {
			return fail("pointer outside recipient")
		}
		if ev.Action != "move" || ev.Code != 0 {
			return fail("invalid pointer event")
		}
	} else {
		ledger := r.keys
		if ev.Kind == "button" {
			ledger = r.buttons
			if ev.Code < 1 || ev.Code > 5 {
				return fail("invalid button")
			}
		} else if ev.Kind != "key" || ev.Code < 8 || ev.Code > 255 {
			return fail("invalid physical key")
		}
		if ev.X != 0 || ev.Y != 0 {
			return fail("unexpected key/button coordinates")
		}
		switch ev.Action {
		case "down":
			if ledger[ev.Code] {
				return fail("duplicate press")
			}
		case "up":
			if !ledger[ev.Code] {
				return fail("unowned release")
			}
		case "repeat":
			if ev.Kind != "key" || !ledger[ev.Code] {
				return fail("unowned key repeat")
			}
		default:
			return fail("invalid input action")
		}
	}
	if err := r.backend.Apply(ev); err != nil {
		return errors.Join(err, r.retireLocked())
	}
	r.sequence = ev.Sequence
	if ev.Kind == "key" || ev.Kind == "button" {
		ledger := r.keys
		if ev.Kind == "button" {
			ledger = r.buttons
		}
		if ev.Action == "down" {
			ledger[ev.Code] = true
		} else if ev.Action == "up" {
			delete(ledger, ev.Code)
		}
	}
	// The native call cannot be recalled; immediately retire if the exact run
	// lost authority while it completed. Never forward queued input afterward.
	if !r.live() {
		return fail("pair authority retired during input")
	}
	return nil
}
func (r *Recipient) Held() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.keys) != 0 || len(r.buttons) != 0
}

func nativePresent(backend NativeInput) bool {
	return backend != nil && !(reflect.ValueOf(backend).Kind() == reflect.Pointer && reflect.ValueOf(backend).IsNil())
}
