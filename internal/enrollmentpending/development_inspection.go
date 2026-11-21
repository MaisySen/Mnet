//go:build mnet_enrollment_development

package enrollmentpending

import (
	"encoding/json"
	"reflect"
)

// DevelopmentInspectionDATA is sanitized FILE classification only. It creates
// no journal, network client, source approval or finalization readiness.
type DevelopmentInspectionDATA struct {
	Phase    string `json:"phase"`
	Recovery string `json:"recovery"`
}

func InspectDevelopmentDATA(raw []byte, device string, cfg SourceConfig) (DevelopmentInspectionDATA, error) {
	var r record
	if !strictRecord(raw, &r) || !validRecord(r) || device != r.Launch.DevicePublicKey {
		return DevelopmentInspectionDATA{}, ErrInvalid
	}
	expected, ok := cfg.Policy.Snapshot()
	if !ok || !reflect.DeepEqual(expected, r.Policy) {
		return DevelopmentInspectionDATA{}, ErrInvalid
	}
	result := DevelopmentInspectionDATA{Phase: r.Phase, Recovery: "unavailable"}
	if r.Phase == "prepared" {
		result.Phase = "prepared-listener-not-owned"
	}
	if historyEligible(r) || r.Version == 6 {
		if !historyPolicyCurrent(r, cfg, device) {
			return DevelopmentInspectionDATA{}, ErrInvalid
		}
		result.Recovery = "authenticated-history"
	}
	if r.Phase == "native-finalization-prepared" || r.Version == 8 {
		result.Recovery = "prepared-original-state"
	}
	// Force the result to remain a small explicitly sanitized projection.
	if _, err := json.Marshal(result); err != nil {
		return DevelopmentInspectionDATA{}, ErrInvalid
	}
	return result, nil
}
