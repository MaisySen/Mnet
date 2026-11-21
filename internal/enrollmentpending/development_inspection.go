//go:build mnet_enrollment_development

package enrollmentpending

import (
	"bytes"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"reflect"
)

// DevelopmentInspectionDATA is sanitized FILE classification only. It creates
// no journal, network client, source approval or finalization readiness.
type DevelopmentInspectionDATA struct {
	OriginalAccessExpiresAtMS int64  `json:"originalAccessExpiresAtMS,omitempty"`
	Phase                     string `json:"phase"`
	Recovery                  string `json:"recovery"`
}

func InspectDevelopmentDATA(raw []byte, device string, cfg SourceConfig) (DevelopmentInspectionDATA, error) {
	var r record
	if !strictRecord(raw, &r) || !validRecord(r) || device != r.Launch.DevicePublicKey {
		return DevelopmentInspectionDATA{}, ErrInvalid
	}
	canonical, _ := json.Marshal(r)
	if !bytes.Equal(raw, canonical) {
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
	if r.Phase == "native-finalization-prepared" || (r.Version == 8 && (r.Phase == "history-lookup-may-have-started" || r.Phase == "history-receipt-data")) {
		result.Recovery = "prepared-original-state"
	}
	if doc, _, e := finalizationDocument(r); e == nil {
		result.OriginalAccessExpiresAtMS = doc.Receipt.AccessExpiresAtMS
	} else if h, ok := decodeHistory(r); ok && h.ReceiptJSON != "" {
		var receipt contract.HistoryReceiptWire
		if json.Unmarshal([]byte(h.ReceiptJSON), &receipt) == nil {
			result.OriginalAccessExpiresAtMS = receipt.Receipt.AccessExpiresAtMS
		}
	}
	// Force the result to remain a small explicitly sanitized projection.
	if _, err := json.Marshal(result); err != nil {
		return DevelopmentInspectionDATA{}, ErrInvalid
	}
	return result, nil
}

// CheckDevelopmentSavedDATA correlates existing saved joining with its exact
// completed journal without creating bookkeeping or any runtime authority.
func CheckDevelopmentSavedDATA(raw []byte, doc contract.NativeCredentialDocument, original, policy string) bool {
	var r record
	if !strictRecord(raw, &r) || !validRecord(r) || r.Phase != "native-finalization-completed" {
		return false
	}
	expected, f, err := finalizationDocument(r)
	return err == nil && expected == doc && f.OriginalStateRevision == original && f.PolicyRevision == policy
}
