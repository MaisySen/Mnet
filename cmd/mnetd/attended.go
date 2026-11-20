package main

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"fintex.au/mnet/internal/session"
)

// The UI is local-only. A peer can request a prompt, never answer one.
// Each Ask owns and joins its process; cancellation means denial/end.
type attendedPrompt struct{ Kind, Peer, Scope string }
type attendedUI interface {
	Ask(context.Context, attendedPrompt) (string, error)
}

func attendedPeerLabel(name string) string {
	var out []rune
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		out = append(out, r)
		if len(out) == 64 {
			break
		}
	}
	if len(out) == 0 {
		return "Verified mesh peer"
	}
	return string(out)
}
func validateAttendedOptions(o options) error {
	if !o.sessAttended {
		return nil
	}
	if !o.sessHost || o.sessAuto || o.sessRec != "" || o.sessProof || o.serviceAction != "" || isServiceSession() {
		return errors.New("attended hosting requires -session-host in a desktop console, without auto-accept, recording, proof or service mode")
	}
	return nil
}

// Only a checked local backend may return this when authority ended during
// dispatch. It is not proof of release; its owner must still join Close.
var errAttendedInputRetired = errors.New("local input authority retired")

// Serialize input backend ownership with grant transitions. In particular a
// paused/revoked host releases its owned pressed keys/buttons before prompting.
type attendedInput struct {
	mu              sync.Mutex
	gate            *session.HostControl
	backend         session.Injector
	create          func() (session.Injector, error)
	fail            func()
	failed          bool
	live            func() bool
	backendRetired  chan struct{}
	backendWatchers sync.WaitGroup
}

func (i *attendedInput) Inject(ev session.InputEvent) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.backend == nil || (i.live != nil && !i.live()) || !i.gate.InputAllowed(ev) {
		return nil
	}
	var err error
	if checked, ok := i.backend.(interface {
		InjectAuthorized(session.InputEvent, func() bool) error
	}); ok {
		err = checked.InjectAuthorized(ev, func() bool { return (i.live == nil || i.live()) && i.gate.InputAllowed(ev) })
	} else {
		err = i.backend.Inject(ev)
	}
	if err != nil {
		// A revoked in-flight event is not a device failure. It may already have
		// completed in the child, so do not report success or send another event.
		// Retire and join this owner first; Close failure still ends the session.
		if errors.Is(err, errAttendedInputRetired) && ((i.live != nil && !i.live()) || !i.gate.InputAllowed(ev)) {
			i.closeLocked()
			return err
		}
		i.failed = true
		i.gate.End()
		i.closeLocked()
	}
	return err
}
func (i *attendedInput) closeLocked() {
	if i.backendRetired != nil {
		close(i.backendRetired)
		i.backendRetired = nil
	}
	if c, ok := i.backend.(io.Closer); ok {
		if c.Close() != nil {
			i.failed = true
		}
	}
	i.backend = nil
	if i.failed {
		i.gate.End()
		if i.fail != nil {
			i.fail()
		}
	}
}
func (i *attendedInput) Close() error {
	i.mu.Lock()
	i.gate.End()
	i.closeLocked()
	i.mu.Unlock()
	i.backendWatchers.Wait()
	return nil
}

// A termination report belongs to exactly this backend lifetime. Retire the
// watcher before normal Close so old-process exit cannot end a resumed grant.
func (i *attendedInput) watchLocked(backend session.Injector) {
	lifetime, ok := backend.(interface{ Stopped() <-chan struct{} })
	if !ok {
		return
	}
	retired := make(chan struct{})
	i.backendRetired = retired
	i.backendWatchers.Add(1)
	go func() {
		defer i.backendWatchers.Done()
		select {
		case <-retired:
			return
		case <-lifetime.Stopped():
		}
		i.mu.Lock()
		defer i.mu.Unlock()
		if i.backendRetired != retired {
			return
		}
		i.failed = true
		i.gate.End()
		i.closeLocked()
	}()
}
func (i *attendedInput) decide(s session.ControlStatus, allow bool) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	current := i.gate.Snapshot()
	if i.failed || (i.live != nil && !i.live()) || current != s || (s.State != "pending" && s.State != "paused") {
		return errors.New("retired local decision")
	}
	i.closeLocked()
	if i.failed {
		return errors.New("local input cleanup failed")
	}
	if allow {
		backend, err := i.create()
		if err != nil || backend == nil {
			if c, ok := backend.(io.Closer); ok {
				if c.Close() != nil {
					i.failed = true
					i.gate.End()
					if i.fail != nil {
						i.fail()
					}
				}
			}
			_ = i.gate.Decide(s.Session, s.Request, false)
			return errors.New("local input unavailable")
		}
		i.backend = backend
		i.watchLocked(backend)
	}
	if i.live != nil && !i.live() {
		i.closeLocked()
		return errors.New("retired local owner")
	}
	if err := i.gate.Decide(s.Session, s.Request, allow); err != nil {
		i.closeLocked()
		return err
	}
	return nil
}
func (i *attendedInput) pause() (session.ControlStatus, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	s := i.gate.Snapshot()
	err := i.gate.Pause(s.Session, s.Request)
	if err != nil {
		return s, err
	}
	i.closeLocked()
	return i.gate.Snapshot(), nil
}
func (i *attendedInput) revoke() {
	i.mu.Lock()
	defer i.mu.Unlock()
	s := i.gate.Snapshot()
	_ = i.gate.Revoke(s.Session, s.Request)
	i.closeLocked()
}

