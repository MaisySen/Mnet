//go:build mnet_enrollment_development

package osownership

import "time"

const endpointHandshakeBudget = 30 * time.Second
const endpointCleanupBudget = 500 * time.Millisecond
const endpointFinalStatsBudget = 500 * time.Millisecond
const endpointGraceBudget = endpointCleanupBudget + endpointFinalStatsBudget + 250*time.Millisecond
const endpointIdleSafetyBudget = 50 * time.Millisecond

// Checked protocol accounting for the sole START already consumed on the
// concrete inherited channel. Not a constructor for channel/native authority.
func (s *endpointFrameState) consumeStart(f endpointFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.inbound != 1 || s.outbound != 2 || s.received != 0 || s.sent != 0 || s.prepared || s.admission || f.kind != endpointStart || f.direction != 1 || f.sequence != 1 || f.generation != s.generation {
		s.stopLocked()
		return errEndpointFrame
	}
	if _, e := encodeEndpointFrame(f); e != nil {
		s.stopLocked()
		return e
	}
	s.received = 1
	return nil
}
func (s *endpointFrameState) datagramRoom() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.stopped && s.admission && len(s.queue) < endpointQueue
}
