//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"errors"
	"sync"
)

var errEndpointFrame = errors.New("endpoint-frame-invalid-or-stopped")
var errEndpointBackpressure = errors.New("endpoint-frame-backpressure")

const endpointHeader = 32
const endpointQueue = 16
const (
	endpointPrepared byte = iota + 1
	endpointAdmission
	endpointDatagram
	endpointStop
	endpointStopped
)

// Framing is DATA, never authentication, custody or a completion receipt.
type endpointFrame struct {
	kind, direction      byte
	sequence, generation uint64
	payload              []byte
}

func endpointFrameLimit(kind byte) int {
	switch kind {
	case endpointPrepared:
		return endpointPreparedBytes
	case endpointAdmission:
		return endpointAdmissionBytes
	case endpointDatagram:
		return MTU
	case endpointStop, endpointStopped:
		return 0
	}
	return -1
}
func encodeEndpointFrame(f endpointFrame) ([]byte, error) {
	limit := endpointFrameLimit(f.kind)
	if limit < 0 || len(f.payload) > limit || f.sequence == 0 || f.generation == 0 || (f.direction != 1 && f.direction != 2) {
		return nil, errEndpointFrame
	}
	if err := validateEndpointControl(f); err != nil {
		return nil, err
	}
	out := make([]byte, endpointHeader+len(f.payload))
	copy(out, "MNE1")
	out[4] = 1
	out[5] = f.kind
	out[6] = f.direction
	binary.BigEndian.PutUint64(out[8:16], f.sequence)
	binary.BigEndian.PutUint64(out[16:24], f.generation)
	binary.BigEndian.PutUint32(out[24:28], uint32(len(f.payload)))
	copy(out[32:], f.payload)
	return out, nil
}
func decodeEndpointFrame(raw []byte) (endpointFrame, error) {
	var f endpointFrame
	if len(raw) < endpointHeader || string(raw[:4]) != "MNE1" || raw[4] != 1 || raw[7] != 0 || binary.BigEndian.Uint32(raw[28:32]) != 0 {
		return f, errEndpointFrame
	}
	f = endpointFrame{kind: raw[5], direction: raw[6], sequence: binary.BigEndian.Uint64(raw[8:16]), generation: binary.BigEndian.Uint64(raw[16:24])}
	n := int(binary.BigEndian.Uint32(raw[24:28]))
	limit := endpointFrameLimit(f.kind)
	if limit < 0 || n > limit || len(raw) != endpointHeader+n || f.sequence == 0 || f.generation == 0 || (f.direction != 1 && f.direction != 2) {
		return endpointFrame{}, errEndpointFrame
	}
	f.payload = append([]byte(nil), raw[endpointHeader:]...)
	if err := validateEndpointControl(f); err != nil {
		return endpointFrame{}, err
	}
	return f, nil
}

// One private instance per inherited direction, never a network service. Feed
// accepts at most one frame's worth per call; caller chunks a coalesced read.
// EOF always ends the lane; partial buffered bytes make it malformed as well.
type endpointFrameDecoder struct {
	pending []byte
	ended   bool
}

func (d *endpointFrameDecoder) feed(chunk []byte) ([]endpointFrame, error) {
	if d.ended || len(chunk) > endpointHeader+MTU {
		d.ended = true
		return nil, errEndpointFrame
	}
	d.pending = append(d.pending, chunk...)
	var out []endpointFrame
	for len(d.pending) >= endpointHeader {
		n := int(binary.BigEndian.Uint32(d.pending[24:28]))
		limit := endpointFrameLimit(d.pending[5])
		if limit < 0 || n > limit || len(d.pending) > 2*(endpointHeader+MTU) {
			d.ended = true
			d.pending = nil
			return nil, errEndpointFrame
		}
		if len(d.pending) < endpointHeader+n {
			break
		}
		f, err := decodeEndpointFrame(d.pending[:endpointHeader+n])
		if err != nil {
			d.ended = true
			d.pending = nil
			return nil, err
		}
		out = append(out, f)
		d.pending = d.pending[endpointHeader+n:]
		if len(out) > endpointQueue {
			d.ended = true
			d.pending = nil
			return nil, errEndpointFrame
		}
	}
	return out, nil
}
func (d *endpointFrameDecoder) eof() error {
	d.ended = true
	bad := len(d.pending) != 0
	d.pending = nil
	if bad {
		return errEndpointFrame
	}
	return errors.New("endpoint-channel-eof")
}

// This state machine enforces protocol ordering only. Receiving admission does
// NOT set Transaction.Ready, authenticate plaintext or issue a packet-use permit.
// The actual original reactor must separately require its genuine live owner.
type endpointFrameState struct {
	mu                           sync.Mutex
	generation, received, sent   uint64
	inbound, outbound            byte
	prepared, admission, stopped bool
	policy                       *endpointPacketPolicy
	preparedBody                 endpointPreparedBody
	queue                        [][]byte
	inFlight                     []byte
}

