package enrollmentcontract

// Inactive historical receipt DATA. This codec is the existing Store v2 byte
// recipe, not a signature, channel authentication, usable credential or permit.
import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
)

type ReceiptV2 struct {
	Issuer                    string `json:"issuer"`
	RegistrarOrigin           string `json:"registrar_origin"`
	Audience                  string `json:"audience"`
	ServiceID                 string `json:"service_id"`
	WorkspaceID               string `json:"workspace_id"`
	MappingRevision           string `json:"mapping_revision"`
	HandoffID                 string `json:"handoff_id"`
	AttemptID                 string `json:"attempt_id"`
	ReserveID                 string `json:"reserve_id"`
	DescriptorDigest          string `json:"descriptor_digest"`
	DescriptorEncodingVersion int    `json:"descriptor_encoding_version"`
	ReceiptID                 string `json:"receipt_id"`
	NodeID                    string `json:"node_id"`
	NetworkID                 string `json:"network_id"`
	DevicePublicKey           string `json:"device_public_key"` // existing Store STANDARD base64
	CredentialDigest          string `json:"credential_digest"`
	CommittedAtMS             int64  `json:"committed_at_ms"`
	AccessExpiresAtMS         int64  `json:"access_expires_at_ms"`
	ChallengeID               string `json:"challenge_id"`
	TranscriptDigest          string `json:"transcript_digest"`
}

func (ReceiptV2) String() string   { return "[private untrusted receipt-v2 DATA]" }
func (ReceiptV2) GoString() string { return "[private untrusted receipt-v2 DATA]" }

// ReceiptV2Bytes reproduces the stored recipe exactly, including fixed descriptor
// version text. It intentionally has NO validation/authentication. Existing
// Store namespace validation owns historical compatibility; new native documents
// additionally validate all fields with CheckNativeCredentialDocument.
func ReceiptV2Bytes(r ReceiptV2) []byte {
	var out []byte
	field := func(b []byte) { out = binary.BigEndian.AppendUint32(out, uint32(len(b))); out = append(out, b...) }
	for _, s := range []string{"mnet/store-enrollment-receipt/v2", r.Issuer, r.RegistrarOrigin, r.Audience, r.ServiceID, r.WorkspaceID, r.MappingRevision, r.HandoffID, r.AttemptID, r.ReserveID, r.DescriptorDigest, "1", r.ReceiptID, r.NodeID, r.NetworkID, r.DevicePublicKey, r.CredentialDigest} {
		field([]byte(s))
	}
	for _, n := range []int64{r.CommittedAtMS, r.AccessExpiresAtMS} {
		field(binary.BigEndian.AppendUint64(nil, uint64(n)))
	}
	field([]byte(r.ChallengeID))
	field([]byte(r.TranscriptDigest))
	return out
}
func ReceiptV2Digest(r ReceiptV2) string {
	sum := sha256.Sum256(ReceiptV2Bytes(r))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ReceiptV2ForChallenge constructs ONLY syntactically checked DATA. No verified
// provenance or clock is supplied by this function, no Store state is consumed.
func ReceiptV2ForChallenge(c Challenge, nodeID, receiptID string, committedMS int64) (ReceiptV2, error) {
	transcript, err := Encode(c)
	if err != nil || !text(nodeID) || committedMS < c.Binding.CheckedAtMS || committedMS >= c.ExpiresAtMS || committedMS >= c.Binding.AccessUntilMS {
		return ReceiptV2{}, ErrInvalid
	}
	if _, err := digest(receiptID); err != nil {
		return ReceiptV2{}, err
	}
	b := c.Binding
	key, _ := base64.RawURLEncoding.Strict().DecodeString(b.DevicePublicKey)
	sum := sha256.Sum256(transcript)
	return ReceiptV2{Issuer: b.IssuerLabel, RegistrarOrigin: b.RegistrarOrigin, Audience: b.Audience, ServiceID: b.ServiceID, WorkspaceID: b.WorkspaceID, MappingRevision: b.MappingRevision, HandoffID: b.HandoffID, AttemptID: b.AttemptID, ReserveID: b.ReserveID, DescriptorDigest: b.DescriptorDigest, DescriptorEncodingVersion: b.DescriptorEncodingVersion, ReceiptID: receiptID, NodeID: nodeID, NetworkID: b.NetworkID, DevicePublicKey: base64.StdEncoding.EncodeToString(key), CredentialDigest: b.CredentialDigest, CommittedAtMS: committedMS, AccessExpiresAtMS: b.AccessUntilMS, ChallengeID: c.ID, TranscriptDigest: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}
