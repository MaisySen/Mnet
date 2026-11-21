//go:build mnet_enrollment_development

package osownership

// Evidence consists only of fixed allowlisted status and descriptor numbers,
// no environment, arbitrary proc contents, file paths or user material.
type BootstrapReadOnlyEvidence struct {
	Client            ClientBootstrapEvidence `json:"client"`
	IPv6Directories   map[string]string       `json:"ipv6_directories"`
	OwnerNamespace    Namespace               `json:"owner_namespace"`
	OuterNamespace    Namespace               `json:"outer_namespace"`
	ClientJoined      bool                    `json:"client_joined"`
	BaselineUnchanged bool                    `json:"baseline_unchanged"`
	LinkCount         int                     `json:"link_count"`
}
type ClientBootstrapEvidence struct {
	FailureClass   string            `json:"failure_class,omitempty"`
	FilterBaseline int               `json:"filter_baseline"`
	PID            int               `json:"pid"`
	ParentPID      int               `json:"parent_pid"`
	UID            int               `json:"uid"`
	Status         map[string]string `json:"status"`
	FDs            []int             `json:"fds"`
	Namespace      Namespace         `json:"namespace"`
	ProbeDenied    map[string]int    `json:"probe_denied"`
}
