package control

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"

	"fintex.au/mnet/internal/enrollmentcontract"
)

// DATA persisted inside strict registrar namespace only. Public digest data is
// not authentication and no production entry point constructs a consumed grant.
type registrarConsumption struct {
	Challenge        enrollmentcontract.Challenge `json:"challenge"`
	TranscriptDigest string                       `json:"transcript_digest"`
}

func registrarTranscriptDigest(c enrollmentcontract.Challenge) string {
	raw, err := enrollmentcontract.Encode(c)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
func registrarProofKey(c enrollmentcontract.Challenge) string {
	b, _ := json.Marshal([]string{c.Binding.PrincipalAuthority, c.Binding.RegistrarOrigin, c.ID})
	return string(b)
}
func validateRegistrarConsumption(state *registrarState, index string, r *registrarRecord) error {
	if r.ReceiptEncodingVersion == 1 {
		if r.Consumption != nil {
			return errRegistrarState
		}
		return nil
	}
	if state.Version != 2 || r.Consumption == nil {
		return errRegistrarState
	}
	c := r.Consumption.Challenge
	b := c.Binding
	if r.Consumption.TranscriptDigest == "" || registrarTranscriptDigest(c) != r.Consumption.TranscriptDigest {
		return errRegistrarState
	}
	if state.ConsumedProofs[registrarProofKey(c)] != index {
		return errRegistrarState
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(b.DevicePublicKey)
	if err != nil {
		return errRegistrarState
	}
	if b.NetworkID != r.NetworkID || base64.StdEncoding.EncodeToString(raw) != r.DevicePublicKey || b.CredentialDigest != r.CredentialDigest || b.IssuerLabel != r.Scope.Issuer || b.RegistrarOrigin != r.Scope.RegistrarOrigin || b.Audience != r.Scope.Audience || b.ServiceID != r.Scope.ServiceID || b.WorkspaceID != r.Scope.WorkspaceID || b.MappingRevision != r.Scope.MappingRevision || b.HandoffID != r.Scope.HandoffID || b.AttemptID != r.Scope.AttemptID || b.ReserveID != r.Scope.ReserveID || b.DescriptorDigest != r.DescriptorDigest || b.DescriptorEncodingVersion != r.DescriptorEncodingVersion {
		return errRegistrarState
	}
	if b.AccessUntilMS != r.AccessExpiresAtMS || r.CommittedAtMS < b.CheckedAtMS || r.CommittedAtMS >= c.ExpiresAtMS {
		return errRegistrarState
	}
	return nil
}

// Keep present-empty v2 index explicit; omitempty alone would silently remove
// an empty map on unrelated saves. V1 bytes retain their previous shape.
func (r registrarState) MarshalJSON() ([]byte, error) {
	if r.Version == 1 {
		return json.Marshal(struct {
			Version int                         `json:"version"`
			Records map[string]*registrarRecord `json:"records"`
		}{r.Version, r.Records})
	}
	return json.Marshal(struct {
		Version        int                         `json:"version"`
		Records        map[string]*registrarRecord `json:"records"`
		ConsumedProofs map[string]string           `json:"consumed_proofs"`
	}{r.Version, r.Records, r.ConsumedProofs})
}
func registrarConsumptionJSON(raw []byte) bool {
	if !registrarExactFields(raw, []string{"challenge", "transcript_digest"}) {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	if !registrarExactFields(object["challenge"], []string{"binding", "id", "nonce", "ephemeral_public_key", "expires_at_ms"}) {
		return false
	}
	var challenge map[string]json.RawMessage
	if json.Unmarshal(object["challenge"], &challenge) != nil {
		return false
	}
	return registrarExactFields(challenge["binding"], []string{"principal_authority", "principal_namespace", "principal_id", "session_binding", "issuer_label", "registrar_origin", "audience", "service_id", "workspace_id", "network_id", "mapping_revision", "handoff_id", "attempt_id", "reserve_id", "descriptor_digest", "descriptor_encoding_version", "device_public_key", "credential_digest", "checked_at_ms", "reserve_deadline_ms", "access_until_ms"})
}
