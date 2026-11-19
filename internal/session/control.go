package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
)

const controlKind = "mnet-control-v1"

type ControlStatus struct {
	Session  string `json:"session"`
	Request  string `json:"request,omitempty"`
	Revision uint64 `json:"revision"`
	State    string `json:"state"`
}
type controlMessage struct {
	Kind     string `json:"kind"`
	Session  string `json:"session"`
	Request  string `json:"request,omitempty"`
	Revision uint64 `json:"revision,omitempty"`
	State    string `json:"state,omitempty"`
}

func controlToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
func validControlToken(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 16 && hex.EncodeToString(b) == s
}
func isControlStream(s Stream) bool {
	var m struct {
		Kind string `json:"kind"`
	}
	return json.Unmarshal(s.OpenJSON, &m) == nil && m.Kind == controlKind
}

// HostControl is one connection's local-decision gate. Requests never grant
// access; only a bound local Decide can do so. No actor identity comes from JSON.
type HostControl struct {
	ended     bool
	mu        sync.Mutex
	status    ControlStatus
	mux       *Mux
	stream    uint32
	onRequest func(ControlStatus)
	seen      map[string]bool
}

func NewHostControl(onRequest func(ControlStatus)) *HostControl {
	return &HostControl{status: ControlStatus{Session: controlToken(), State: "unavailable"}, onRequest: onRequest, seen: make(map[string]bool)}
}
func (c *HostControl) Snapshot() ControlStatus { c.mu.Lock(); defer c.mu.Unlock(); return c.status }
func (c *HostControl) InputAllowed(ev InputEvent) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status.State == "allowed" && ev.ControlSession == c.status.Session && ev.ControlRequest == c.status.Request
}
func (c *HostControl) emitLocked() error {
	c.status.Revision++
	if c.mux == nil {
		return errors.New("control unavailable")
	}
	err := c.mux.controlJSON(c.stream, controlMessage{Kind: controlKind, Session: c.status.Session, Request: c.status.Request, State: c.status.State, Revision: c.status.Revision})
	if err != nil {
		c.ended = true
		c.status.State = "unavailable"
		c.mux.Close()
	}
	return err
}
func (c *HostControl) Start(m *Mux) {
	c.mu.Lock()
	if c.ended || c.mux != nil || c.status.Session == "" {
		c.mu.Unlock()
		return
	}
	c.mux = m
	s, err := m.open(PriInteractive, controlMessage{Kind: controlKind, Session: c.status.Session}, false)
	if err != nil {
		c.mu.Unlock()
		m.Close()
		return
	}
	c.stream = s.ID
	c.status.State = "available"
	err = c.emitLocked()
	c.mu.Unlock()
	if err != nil {
		m.Close()
		return
	}
	go c.receive(s)
}
func (c *HostControl) receive(s Stream) {
	defer s.release()
	defer s.Mux.Close()
	defer c.End()
	for {
		f, err := s.Recv(0)
		if err != nil {
			return
		}
		var msg controlMessage
		if json.Unmarshal(f.Payload, &msg) != nil || msg.Kind != controlKind {
			return
		}
		c.mu.Lock()
		if c.ended || msg.Session != c.status.Session || !validControlToken(msg.Request) {
			c.mu.Unlock()
			continue
		}
		notify := false
		switch msg.State {
		case "request":
			if c.status.State == "pending" || c.status.State == "allowed" || c.status.State == "paused" {
				c.mu.Unlock()
				continue
			}
			// A consumed request cannot be replayed after its decline/revocation.
			if c.seen[msg.Request] {
				c.mu.Unlock()
				continue
			}
			if len(c.seen) >= 256 {
				c.mu.Unlock()
				return
			}
			c.seen[msg.Request] = true
			c.status.Request = msg.Request
			c.status.State = "pending"
			notify = true
		case "cancel":
			if msg.Request != c.status.Request || (c.status.State != "pending" && c.status.State != "allowed" && c.status.State != "paused") {
				c.mu.Unlock()
				continue
			}
			c.status.State = "declined"
			notify = true
		default:
			c.mu.Unlock()
			continue
		}
		err = c.emitLocked()
		snapshot := c.status
		callback := c.onRequest
		c.mu.Unlock()
		if err != nil {
			return
		}
		if notify && callback != nil {
			callback(snapshot)
		}
	}
}
func (c *HostControl) Decide(sessionID, request string, allow bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended || sessionID != c.status.Session || request == "" || request != c.status.Request || (c.status.State != "pending" && c.status.State != "paused") {
		return errors.New("stale control decision")
	}
	if allow {
		c.status.State = "allowed"
	} else {
		c.status.State = "declined"
	}
	return c.emitLocked()
}
func (c *HostControl) Pause(sessionID, request string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended || sessionID != c.status.Session || request != c.status.Request || c.status.State != "allowed" {
		return errors.New("control is not granted")
	}
	c.status.State = "paused"
	return c.emitLocked()
}
func (c *HostControl) Revoke(sessionID, request string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended || sessionID != c.status.Session || request == "" || request != c.status.Request {
		return errors.New("stale control revocation")
	}
	c.status.State = "revoked"
	return c.emitLocked()
}
func (c *HostControl) End() {
	c.mu.Lock()
	c.ended = true
	c.status.State = "unavailable"
	c.mu.Unlock()
}

// ViewerControl accepts only host-originated status on the dedicated stream.
// It never treats a viewer request, a chat message or a local UI label as a grant.
type ViewerControl struct {
	ended         bool
	notifyMu      sync.Mutex
	notifying     bool // one callback drain, never a lock across app code
	notifyPending bool // coalesce to the current snapshot, not captured states
	mu            sync.Mutex
	status        ControlStatus
	mux           *Mux
	stream        uint32
	attached      bool
	onStatus      func(ControlStatus)
}

