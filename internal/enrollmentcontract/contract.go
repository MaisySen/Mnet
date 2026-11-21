// Package enrollmentcontract defines INACTIVE enrollment binding bytes and
// fixture-clock checks. It authenticates nothing and has no permit type, proof
// verifier, Store access, network call or caller in the running application.
package enrollmentcontract

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"fintex.au/mnet/internal/enrollmentidentity"
)

const Domain = "mnet/enrollment-key-possession/experimental-v4"
const PrincipalNamespace = "multies-local-user-v1"

var ErrInvalid = errors.New("invalid inactive enrollment transcript")

// Binding is untrusted public DATA, not an authenticated claim or grant. In a
// future adapter each expected field needs an independent verified source.
// IssuerLabel preserves the existing configured label. PrincipalAuthority and
// Namespace explicitly distinguish local Multies IDs from OIDC subjects.
type Binding struct {
	PrincipalAuthority        string `json:"principal_authority"`
	PrincipalNamespace        string `json:"principal_namespace"`
	PrincipalID               string `json:"principal_id"`
	SessionBinding            string `json:"session_binding"`
	IssuerLabel               string `json:"issuer_label"`
	RegistrarOrigin           string `json:"registrar_origin"`
	Audience                  string `json:"audience"`
	ServiceID                 string `json:"service_id"`
	WorkspaceID               string `json:"workspace_id"`
	NetworkID                 string `json:"network_id"`
	MappingRevision           string `json:"mapping_revision"`
	HandoffID                 string `json:"handoff_id"`
	AttemptID                 string `json:"attempt_id"`
	ReserveID                 string `json:"reserve_id"`
	DescriptorDigest          string `json:"descriptor_digest"`
	DescriptorEncodingVersion int    `json:"descriptor_encoding_version"`
	DevicePublicKey           string `json:"device_public_key"`
	CredentialDigest          string `json:"credential_digest"`
	CheckedAtMS               int64  `json:"checked_at_ms"`
	ReserveDeadlineMS         int64  `json:"reserve_deadline_ms"`
	AccessUntilMS             int64  `json:"access_until_ms"`
}
type Challenge struct {
	Binding            Binding `json:"binding"`
	ID                 string  `json:"id"`
	Nonce              string  `json:"nonce"`
	EphemeralPublicKey string  `json:"ephemeral_public_key"`
	ExpiresAtMS        int64   `json:"expires_at_ms"`
}

func text(s string) bool {
	if s == "" || len(s) > 2048 || !utf8.ValidString(s) || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func digest(s string) ([]byte, error) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	nonzero := false
	for _, v := range b {
		nonzero = nonzero || v != 0
	}
	if !nonzero {
		return nil, ErrInvalid
	}
	return b, nil
}

// Encode serializes untrusted data, never verifies possession/consent/current
// eligibility. A valid transcript MUST NOT become a Store permit by casting it.
func Encode(c Challenge) ([]byte, error) {
	b := c.Binding
	if b.PrincipalNamespace != PrincipalNamespace || !enrollmentidentity.ValidOrigin(b.PrincipalAuthority) || !enrollmentidentity.ValidIssuer(b.IssuerLabel) || !enrollmentidentity.ValidOrigin(b.RegistrarOrigin) || b.DescriptorEncodingVersion != 1 {
		return nil, ErrInvalid
	}
	for _, s := range []string{b.PrincipalID, b.Audience, b.ServiceID, b.WorkspaceID, b.NetworkID, b.MappingRevision} {
		if !text(s) {
			return nil, ErrInvalid
		}
	}
	if _, err := digest(b.SessionBinding); err != nil {
		return nil, err
	}
	if b.CheckedAtMS <= 0 || b.ReserveDeadlineMS <= b.CheckedAtMS || b.AccessUntilMS <= b.CheckedAtMS || b.AccessUntilMS > b.ReserveDeadlineMS || c.ExpiresAtMS <= b.CheckedAtMS || c.ExpiresAtMS > b.AccessUntilMS {
		return nil, ErrInvalid
	}
	fields := [][]byte{[]byte(Domain)}
	for _, s := range []string{c.ID, c.Nonce, c.EphemeralPublicKey} {
		raw, err := digest(s)
		if err != nil {
			return nil, err
		}
		fields = append(fields, raw)
	}
	for _, s := range []string{b.PrincipalAuthority, b.PrincipalNamespace, b.PrincipalID, b.SessionBinding, b.IssuerLabel, b.RegistrarOrigin, b.Audience, b.ServiceID, b.WorkspaceID, b.NetworkID, b.MappingRevision} {
		fields = append(fields, []byte(s))
	}
	for _, s := range []string{b.HandoffID, b.AttemptID, b.ReserveID, b.DescriptorDigest} {
		raw, err := digest(s)
		if err != nil {
			return nil, err
		}
		fields = append(fields, raw)
	}
	fields = append(fields, []byte("1"))
	for _, s := range []string{b.DevicePublicKey, b.CredentialDigest} {
		raw, err := digest(s)
		if err != nil {
			return nil, err
		}
		fields = append(fields, raw)
	}
	for _, ms := range []int64{b.CheckedAtMS, b.ReserveDeadlineMS, b.AccessUntilMS, c.ExpiresAtMS} {
		fields = append(fields, binary.BigEndian.AppendUint64(nil, uint64(ms)))
	}
	var out []byte
	for _, f := range fields {
		out = binary.BigEndian.AppendUint32(out, uint32(len(f)))
		out = append(out, f...)
	}
	return out, nil
}

// FixtureTiming is an explicitly supplied OFFLINE test boundary. There are no
// defaults or live config. Passing it only checks arithmetic on caller data,
// never current authority, trusted time, eligibility or a revocation guarantee.
type FixtureTiming struct{ MaxObservationAgeMS, MaxLeaseMS, ClockUncertaintyMS int64 }

func CheckFixtureTime(c Challenge, nowMS int64, p FixtureTiming) error {
	if _, err := Encode(c); err != nil {
		return err
	}
	if p.MaxObservationAgeMS <= 0 || p.MaxLeaseMS <= 0 || p.ClockUncertaintyMS < 0 || p.ClockUncertaintyMS >= p.MaxLeaseMS || nowMS <= 0 {
		return ErrInvalid
	}
	b := c.Binding
	if nowMS < b.CheckedAtMS || nowMS-b.CheckedAtMS > p.MaxObservationAgeMS || b.AccessUntilMS-b.CheckedAtMS > p.MaxLeaseMS {
		return ErrInvalid
	}
	// Subtraction form avoids overflow when checking now + uncertainty < expiry.
	for _, end := range []int64{b.ReserveDeadlineMS, b.AccessUntilMS, c.ExpiresAtMS} {
		if end <= nowMS || end-nowMS <= p.ClockUncertaintyMS {
			return ErrInvalid
		}
	}
	return nil
}
