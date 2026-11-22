//go:build mnet_enrollment_development

package osownership

import "time"

// Three post-PREPARED local requests: capability POST + its map GET, then
// one explicit peer-map refresh. The client's 16-request lifetime maximum is
// not a promise that all requests occur after PREPARED or take 48 seconds.
const endpointRegistrarPhaseRequests = 3
const endpointRegistrarRequestBudget = 3 * time.Second
const endpointCoordinationBudget = 30 * time.Second
const endpointHandshakeSlack = 6 * time.Second
const endpointHandshakeBudget = endpointRegistrarPhaseRequests*endpointRegistrarRequestBudget + endpointCoordinationBudget + endpointHandshakeSlack
const endpointCleanupBudget = 500 * time.Millisecond
const endpointFinalStatsBudget = 500 * time.Millisecond
const endpointGraceBudget = endpointCleanupBudget + endpointFinalStatsBudget + 250*time.Millisecond
const endpointIdleSafetyBudget = 50 * time.Millisecond

// Cooperative STOP accounting only, not hard bounds on synchronous syscalls.
// Allow one STOP-send window, one in-flight observation piece, cleanup and
// final stats, while retaining one idle-safety window of scheduling slack.
const endpointStopFlushBudget = endpointIdleSafetyBudget
const endpointObservationPieceBudget = endpointGraceBudget - endpointCleanupBudget - endpointFinalStatsBudget - 2*endpointIdleSafetyBudget

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
	return !s.stopped && s.admission && s.flowStarted && len(s.queue) < endpointQueue
}
