package control

// Optional durable DEVELOPMENT schema. Nil/omitempty leaves legacy bytes exact.
// Only the tagged incompatible V2 owner constructs a non-nil instance.
import (
	contract "fintex.au/mnet/internal/enrollmentcontract"
)

type registrarAdmissionLedger struct {
	Version               int                                 `json:"version"`
	ConfigFingerprint     string                              `json:"config_fingerprint"`
	SourceAuthorityDigest string                              `json:"source_authority_digest"`
	SourceTotal           int                                 `json:"source_total"`
	PrincipalTotals       map[string]int                      `json:"principal_totals"`
	Entries               map[string]*registrarAdmissionEntry `json:"entries"`
}
type registrarAdmissionEntry struct {
	Version               int    `json:"version"`
	Purpose               string `json:"purpose"`
	AttemptID             string `json:"attempt_id"`
	DescriptorDigest      string `json:"descriptor_digest"`
	RequestDigest         string `json:"request_digest"`
	BootstrapDigest       string `json:"bootstrap_digest"`
	ConfigFingerprint     string `json:"config_fingerprint"`
	SourceAuthorityDigest string `json:"source_authority_digest"`
	Phase                 string `json:"phase"`
	SourceDebit           int    `json:"source_debit"`
	PrincipalID           string `json:"principal_id"`
	PrincipalNamespace    string `json:"principal_namespace"`
	PrincipalTupleDigest  string `json:"principal_tuple_digest"`
	PrincipalDebit        int    `json:"principal_debit"`
	ObservationDigest     string `json:"observation_digest"`
	CheckedAtMS           int64  `json:"checked_at_ms"`
	ReserveDeadlineMS     int64  `json:"reserve_deadline_ms"`
	ChallengeDigest       string `json:"challenge_digest"`
	ChallengeExpiresAtMS  int64  `json:"challenge_expires_at_ms"`
	AccessExpiresAtMS     int64  `json:"access_expires_at_ms"`
	ReceiptID             string `json:"receipt_id"`
	ReceiptDigest         string `json:"receipt_digest"`
}

// No public setter, env/config selector, startup fallback or generic codec route.
// Nil is the accepted legacy/V1 behavior. Tagged V2 instance owns all effects.
type registrarAdmissionPolicy interface {
	checkConfig() error
	reserve(contract.SourceReserveRequest, *registrarEnrollmentOwner) error
	observe(contract.SourceReserveRequest, contract.SourceReserve, *registrarEnrollmentOwner) error
	challenge(contract.SourceReserveRequest, contract.Challenge, *registrarEnrollmentOwner) error
	commit(contract.Challenge, contract.ReceiptV2) (func(), error)
}

// Optional incompatible codec validation. A non-nil ledger can NEVER be read or
// published by nil/V1 codecs even if a caller copied JSON fields into a file.
type registrarAdmissionCodec interface{ validateAdmission(*Store) error }

func (s *Store) validateAdmissionFormat() error {
	if v, ok := s.storageCodec.(registrarAdmissionCodec); ok {
		return v.validateAdmission(s)
	}
	if s.data.Admission != nil {
		return errRegistrarEnrollment
	}
	return nil
}
