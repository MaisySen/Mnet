//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"testing"
)

// Constant/error/codec DATA only. No context or timer is constructed, and no
// source controller, cleanup record, pipe, collector, syscall or file is used.
func TestEndpointStopBudgetDataMemory(t *testing.T) {
	if endpointObservationPieceBudget <= 0 || endpointStopFlushBudget <= 0 {
		t.Fatal("nonpositive cooperative window")
	}
	sum := endpointStopFlushBudget + endpointObservationPieceBudget + endpointCleanupBudget + endpointFinalStatsBudget
	if sum >= endpointGraceBudget {
		t.Fatal("no cooperative accounting slack")
	}
	if endpointObservationPieceBudget+endpointStopFlushBudget != endpointGraceBudget-endpointCleanupBudget-endpointFinalStatsBudget-endpointIdleSafetyBudget {
		t.Fatal("unaccounted source budget")
	}
	// Armed-loop accounting only. Excludes gate, START wait and setup pieces.
	// This checks constant arithmetic only, never syscall or scheduling latency.
}
func TestEndpointStopOutcomeDataMemory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		e      error
		reason endpointStopReason
	}{
		{"decoded-stop", errEndpointStopped, endpointReasonRequested},
		{"input-hup", errEndpointInputClosed, endpointReasonEOF},
		{"hup-with-other-refusal", errors.Join(errEndpointInputClosed, ErrConflict), endpointReasonEOF},
		{"observation-timeout", context.DeadlineExceeded, endpointReasonTimeout},
		{"operation-cancelled", context.Canceled, endpointReasonLifetime},
		{"protocol", errEndpointFrame, endpointReasonProtocol},
		{"unknown", ErrConflict, endpointReasonCustody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := endpointExitReason(tc.e, endpointReasonCustody); got != tc.reason {
				t.Fatal("wrong reason", got)
			}
			if endpointFinalReasonData(tc.e, endpointReasonCustody, true) != tc.reason || endpointFinalReasonData(tc.e, endpointReasonCustody, false) != endpointReasonCleanup {
				t.Fatal("cleanup reason override")
			}
		})
	}
	if errors.Is(errEndpointInputClosed, errEndpointStopped) {
		t.Fatal("HUP falsely proves decoded STOP")
	}
	for _, reason := range []endpointStopReason{endpointReasonRequested, endpointReasonEOF, endpointReasonTimeout, endpointReasonCleanup} {
		b := endpointStatsBody{Reason: reason, CleanupVerified: true}
		wire, e := b.encode()
		if e != nil {
			t.Fatal(e)
		}
		decoded, e := decodeEndpointStats(wire)
		if e != nil || decoded != b {
			t.Fatal("stats roundtrip", e)
		}
		if !endpointStopCleanupData(endpointStopped, decoded) {
			t.Fatal("final verified cleanup lost")
		}
		if endpointStopCleanupData(endpointStats, decoded) {
			t.Fatal("ordinary stats became final cleanup")
		}
		if endpointStopDecodedData(endpointStopped, decoded) != (reason == endpointReasonRequested) {
			t.Fatal("decoded STOP claim")
		}
		decoded.CleanupVerified = false
		if endpointStopCleanupData(endpointStopped, decoded) {
			t.Fatal("reason accepted unverified cleanup")
		}
	}
	t.Run("empty-eof", func(t *testing.T) {
		var d endpointFrameDecoder
		if !errors.Is(d.eof(), errEndpointInputClosed) || !d.ended || len(d.pending) != 0 {
			t.Fatal("clean EOF")
		}
	})
	t.Run("partial-eof", func(t *testing.T) {
		d := endpointFrameDecoder{pending: []byte{1}}
		if !errors.Is(d.eof(), errEndpointFrame) || !d.ended || len(d.pending) != 0 {
			t.Fatal("partial EOF accepted")
		}
	})
}
