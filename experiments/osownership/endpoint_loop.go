//go:build mnet_enrollment_development

package osownership

import "errors"

func endpointTerminalFirst(frames []endpointFrame) ([]endpointFrame, bool) {
	for _, f := range frames {
		if f.kind == endpointStop || f.kind == endpointStopped {
			return []endpointFrame{f}, true
		}
	}
	return frames, false
}

var errEndpointCleanupUnverified = errors.New("endpoint-original-close-and-absence-not-acknowledged")
var errEndpointNoProgress = errors.New("endpoint-nonblocking-no-progress")
var errEndpointPacketDrop = errors.New("endpoint-kernel-packet-drop")
var errEndpointStopped = errors.New("endpoint-stop-requested")
var errEndpointInputClosed = errors.New("endpoint-input-closed-without-decoded-stop")

// Resident lane only, no worker accesses these counters or the reactor. Bridge
// workers may only fill/take bounded queues; they never call turn or TUN I/O.
type endpointLoopCounts struct{ Turns, Read, Written, PolicyDrops, KernelDrops, QueueDrops, Yields, Fatal uint64 }

func endpointCount(v *uint64) {
	if *v != ^uint64(0) {
		*v++
	}
}

type endpointLaneIO interface {
	drainOne() error
	receiveOne(endpointFrame) error
	stopOwned() error
}
type endpointResidentLoop struct {
	inbound              []endpointFrame
	stopPending, stopped bool
	counts               endpointLoopCounts
}

func (l *endpointResidentLoop) offer(f endpointFrame) error {
	if l.stopped {
		return errEndpointStopped
	}
	if _, err := encodeEndpointFrame(f); err != nil {
		l.stopPending = true
		l.inbound = nil
		return err
	}
	// Stop takes priority even when queue is full. Ordered frame validation must
	// happen in the decoder/sequence owner before offer; this is not admission.
	if f.kind == endpointStop || f.kind == endpointStopped {
		l.stopPending = true
		l.inbound = nil
		return nil
	}
	if len(l.inbound) >= endpointQueue {
		return errEndpointBackpressure
	}
	f.payload = append([]byte(nil), f.payload...)
	l.inbound = append(l.inbound, f)
	return nil
}
func (l *endpointResidentLoop) stop(io endpointLaneIO, cause error) error {
	l.stopped = true
	l.inbound = nil
	return errors.Join(cause, io.stopOwned())
}

// One bounded turn: at most one inbound write then one read. The actual driver
// must run turn on the same held resident TID; no timer, spin loop or goroutine
// is started here. Stop checked before each direction. EOF calls end explicitly.
func (l *endpointResidentLoop) turn(io endpointLaneIO) error {
	if io == nil {
		return errEndpointFrame
	}
	if l.stopped {
		return errEndpointStopped
	}
	if l.stopPending {
		return l.stop(io, errEndpointStopped)
	}
	endpointCount(&l.counts.Turns)
	handle := func(e error) error {
		if e == nil {
			return nil
		}
		var refusal packetRefusal
		switch {
		case errors.As(e, &refusal):
			endpointCount(&l.counts.PolicyDrops)
		case errors.Is(e, errEndpointPacketDrop):
			endpointCount(&l.counts.KernelDrops)
		case errors.Is(e, errEndpointBackpressure):
			endpointCount(&l.counts.QueueDrops)
		case errors.Is(e, errEndpointNoProgress):
			endpointCount(&l.counts.Yields)
		default:
			endpointCount(&l.counts.Fatal)
			return l.stop(io, e)
		}
		return nil
	}
	if len(l.inbound) > 0 {
		f := l.inbound[0]
		l.inbound = l.inbound[1:]
		e := io.receiveOne(f)
		if e == nil && f.kind == endpointDatagram {
			endpointCount(&l.counts.Written)
		}
		if e = handle(e); e != nil {
			return e
		}
	}
	if l.stopPending {
		return l.stop(io, errEndpointStopped)
	}
	e := io.drainOne()
	if e == nil {
		endpointCount(&l.counts.Read)
	}
	return handle(e)
}
func (l *endpointResidentLoop) end(io endpointLaneIO, cause error) error {
	if io == nil {
		return errEndpointFrame
	}
	if l.stopped {
		return errEndpointStopped
	}
	return l.stop(io, cause)
}
