//go:build mnet_enrollment_development

package osownership

import "time"

// Projection consumes source-owned witnesses, never creates them or verifies an
// OS itself. A witness is scoped to one target/session/namespace and interval.
// The caller's clock must be trusted; expiry enforcement stays in Transaction.
type networkCapabilityWitness struct {
	Kind       string
	TargetID   string
	SessionID  string
	Namespace  Namespace
	ObservedAt time.Time
	ValidUntil time.Time
}
type networkCapabilityEvidence struct {
	Final             *adapterFinalState
	RunnerObservation string
	TargetID          string
	SessionID         string
	Namespace         Namespace
	Now               time.Time
	Witnesses         []networkCapabilityWitness
	CleanupPending    bool
}
type networkCapabilityStatus struct {
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
	if e.Final == nil || e.CleanupPending || e.Final.requiresCleanup() {
		r.State = "cleanup-pending"
		return r
	}
	kinds := map[string]bool{}
	if e.TargetID != "" && e.SessionID != "" && !e.Now.IsZero() && sameNamespace(e.Namespace, e.Namespace) {
		for _, w := range e.Witnesses {
			w.ObservedAt = w.ObservedAt.Round(0).UTC()
			w.ValidUntil = w.ValidUntil.Round(0).UTC()
			if w.TargetID != e.TargetID || w.SessionID != e.SessionID || !sameNamespace(w.Namespace, e.Namespace) || w.ObservedAt.IsZero() || e.Now.Before(w.ObservedAt) || !e.Now.Before(w.ValidUntil) {
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
