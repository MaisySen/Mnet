//go:build mnet_enrollment_development

package osownership

import "time"

// Projection consumes source-owned witnesses, never creates them or verifies an
// OS itself. A witness is scoped to one target/session/namespace and interval.
// The caller's clock must be trusted; expiry enforcement stays in Transaction.
type networkCapabilityWitness struct {
	AttemptID  string
	Kind       string
	TargetID   string
	SessionID  string
	Namespace  Namespace
	ObservedAt time.Time
	ValidUntil time.Time
}
type networkCapabilityEvidence struct {
	AttemptID           string
	ObservationSequence uint64
	Final               *adapterFinalState
	Active              *adapterActiveObservation
	RunnerObservation   string
	TargetID            string
	SessionID           string
	Namespace           Namespace
	Now                 time.Time
	Witnesses           []networkCapabilityWitness
	CleanupPending      bool
}
type networkCapabilityStatus struct {
	AttemptID         string    `json:"attempt_id"`
	CustodyUnresolved bool      `json:"custody_unresolved"`
	State             string    `json:"state"`
	RunnerObservation string    `json:"runner_observation"`
	UserDeployment    string    `json:"user_deployment"`
	TargetID          string    `json:"target_id"`
	SessionID         string    `json:"session_id"`
	AsOf              time.Time `json:"as_of"`
	ValidUntil        time.Time `json:"valid_until"`
	NetworkingReady   bool      `json:"networking_ready"`
	AutomaticAction   bool      `json:"automatic_action"`
}

func projectNetworkCapability(e networkCapabilityEvidence) networkCapabilityStatus {
	e.Now = e.Now.Round(0).UTC() // compare explicit wall intervals, not hidden monotonic stamps
	r := networkCapabilityStatus{State: "not-verified", RunnerObservation: e.RunnerObservation, UserDeployment: "unknown", TargetID: e.TargetID, SessionID: e.SessionID, AsOf: e.Now}

	r.AttemptID = e.AttemptID

	r.CustodyUnresolved = e.CleanupPending // preserved on EVERY branch, even foreign
	id := adapterIdentity{TargetID: e.TargetID, SessionID: e.SessionID, Namespace: e.Namespace, AttemptID: e.AttemptID}
	if e.Final != nil {
		if e.Final.Identity != id || !id.valid() {
			if e.Final.requiresCleanup() {
				r.State = "foreign-attempt-cleanup-unresolved"
			} else {
				r.State = "final-scope-mismatch"
			}
			return r
		}
		r.CustodyUnresolved = r.CustodyUnresolved || e.Final.requiresCleanup()
		if r.CustodyUnresolved {
			r.State = "custody-unresolved"
			return r
		}
		o := e.Final.Observation
		if o == nil || o.Identity != id || o.Sequence == 0 || o.Sequence != e.ObservationSequence || o.ObservedAt.IsZero() || e.Now.Before(o.ObservedAt) {
			r.State = "historical-final-undated-or-unbound"
			return r
		}
		if e.Final.CloseConfirmed && e.Final.RawBaselineEqual {
			r.State = "session-closed-clean"
		} else {
			r.State = "historical-final-no-active-path"
		}
		// A final is teardown evidence only, never current readiness. No expiry is
		// assigned to historical facts, AsOf is the observation, not current time.
		r.AsOf = o.ObservedAt
		return r
	}
	if r.CustodyUnresolved {
		r.State = "custody-unresolved"
		return r
	}
	a := e.Active
	if !a.current() || !id.valid() || a.identity != id || e.ObservationSequence != a.sequence || e.Now.IsZero() || e.Now.Before(a.observedAt) || !e.Now.Before(a.validUntil) || !e.Now.Before(a.planExpiry) {
		r.State = "active-observation-unverified"
		return r
	}
	r.ValidUntil = a.validUntil
	kinds := map[string]bool{}
	if e.TargetID != "" && e.SessionID != "" && !e.Now.IsZero() && sameNamespace(e.Namespace, e.Namespace) {
		for _, w := range e.Witnesses {
			w.ObservedAt = w.ObservedAt.Round(0).UTC()
			w.ValidUntil = w.ValidUntil.Round(0).UTC()
			if w.AttemptID != e.AttemptID || w.TargetID != e.TargetID || w.SessionID != e.SessionID || !sameNamespace(w.Namespace, e.Namespace) || w.ObservedAt.IsZero() || e.Now.Before(w.ObservedAt) || !e.Now.Before(w.ValidUntil) {
				continue
			}
			switch w.Kind {
			case "deployment", "os-path", "transport", "authority":
			default:
				continue
			}
			// Duplicates are contradictory/ambiguous evidence, not freshness upgrades.
			if kinds[w.Kind] {
				r.State = "ambiguous-evidence"
				return r
			}
			kinds[w.Kind] = true
			if r.ValidUntil.IsZero() || w.ValidUntil.Before(r.ValidUntil) {
				r.ValidUntil = w.ValidUntil
			}
		}
	}
	if kinds["deployment"] {
		r.UserDeployment = "verified-for-bound-target"
	}
	if !kinds["authority"] {
		r.State = "no-live-network-authority"
		return r
	}
	if kinds["os-path"] && kinds["transport"] && kinds["deployment"] {
		r.State = "verified-specified-network-path"
		r.NetworkingReady = true
		return r
	}
	if e.RunnerObservation == "device-open-denied" {
		r.State = "historical-runner-device-open-denied-target-unverified"
		return r
	}
	if kinds["transport"] {
		r.State = "transport-only-os-path-unverified"
	}
	return r
}

// Conservative final-state mapping, intentionally independent of a caller's
// optional CleanupPending flag. A missing final record cannot claim readiness.
func (f adapterFinalState) requiresCleanup() bool {
	if f.CustodyUnresolved || f.RejectedHandle != nil || f.CloseAmbiguous || f.CleanupPending != nil {
		return true
	}
	if f.CreateAttempted && (!f.CloseConfirmed || !f.RawBaselineEqual) {
		return true
	}
	// Poisoned/pending history can be resolved only by proven close plus baseline.
	return (f.Poisoned || f.PossiblyApplied != nil || f.CleanupRetry != nil) && (!f.CloseConfirmed || !f.RawBaselineEqual)
}
