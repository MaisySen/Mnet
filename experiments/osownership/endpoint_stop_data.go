//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
)

// Reporting DATA only. An input half-close is not proof a STOP was decoded.
// Cleanup verification remains a separate source outcome and can override the
// diagnostic reason after this classification, never the other way around.
func endpointExitReason(e error, fallback endpointStopReason) endpointStopReason {
	switch {
	case errors.Is(e, errEndpointInputClosed):
		return endpointReasonEOF
	case errors.Is(e, errEndpointStopped):
		return endpointReasonRequested
	case errors.Is(e, context.DeadlineExceeded):
		return endpointReasonTimeout
	case errors.Is(e, context.Canceled):
		return endpointReasonLifetime
	case errors.Is(e, errEndpointFrame):
		return endpointReasonProtocol
	default:
		return fallback
	}
}

// Current v1 format retains one diagnostic reason. Unverified cleanup masks
// the triggering reason; it never becomes successful cleanup by classification.
func endpointFinalReasonData(e error, fallback endpointStopReason, verified bool) endpointStopReason {
	if !verified {
		return endpointReasonCleanup
	}
	return endpointExitReason(e, fallback)
}
func endpointStopCleanupData(kind byte, b endpointStatsBody) bool {
	return kind == endpointStopped && b.CleanupVerified
}
func endpointStopDecodedData(kind byte, b endpointStatsBody) bool {
	return kind == endpointStopped && b.Reason == endpointReasonRequested
}
