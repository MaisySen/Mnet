//go:build mnet_enrollment_development

package osownership

import "sync"

// Single bridge reader may only decode into this bounded inbox. The resident
// takes at most one frame per turn. Mutex covers memory only, never bridge or
// controller/TUN I/O. Overflow is fatal rather than silently skipping a frame
// sequence. Stop/EOF/error discards queued packets before the next lane turn.
type endpointBridgeInbox struct {
	mu       sync.Mutex
	pending  []endpointFrame
	stop     *endpointFrame
	terminal error
}

func (q *endpointBridgeInbox) push(f endpointFrame) error {
	if _, e := encodeEndpointFrame(f); e != nil {
		q.end(e)
		return e
	}
	f.payload = append([]byte(nil), f.payload...)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.terminal != nil || q.stop != nil {
		return errEndpointStopped
	}
	if f.kind == endpointStop || f.kind == endpointStopped {
		q.stop = &f
		q.pending = nil
		return nil
	}
	if len(q.pending) >= endpointQueue {
		q.pending = nil
		q.terminal = errEndpointBackpressure
		return errEndpointBackpressure
	}
	q.pending = append(q.pending, f)
	return nil
}
func (q *endpointBridgeInbox) end(e error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if e == nil {
		e = errEndpointFrame
	}
	if q.terminal == nil {
		q.terminal = e
	}
	q.pending = nil
}
func (q *endpointBridgeInbox) halted() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.terminal != nil {
		return q.terminal
	}
	if q.stop != nil {
		return errEndpointStopped
	}
	return nil
}
func (q *endpointBridgeInbox) take() (endpointFrame, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.terminal != nil {
		return endpointFrame{}, false, q.terminal
	}
	if q.stop != nil {
		return *q.stop, true, nil
	}
	if len(q.pending) == 0 {
		return endpointFrame{}, false, nil
	}
	f := q.pending[0]
	q.pending = q.pending[1:]
	return f, true, nil
}
func (l *endpointResidentLoop) turnInbox(io endpointLaneIO, q *endpointBridgeInbox) error {
	if q == nil {
		return l.end(io, errEndpointFrame)
	}
	f, ok, e := q.take()
	if e != nil {
		return l.end(io, e)
	}
	if ok {
		if e = l.offer(f); e != nil {
			return l.end(io, e)
		}
	}
	return l.turn(io)
}
