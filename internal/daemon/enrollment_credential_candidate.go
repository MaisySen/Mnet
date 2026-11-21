package daemon

import (
	"encoding/base64"
	contract "fintex.au/mnet/internal/enrollmentcontract"
)

// Private context-bound document DATA, not an accepted receipt or usable secret.
// No file loader, getter, State writer, production caller or transport consumes it.
type nativeCredentialCandidate struct {
	context  *nativeEnrollmentContext
	document contract.NativeCredentialDocument
}

func (nativeCredentialCandidate) String() string { return "[private native credential candidate DATA]" }
func (nativeCredentialCandidate) GoString() string {
	return "[private native credential candidate DATA]"
}

// check requires actual d.mu and original current native key/policy owner. This
// does not know receipt provenance/current registrar access and grants nothing.
func (d *Daemon) prepareCredentialCandidateLocked(raw []byte) (*nativeCredentialCandidate, error) {
	context, err := d.enrollmentContextLocked()
	if err != nil {
		return nil, err
	}
	document, err := contract.DecodeNativeCredentialDocument(raw)
	if err != nil {
		return nil, err
	}
	candidate := &nativeCredentialCandidate{context: context, document: document}
	if err := d.checkCredentialCandidateLocked(candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}
func (d *Daemon) checkCredentialCandidateLocked(candidate *nativeCredentialCandidate) error {
	if candidate == nil || candidate.context == nil {
		return errEnrollmentContextStale
	}
	if err := d.checkEnrollmentContextLocked(candidate.context); err != nil {
		return err
	}
	doc := candidate.document
	if contract.CheckNativeCredentialDocument(doc) != nil {
		return errEnrollmentContextStale
	}
	b := doc.Challenge.Binding
	raw, err := base64.RawURLEncoding.Strict().DecodeString(b.DevicePublicKey)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != candidate.context.publicKey {
		return errEnrollmentContextStale
	}
	mapping := contract.SourceMapping{WorkspaceID: b.WorkspaceID, Issuer: b.IssuerLabel, RegistrarOrigin: b.RegistrarOrigin, Audience: b.Audience, MappingRevision: b.MappingRevision, NetworkID: b.NetworkID}
	if !candidate.context.policy.AllowsObservedDATA(mapping, b.PrincipalAuthority, b.PrincipalNamespace, b.PrincipalID, b.ServiceID) {
		return errEnrollmentContextStale
	}
	// Enrollment cannot silently replace a legacy enrolled identity or inherit
	// legacy auth keys/routes/exit capabilities. This is candidate refusal only.
	s := d.stateOwner.saved
	if s.NodeID != "" || s.NodeToken != "" || s.AuthKey != "" || s.ExitNode || len(s.AdvertiseRoutes) != 0 || s.UseExitNode != "" {
		return errEnrollmentContextStale
	}
	return nil
}
