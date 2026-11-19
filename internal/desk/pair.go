package desk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
)

// Binding is independently accepted by each endpoint, never learned from an
// untrusted hello. Keys identify exact devices in the caller's authenticated
// transport. This package cannot turn a label/IP, network membership, or a
// Remote Desktop grant into Shared Desk consent.
type Binding struct {
	Run       string `json:"run"`
	Primary   string `json:"primary_key"`
	Recipient string `json:"recipient_key"`
	Layout    string `json:"layout_hash"`
	Keyboard  string `json:"keyboard_hash"`
	Display   string `json:"display"`
}

func digest(s string) bool {
	v, e := hex.DecodeString(s)
	return e == nil && len(v) == 32 && hex.EncodeToString(v) == s
}
func (c *Compiled) Fingerprint() string {
	// Validated immutable display order is part of the approved layout. Reordering
	// conservatively creates a new generation rather than silently reusing one.
	data, _ := json.Marshal(Layout{Version: 1, Displays: c.displays})
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
func (b Binding) Validate(c *Compiled) error {
	if c == nil || !digest(b.Run) || !digest(b.Layout) || !digest(b.Keyboard) || b.Primary == "" || b.Recipient == "" || b.Primary == b.Recipient || len(b.Primary) > 256 || len(b.Recipient) > 256 || b.Layout != c.Fingerprint() {
		return errors.New("exact desk binding unavailable")
	}
	for _, d := range c.displays {
		if d.ID == b.Display && d.Device == b.Recipient {
			return nil
		}
	}
	return errors.New("desk binding target absent from layout")
}

// Switchboard is one local primary's explicit generation. Backend factories
// must authenticate the exact Binding, require current local pair consent,
// and complete their native readiness check. It never captures input itself.
// First-slice policy: do not cross edges with any held key/button (no drags).
type Switchboard struct {
	mu             sync.Mutex
	layout         *Compiled
	primary        string
	current        string
	x, y           int64
	live           func() bool
	recipient      *Recipient
	stopped        atomic.Bool
	cleanupErr     error
	done           chan struct{}
	disableCapture func()
}

func NewSwitchboard(layout *Compiled, primary, localDisplay string, live func() bool, disableCapture func()) (*Switchboard, error) {
	if layout == nil || live == nil || !live() || disableCapture == nil {
		return nil, errors.New("desk primary prerequisites unavailable")
	}
	for _, d := range layout.displays {
		if d.ID == localDisplay && d.Device == primary {
			return &Switchboard{layout: layout, primary: primary, current: localDisplay, live: live, disableCapture: disableCapture, done: make(chan struct{})}, nil
		}
	}
	return nil, errors.New("local display absent from approved layout")
}
func (s *Switchboard) retireLocked() error {
	if s.recipient != nil {
		s.cleanupErr = errors.Join(s.cleanupErr, s.recipient.Close())
		s.recipient = nil
	}
	return s.cleanupErr
}

// Stop disables forwarding before waiting for any in-flight native operation or
// remote release. Failure to confirm release permanently blocks this board.
func (s *Switchboard) Stop() error {
	s.stopCapture()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retireLocked()
}
func (s *Switchboard) Apply(ev Input) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped.Load() || !s.live() {
		s.stopCapture()
		return errors.Join(errors.New("desk primary retired"), s.retireLocked())
	}
	if s.recipient == nil {
		return errors.New("desk active locally; do not forward")
	}
	if err := s.recipient.Apply(ev); err != nil {
		s.stopCapture()
		return errors.Join(err, s.retireLocked())
	}
	if ev.Kind == "pointer" {
		s.x, s.y = ev.X, ev.Y
	}
	return nil
}