func NewViewerControl(callback func(ControlStatus)) *ViewerControl {
	return &ViewerControl{status: ControlStatus{State: "unavailable"}, onStatus: callback}
}
func (c *ViewerControl) Snapshot() ControlStatus { c.mu.Lock(); defer c.mu.Unlock(); return c.status }
func (c *ViewerControl) notify() {
	c.notifyMu.Lock()
	c.notifyPending = true
	if c.notifying {
		c.notifyMu.Unlock()
		return
	}
	c.notifying = true
	for c.notifyPending {
		c.notifyPending = false
		c.notifyMu.Unlock()
		// Reentrant Cancel or End queues the latest status for this same drain.
		// Never wait for app code while holding a notification or state lock.
		if c.onStatus != nil {
			c.onStatus(c.Snapshot())
		}
		c.notifyMu.Lock()
	}
	c.notifying = false
	c.notifyMu.Unlock()
}
func (c *ViewerControl) Handle(s Stream) {
	c.mu.Lock()
	var open controlMessage
	if c.ended || c.attached || json.Unmarshal(s.OpenJSON, &open) != nil || !validControlToken(open.Session) {
		c.mu.Unlock()
		s.Mux.Close()
		return
	}
	c.attached = true
	c.mux = s.Mux
	c.stream = s.ID
	c.status.Session = open.Session
	c.mu.Unlock()
	defer s.release()
	defer s.Mux.Close()
	defer c.End()
	for {
		f, err := s.Recv(0)
		if err != nil {
			return
		}
		var msg controlMessage
		if json.Unmarshal(f.Payload, &msg) != nil || msg.Kind != controlKind {
			return
		}
		c.mu.Lock()
		if c.ended || msg.Session != c.status.Session || msg.Revision <= c.status.Revision {
			c.mu.Unlock()
			continue
		}
		switch msg.State {
		case "available", "pending", "allowed", "paused", "declined", "revoked":
		default:
			c.mu.Unlock()
			return
		}
		// Every grant/revoke is bound to our latest request. An unsolicited grant
		// cannot arm viewer input, even from a peer implementing the protocol badly.
		if msg.State != "available" && (msg.Request == "" || msg.Request != c.status.Request) {
			c.mu.Unlock()
			continue
		}
		if msg.State == "available" && c.status.Request != "" {
			c.mu.Unlock()
			continue
		}
		if (msg.State == "allowed" || msg.State == "paused" || msg.State == "pending") && (c.status.State == "declined" || c.status.State == "revoked") {
			c.mu.Unlock()
			continue
		}
		c.status.State = msg.State
		c.status.Revision = msg.Revision
		c.mu.Unlock()
		c.notify()
	}
}
func (c *ViewerControl) Request() error {
	c.mu.Lock()
	if c.ended || !c.attached || c.status.State == "unavailable" || c.status.State == "pending" || c.status.State == "allowed" || c.status.State == "paused" {
		c.mu.Unlock()
		return errors.New("control request unavailable")
	}
	token := controlToken()
	if token == "" {
		c.mu.Unlock()
		return errors.New("control request unavailable")
	}
	c.status.Request = token
	c.status.State = "pending"
	err := c.mux.controlJSON(c.stream, controlMessage{Kind: controlKind, Session: c.status.Session, Request: token, State: "request"})
	c.mu.Unlock()
	c.notify()
	if err != nil {
		c.End()
		c.mux.Close()
	}
	return err
}
func (c *ViewerControl) Cancel() error {
	c.mu.Lock()
	if c.status.State != "pending" {
		c.mu.Unlock()
		return errors.New("no pending control request")
	}
	c.status.State = "declined"
	err := c.mux.controlJSON(c.stream, controlMessage{Kind: controlKind, Session: c.status.Session, Request: c.status.Request, State: "cancel"})
	c.mu.Unlock()
	c.notify()
	if err != nil {
		c.End()
		c.mux.Close()
	}
	return err
}
func (c *ViewerControl) BindInput(ev *InputEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.State != "allowed" {
		return errors.New("control is not granted")
	}
	ev.ControlSession = c.status.Session
	ev.ControlRequest = c.status.Request
	return nil
}
func (c *ViewerControl) End() {
	c.mu.Lock()
	if c.ended {
		c.mu.Unlock()
		return
	}
	c.ended = true
	c.status.State = "unavailable"
	c.mu.Unlock()
	c.notify()
}

// Withdraw is explicit local cancellation of this pending/allowed/paused
// request. It is not a disconnect and never carries approval to a new request.
func (c *ViewerControl) Withdraw() error {
	c.mu.Lock()
	if c.ended || (c.status.State != "pending" && c.status.State != "allowed" && c.status.State != "paused") {
		c.mu.Unlock()
		return errors.New("no active control request")
	}
	c.status.State = "declined"
	err := c.mux.controlJSON(c.stream, controlMessage{Kind: controlKind, Session: c.status.Session, Request: c.status.Request, State: "cancel"})
	c.mu.Unlock()
	c.notify()
	if err != nil {
		c.End()
		c.mux.Close()
	}
	return err
}
func (c *ViewerControl) bindInputFor(expected ControlStatus, ev *InputEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended || c.status.State != "allowed" || c.status != expected {
		return errors.New("exact control revision no longer granted")
	}
	ev.ControlSession = c.status.Session
	ev.ControlRequest = c.status.Request
	return nil
}