// Serve only after view approval and a fresh authorization check at admission.
// No file destination/approval or recording is configured in this mode.
func serveAttendedSession(parent context.Context, m *session.Mux, peer string, negotiate bool, ui attendedUI, capture session.Capturer, createInput func() (session.Injector, error), authorized func() bool) (*session.Host, <-chan struct{}) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	requests := make(chan session.ControlStatus, 1)
	var offerMu sync.Mutex
	offer := func(s session.ControlStatus) {
		offerMu.Lock()
		defer offerMu.Unlock()
		select {
		case <-requests:
		default:
		}
		select {
		case requests <- s:
		default:
		}
	}
	var input *attendedInput
	gate := session.NewHostControlWithPointerScope(func(s session.ControlStatus) {
		if s.State == "pending" {
			offer(s)
		} else {
			input.mu.Lock()
			input.closeLocked()
			input.mu.Unlock()
		}
	})
	live := func() bool {
		if ctx.Err() != nil {
			return false
		}
		select {
		case <-m.Closed():
			return false
		default:
		}
		return authorized == nil || authorized()
	}
	createOwnedInput := func() (session.Injector, error) {
		backend, err := createInput()
		if err != nil || backend == nil {
			return backend, err
		}
		if geometry, ok := backend.(interface{ CheckCaptureGeometry(int, int) error }); ok {
			w, h := capture.Geometry()
			if err = geometry.CheckCaptureGeometry(w, h); err != nil {
				return backend, err // the grant owner joins/reports cleanup failure
			}
		}
		return backend, nil
	}
	input = &attendedInput{gate: gate, create: createOwnedInput, live: live, fail: func() { m.Close() }}
	h := session.ServeHostWithOptions(m, capture, input, 8, session.HostOptions{Control: gate, NegotiateControl: negotiate, Authorized: authorized})
	peer = attendedPeerLabel(peer)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			action, err := ui.Ask(ctx, attendedPrompt{Kind: "active", Peer: peer})
			if err != nil || ctx.Err() != nil || action == "end" {
				m.Close()
				return
			}
			switch action {
			case "pause":
				s, err := input.pause()
				if err == nil {
					offer(s)
				}
			case "revoke":
				input.revoke()
			default:
				m.Close()
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case s := <-requests:
				if gate.Snapshot() != s {
					continue
				}
				decisionCtx, stop := context.WithTimeout(ctx, 30*time.Second)
				watched := make(chan struct{})
				go func() {
					defer close(watched)
					tick := time.NewTicker(50 * time.Millisecond)
					defer tick.Stop()
					for {
						select {
						case <-decisionCtx.Done():
							return
						case <-tick.C:
							if gate.Snapshot() != s {
								stop()
								return
							}
						}
					}
				}()
				action, err := ui.Ask(decisionCtx, attendedPrompt{Kind: "control", Peer: peer, Scope: s.Scope})
				allow := err == nil && decisionCtx.Err() == nil && action == "allow"
				stop()
				<-watched
				_ = input.decide(s, allow)
			}
		}
	}()
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			m.Close()
		case <-m.Closed():
		}
		cancel()
		_ = input.Close()
		wg.Wait()
		h.Stop()
	}()
	return h, done
}

func approveAttendedView(ctx context.Context, ui attendedUI, peer string) bool {
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	action, err := ui.Ask(bounded, attendedPrompt{Kind: "view", Peer: attendedPeerLabel(peer)})
	return err == nil && bounded.Err() == nil && strings.EqualFold(action, "allow")
}

// Called only after current peer/key and allow-list matching. Revalidate after
// the person's answer as well; a pending dialog cannot preserve withdrawn access.
func approveAttendedConnection(ctx context.Context, ui attendedUI, peer string, conn net.Conn, authorized func() bool) bool {
	if ctx.Err() != nil || authorized == nil || !authorized() {
		return false
	}
	if conn.SetDeadline(time.Now().Add(50*time.Second)) != nil {
		return false
	}
	// No valid next message exists before HelloReply. Observe only this
	// pending phase: EOF, any pipelined byte or lost current authority retires
	// the local prompt. Join the sole reader before the mux can start reading.
	pending, cancel := context.WithCancel(ctx)
	defer cancel()
	stop, done := make(chan struct{}), make(chan struct{})
	var retired atomic.Bool
	go func() {
		defer close(done)
		var one [1]byte
		for {
			select {
			case <-stop:
				return
			case <-pending.Done():
				retired.Store(true)
				return
			default:
			}
			if !authorized() {
				retired.Store(true)
				cancel()
				return
			}
			if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
				retired.Store(true)
				cancel()
				return
			}
			n, err := conn.Read(one[:])
			if n > 0 || err == nil {
				retired.Store(true)
				cancel()
				return
			}
			if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
				retired.Store(true)
				cancel()
				return
			}
		}
	}()
	allowed := approveAttendedView(pending, ui, peer)
	close(stop)
	_ = conn.SetReadDeadline(time.Now())
	<-done
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return false
	}
	return allowed && !retired.Load() && ctx.Err() == nil && authorized()
}