// Cross has no auto-accept or fallback. A factory is called only after the exact
// local binding agrees with geometry and the old destination confirms release.
// It cannot restart a stopped generation. Callers own source capture/reclaim.
func (s *Switchboard) Cross(edge Edge, along int64, binding Binding, create func(Binding) (NativeInput, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped.Load() || !s.live() || s.cleanupErr != nil {
		s.stopCapture()
		return errors.Join(errors.New("desk primary retired"), s.retireLocked())
	}
	if s.recipient != nil && s.recipient.Held() {
		return errors.New("desk crossing blocked while input held")
	}
	candidate, found, err := s.layout.MapEdge(s.current, edge, along)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("no adjacent desk at crossing")
	}
	if candidate.Device == s.primary {
		if err = s.retireLocked(); err != nil {
			s.stopCapture()
			return err
		}
		s.current = candidate.Display
		s.x, s.y = candidate.X, candidate.Y
		return nil
	}
	if err = binding.Validate(s.layout); err != nil {
		return err
	}
	if binding.Primary != s.primary || binding.Recipient != candidate.Device || binding.Display != candidate.Display || create == nil {
		return errors.New("desk pair differs from crossing")
	}
	if err = s.retireLocked(); err != nil {
		s.stopCapture()
		return err
	}
	backend, err := create(binding)
	if err != nil {
		if nativePresent(backend) {
			s.cleanupErr = errors.Join(s.cleanupErr, backend.Release())
		}
		s.stopCapture()
		return errors.Join(err, s.cleanupErr)
	}
	var target Display
	for _, d := range s.layout.displays {
		if d.ID == candidate.Display {
			target = d
		}
	}
	recipient, err := NewRecipient(backend, target.PixelWidth, target.PixelHeight, func() bool { return !s.stopped.Load() && s.live() })
	if err != nil {
		if nativePresent(backend) {
			s.cleanupErr = errors.Join(s.cleanupErr, backend.Release())
			err = errors.Join(err, s.cleanupErr)
		}
		s.stopCapture()
		return err
	}
	s.recipient = recipient
	if terminal := nativeTerminal(backend); terminal != nil {
		go func() {
			select {
			case <-s.done:
				return
			case <-terminal:
				s.backendRetired(recipient)
			}
		}()
	}

	// Entry positioning belongs to the new destination and is never replayed
	// from a previous generation. External capture sequences start after 1.
	if err = recipient.Apply(Input{Sequence: 1, Kind: "pointer", Action: "move", X: candidate.X, Y: candidate.Y}); err != nil {
		s.stopCapture()
		return errors.Join(err, s.retireLocked())
	}
	s.current = candidate.Display
	s.x, s.y = candidate.X, candidate.Y
	return nil
}

// Destination is an observed routing snapshot, not a UI activation request.
func (s *Switchboard) Destination() (Display, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.layout.displays {
		if d.ID == s.current {
			return d, !s.stopped.Load() && s.recipient != nil
		}
	}
	return Display{}, false
}

func (s *Switchboard) Position() (int64, int64) { s.mu.Lock(); defer s.mu.Unlock(); return s.x, s.y }

func (s *Switchboard) stopCapture() {
	if s.stopped.CompareAndSwap(false, true) {
		close(s.done)
		s.disableCapture()
	}
}

func (s *Switchboard) Stopped() <-chan struct{} { return s.done }

func (s *Switchboard) backendRetired(recipient *Recipient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recipient == recipient {
		s.stopCapture()
		s.retireLocked()
	}
}

// PrimaryDisplay describes the first native slice's single local display.
// Geometry can express more; the adapter must refuse unsupported layouts.
func (s *Switchboard) PrimaryDisplay() (Display, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var local Display
	count := 0
	for _, d := range s.layout.displays {
		if d.Device == s.primary {
			local = d
			count++
		}
	}
	if count != 1 {
		return Display{}, errors.New("native primary requires exactly one local display")
	}
	return local, nil
}

// ReturnLocalEdge is a terminal return, not activation of another pair. Held
// input blocks crossing. Disable local forwarding before awaiting recipient
// release; only a confirmed release permits mapped local pointer placement.
func (s *Switchboard) ReturnLocalEdge(edge Edge, along int64) (Candidate, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped.Load() || !s.live() {
		s.stopCapture()
		return Candidate{}, false, errors.Join(errors.New("desk primary retired"), s.retireLocked())
	}
	if s.recipient == nil || s.recipient.Held() {
		return Candidate{}, false, nil
	}
	candidate, found, err := s.layout.MapEdge(s.current, edge, along)
	if err != nil {
		return Candidate{}, false, err
	}
	if !found || candidate.Device != s.primary {
		return Candidate{}, false, nil
	}
	s.stopCapture()
	if err = s.retireLocked(); err != nil {
		return Candidate{}, false, err
	}
	s.current = candidate.Display
	s.x, s.y = candidate.X, candidate.Y
	return candidate, true, nil
}
