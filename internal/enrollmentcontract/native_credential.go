package enrollmentcontract

// Explicit new INACTIVE credential document. Not State.json, a request scheme,
// authenticated delivery, usable credential, finalization marker or permit.
import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

const NativeCredentialKind = "registrar-native-experimental-v1"

type NativeCredentialDocument struct {
	Version        int       `json:"version"`
	Kind           string    `json:"kind"`
	Secret         string    `json:"secret"`
	FinalizationID string    `json:"finalization_id"`
	Challenge      Challenge `json:"challenge"`
	Receipt        ReceiptV2 `json:"receipt"`
	ReceiptDigest  string    `json:"receipt_digest"`
}

func (NativeCredentialDocument) String() string {
	return "[private inactive native credential document DATA]"
}
func (NativeCredentialDocument) GoString() string {
	return "[private inactive native credential document DATA]"
}
func DecodeNativeCredentialDocument(raw []byte) (NativeCredentialDocument, error) {
	var d NativeCredentialDocument
	if !proofWireExact(raw, &d) || CheckNativeCredentialDocument(d) != nil {
		return NativeCredentialDocument{}, ErrInvalid
	}
	return d, nil
}

// Checks historical internal consistency only. No current clock, account/source
// eligibility, receipt origin, policy installation, signature or node is checked.
func CheckNativeCredentialDocument(d NativeCredentialDocument) error {
	if d.Version != 1 || d.Kind != NativeCredentialKind {
		return ErrInvalid
	}
	if _, err := digest(d.FinalizationID); err != nil {
		return err
	}
	if _, err := digest(d.Secret); err != nil {
		return err
	}
	expected, err := ReceiptV2ForChallenge(d.Challenge, d.Receipt.NodeID, d.Receipt.ReceiptID, d.Receipt.CommittedAtMS)
	if err != nil || expected != d.Receipt || ReceiptV2Digest(expected) != d.ReceiptDigest {
		return ErrInvalid
	}
	want, err := digest(expected.CredentialDigest)
	if err != nil {
		return err
	}
	got := sha256.Sum256([]byte(d.Secret))
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		return ErrInvalid
	}
	return nil
}

// NativeCredentialSecretDigest hashes canonical native secret TEXT, not its
// decoded bytes. This is binding DATA only, never credential verification.
func NativeCredentialSecretDigest(secret string) (string, error) {
	if _, err := digest(secret); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
