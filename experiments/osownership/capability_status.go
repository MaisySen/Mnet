//go:build mnet_enrollment_development

package osownership

// Pure projection of already-owned evidence, not a detector or authorization.
// No inferred install permission, target-device access or automatic self-check.
type networkCapabilityEvidence struct {
	RunnerObservation      string
	UserDeploymentVerified bool
	CleanupPending         bool
	TransportSessionActive bool
	OSPathVerified         bool
	OriginalAuthorityLive  bool
}
type networkCapabilityStatus struct {
	State             string `json:"state"`
	RunnerObservation string `json:"runner_observation"`
	UserDeployment    string `json:"user_deployment"`
	NetworkingReady   bool   `json:"networking_ready"`
	AutomaticAction   bool   `json:"automatic_action"`
}

func projectNetworkCapability(e networkCapabilityEvidence) networkCapabilityStatus {
	r := networkCapabilityStatus{State: "not-verified", RunnerObservation: e.RunnerObservation, UserDeployment: "unknown"}
	if e.UserDeploymentVerified {
		r.UserDeployment = "verified-for-specified-target"
	}
	if e.CleanupPending {
		r.State = "cleanup-pending"
		return r
	}
	if !e.OriginalAuthorityLive {
		r.State = "no-live-network-authority"
		return r
	}
	if e.OSPathVerified && e.TransportSessionActive && e.UserDeploymentVerified {
		r.State = "verified-specified-network-path"
		r.NetworkingReady = true
		return r
	}
	if e.RunnerObservation == "device-open-denied" {
		r.State = "runner-device-access-blocked"
		return r
	}
	if e.TransportSessionActive {
		r.State = "transport-only-os-path-unverified"
	}
	return r
}
