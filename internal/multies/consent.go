package multies

// AttendedConsent is an inactive consent state machine for the proposed
// consumer flow. It grants no identity or transport authority. Callers must
// first verify current Multies grants, exact actors and OS permissions; no
// production caller or session adapter is installed here.

import (
	"sync"
	"time"
)

const MaxConsentLifetime = 10 * time.Minute

type SessionBinding struct {
	WorkspaceID     string
	HostUserID      string
	ViewerUserID    string
	HostDeviceKey   string
	ViewerDeviceKey string
}

type ConsentState string

const (
	WaitingForView    ConsentState = "waiting-view"
	Viewing           ConsentState = "viewing"
	WaitingForControl ConsentState = "waiting-control"
	Controlling       ConsentState = "controlling"
	ConsentEnded      ConsentState = "ended"
)

type ConsentSnapshot struct {
	State               ConsentState
	CanView, CanControl bool
}

type AttendedConsent struct {
	mu         sync.Mutex
	binding    SessionBinding
	state      ConsentState
	generation uint64
	expires    time.Time
	now        func() time.Time
}

func validBinding(b SessionBinding) bool {
	return idPattern.MatchString(b.WorkspaceID) && idPattern.MatchString(b.HostUserID) && idPattern.MatchString(b.ViewerUserID) && validDeviceKey(b.HostDeviceKey) && validDeviceKey(b.ViewerDeviceKey) && b.HostDeviceKey != b.ViewerDeviceKey
}
func NewAttendedConsent(b SessionBinding, expires time.Time, now func() time.Time) (*AttendedConsent, error) {
	if now == nil {
		now = time.Now
	}
	at := now()
	if !validBinding(b) || !expires.After(at) || expires.After(at.Add(MaxConsentLifetime)) {
		return nil, ErrDenied
	}
	return &AttendedConsent{binding: b, state: WaitingForView, expires: expires, now: now}, nil
}
func (c *AttendedConsent) alive() bool {
	if c.now == nil || !c.expires.After(c.now()) || !validBinding(c.binding) {
		c.state = ConsentEnded
		return false
	}
	return c.state != ConsentEnded
}
func (c *AttendedConsent) snapshot() ConsentSnapshot {
	switch c.state {
	case Viewing, WaitingForControl:
		return ConsentSnapshot{State: c.state, CanView: true}
	case Controlling:
		return ConsentSnapshot{State: c.state, CanView: true, CanControl: true}
	case WaitingForView:
		return ConsentSnapshot{State: c.state}
	default:
		return ConsentSnapshot{State: ConsentEnded}
	}
}
func (c *AttendedConsent) Snapshot() ConsentSnapshot {
	if c == nil {
		return ConsentSnapshot{State: ConsentEnded}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alive()
	return c.snapshot()
}

// DecideView is accepted only once from the exact host for this binding.
func (c *AttendedConsent) DecideView(b SessionBinding, host string, allow bool) error {
	if c == nil {
		return ErrDenied
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.alive() || b != c.binding || host != c.binding.HostUserID || c.state != WaitingForView {
		return ErrDenied
	}
	if allow {
		c.state = Viewing
	} else {
		c.state = ConsentEnded
	}
	return nil
}

// RequestControl is a viewer request, never input authority. While pending,
// viewing can continue. A second request cannot replace the pending decision.
func (c *AttendedConsent) RequestControl(b SessionBinding, viewer string) (uint64, error) {
	if c == nil {
		return 0, ErrDenied
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.alive() || b != c.binding || viewer != c.binding.ViewerUserID || c.state != Viewing {
		return 0, ErrDenied
	}
	if c.generation == ^uint64(0) {
		c.state = ConsentEnded
		return 0, ErrDenied
	}
	c.generation++
	c.state = WaitingForControl
	return c.generation, nil
}
func (c *AttendedConsent) DecideControl(b SessionBinding, host string, generation uint64, allow bool) error {
	if c == nil {
		return ErrDenied
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.alive() || b != c.binding || host != c.binding.HostUserID || generation != c.generation || generation == 0 || c.state != WaitingForControl {
		return ErrDenied
	}
	if allow {
		c.state = Controlling
	} else {
		c.state = Viewing
	}
	return nil
}

// RevokeControl drops input authority and cancels any pending decision. A late
// approval cannot restore it without a new explicit request from the viewer.
func (c *AttendedConsent) RevokeControl(b SessionBinding, host string) error {
	if c == nil {
		return ErrDenied
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.alive() || b != c.binding || host != c.binding.HostUserID || (c.state != Controlling && c.state != WaitingForControl) {
		return ErrDenied
	}
	c.state = Viewing
	return nil
}

// End is used by local cancellation, disconnect, membership loss, OS permission
// loss or either person's stop. An ended consent is never reconnectable. A new
// session requires a new instance and both consent decisions again.
func (c *AttendedConsent) End() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.state = ConsentEnded
	c.mu.Unlock()
}