func newEndpointFrameState(generation uint64, inbound byte, policy *endpointPacketPolicy) (*endpointFrameState, error) {
	if generation == 0 || (inbound != 1 && inbound != 2) || policy == nil {
		return nil, errEndpointFrame
	}
	return &endpointFrameState{generation: generation, inbound: inbound, outbound: 3 - inbound, policy: policy}, nil
}
func (s *endpointFrameState) stopLocked() { s.stopped = true; s.queue = nil; s.inFlight = nil }
func (s *endpointFrameState) stop()       { s.mu.Lock(); defer s.mu.Unlock(); s.stopLocked() }
func (s *endpointFrameState) receive(f endpointFrame) (admittedEndpointPacket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var zero admittedEndpointPacket
	if s.stopped || f.direction != s.inbound || f.generation != s.generation || s.received == ^uint64(0) || f.sequence != s.received+1 {
		s.stopLocked()
		return zero, errEndpointFrame
	}
	if _, err := encodeEndpointFrame(f); err != nil {
		s.stopLocked()
		return zero, err
	}
	s.received = f.sequence
	switch f.kind {
	case endpointStop, endpointStopped:
		s.stopLocked()
		return zero, errEndpointFrame
	case endpointPrepared:
		if f.direction != 2 || s.prepared || s.admission {
			s.stopLocked()
			return zero, errEndpointFrame
		}
		b, e := decodeEndpointPrepared(f.payload)
		if e != nil {
			s.stopLocked()
			return zero, e
		}
		s.preparedBody = b
		s.prepared = true
	case endpointAdmission:
		if f.direction != 1 || !s.prepared || s.admission {
			s.stopLocked()
			return zero, errEndpointFrame
		}
		b, e := decodeEndpointAdmission(f.payload)
		if e != nil || b.Prepared != s.preparedBody {
			s.stopLocked()
			return zero, errEndpointFrame
		}
		s.admission = true
	case endpointDatagram:
		if !s.admission {
			s.stopLocked()
			return zero, errEndpointFrame
		}
		direction := packetToKernel
		if s.inbound == 2 {
			direction = packetToPeer
		}
		return s.policy.admit(f.payload, direction)
	default:
		s.stopLocked()
		return zero, errEndpointFrame
	}
	return zero, nil
}
func (s *endpointFrameState) enqueue(kind byte, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.sent == ^uint64(0) {
		return errEndpointFrame
	}
	if kind == endpointStop || kind == endpointStopped {
		s.stopLocked()
		return errEndpointFrame
	}
	if kind == endpointPrepared && (s.outbound != 2 || s.prepared || s.admission) {
		return errEndpointFrame
	}
	if kind == endpointAdmission && (s.outbound != 1 || !s.prepared || s.admission) {
		return errEndpointFrame
	}
	if len(s.queue) >= endpointQueue {
		return errEndpointBackpressure
	}
	if kind == endpointDatagram {
		if !s.admission {
			return errEndpointFrame
		}
		direction := packetToPeer
		if s.outbound == 1 {
			direction = packetToKernel
		}
		v, e := s.policy.admit(payload, direction)
		if e != nil {
			return e
		}
		payload = v.bytes
	}
	if kind == endpointPrepared {
		b, e := decodeEndpointPrepared(payload)
		if e != nil {
			return e
		}
		s.preparedBody = b
	}
	if kind == endpointAdmission {
		b, e := decodeEndpointAdmission(payload)
		if e != nil || b.Prepared != s.preparedBody {
			return errEndpointFrame
		}
	}
	raw, e := encodeEndpointFrame(endpointFrame{kind: kind, direction: s.outbound, generation: s.generation, sequence: s.sent + 1, payload: payload})
	if e != nil {
		return e
	}
	s.sent++
	s.queue = append(s.queue, raw)
	if kind == endpointPrepared {
		s.prepared = true
	}
	if kind == endpointAdmission {
		s.admission = true
	}
	return nil
}

// Exactly one writer may have a frame out. The driver writes AFTER this method
// unlocks and acknowledges separately. Never hold this mutex over bridge I/O.
func (s *endpointFrameState) nextWrite() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.inFlight != nil {
		return nil, errEndpointFrame
	}
	if len(s.queue) == 0 {
		return nil, errEndpointBackpressure
	}
	s.inFlight = s.queue[0]
	s.queue = s.queue[1:]
	return append([]byte(nil), s.inFlight...), nil
}
func (s *endpointFrameState) finishWrite(n int, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.inFlight == nil || err != nil || n != len(s.inFlight) {
		s.stopLocked()
		return errEndpointFrame
	}
	s.inFlight = nil
	return nil
}
