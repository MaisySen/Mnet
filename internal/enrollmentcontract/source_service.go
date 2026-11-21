package enrollmentcontract

import (
	"crypto/sha256"
	"encoding/base64"
	"time"
)

// SourceReserveRequest is the exact registrar-to-Multies request DATA shape.
// Unlike a native exchange result, it contains no expected principal/provenance.
// Ticket is private transport material, deliberately excluded from descriptor.
type SourceReserveRequest struct {
	Ticket    string        `json:"ticket"`
	HandoffID string        `json:"handoffId"`
	ReserveID string        `json:"reserveId"`
	Launch    SourceLaunch  `json:"launch"`
	Mapping   SourceMapping `json:"binding"`
}

func (SourceReserveRequest) String() string   { return "[private service reserve request DATA]" }
func (SourceReserveRequest) GoString() string { return "[private service reserve request DATA]" }
func ServiceReserveDescriptorBytes(request SourceReserveRequest, serviceID string) ([]byte, error) {
	if _, err := digest(request.Ticket); err != nil {
		return nil, err
	}
	return sourceDescriptorBytes(request.HandoffID, request.Launch, request.Mapping, serviceID, request.ReserveID)
}
func ServiceReserveDescriptorDigest(request SourceReserveRequest, serviceID string) (string, error) {
	raw, err := ServiceReserveDescriptorBytes(request, serviceID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// CheckSourceMapping validates DATA syntax only, never installed config/current
// membership or the existence/authority of any principal/network/service.
func CheckSourceMapping(m SourceMapping) error {
	if !validSourceMapping(m) {
		return ErrInvalid
	}
	return nil
}

// DecodeServiceReserve is deliberately distinct from DecodeSourceReserve:
// registrar has its own EXACT SENT request/config, not native's own-PKCE result.
// Source row principal/session-binding are OBSERVED DATA, never synthesized into
// expected native identity. No source authentication or permit is created here.
func DecodeServiceReserve(raw []byte, sent SourceReserveRequest, authority, serviceID string, now time.Time) (SourceReserve, error) {
	var r SourceReserve
	computed, err := ServiceReserveDescriptorDigest(sent, serviceID)
	if err != nil {
		return SourceReserve{}, err
	}
	if !decodeSourceExact(raw, &r) || r.ResponseVersion != 2 || r.DescriptorEncodingVersion != 1 || r.Status != "reserved" || r.Enrolled || r.AuthorityGranted || r.Provenance.PrincipalAuthority != authority || !validSourceProvenance(r.Provenance) || r.Launch != sent.Launch || r.Mapping != sent.Mapping || r.HandoffID != sent.HandoffID || r.ReserveID != sent.ReserveID || r.ServiceID != serviceID || r.DescriptorDigest != computed || now.IsZero() {
		return SourceReserve{}, ErrInvalid
	}
	checked, ok := sourceDate(r.CheckedAt)
	if !ok || checked.After(now) {
		return SourceReserve{}, ErrInvalid
	}
	deadline, ok := sourceDate(r.Deadline)
	if !ok || !deadline.After(now) || !deadline.After(checked) {
		return SourceReserve{}, ErrInvalid
	}
	return r, nil
}
