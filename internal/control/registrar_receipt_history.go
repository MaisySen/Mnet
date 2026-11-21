package control

// Private historical recovery only, not current access, enrollment completion,
// a commit producer, source lease, native installation or registered endpoint.
import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

// No current/enrolled/approved flag: retained receipt is immutable history even
// when the node was revoked, expired or deleted. Values are detached DATA.
type registrarReceiptHistory struct {
	Receipt contract.ReceiptV2
	Digest  string
}

func (registrarReceiptHistory) String() string { return "[private registrar historical receipt DATA]" }
func (registrarReceiptHistory) GoString() string {
	return "[private registrar historical receipt DATA]"
}

// registrarHistoricalReceipt uses the independently pinned FULL challenge and
// original presaved secret, not a response-selected node/receipt reference. It
// cannot allocate, revive, renew, save or bypass existing current credential
// checks. Missing/changed/invalid records all refuse, never imply safe-to-retry.
// Full namespace validation remains O(history), and blocks future ingress use
// until reviewed cost bounds exist. There is no production caller.
func (s *Store) registrarHistoricalReceipt(pinned contract.Challenge, secret string) (registrarReceiptHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var empty registrarReceiptHistory
	if err := s.healthLocked(); err != nil {
		return empty, err
	}
	if !registrarCredentialSecret(secret) || s.data.Registrar == nil || s.data.Registrar.Version != 2 {
		return empty, ErrUnauthorized
	}
	if _, err := contract.Encode(pinned); err != nil {
		return empty, ErrUnauthorized
	}
	if s.validateRegistrarState() != nil {
		return empty, ErrUnauthorized
	}
	key, err := base64.RawURLEncoding.Strict().DecodeString(pinned.Binding.DevicePublicKey)
	if err != nil || len(key) != 32 {
		return empty, ErrUnauthorized
	}
	index := registrarClaimKey(pinned.Binding.NetworkID, base64.StdEncoding.EncodeToString(key))
	r := s.data.Registrar.Records[index]
	if r == nil || r.ReceiptEncodingVersion != 2 || r.Consumption == nil || r.Consumption.Challenge != pinned {
		return empty, ErrUnauthorized
	}
	if s.data.Registrar.ConsumedProofs[registrarProofKey(pinned)] != index || r.Consumption.TranscriptDigest != registrarTranscriptDigest(pinned) {
		return empty, ErrUnauthorized
	}
	actual := sha256.Sum256([]byte(secret))
	expected, err := base64.RawURLEncoding.Strict().DecodeString(r.CredentialDigest)
	if err != nil || len(expected) != 32 || subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return empty, ErrUnauthorized
	}
	// Reconstruct exact existing v2 bytes, never generate a new identity or time.
	receipt, err := contract.ReceiptV2ForChallenge(pinned, r.NodeID, r.ReceiptID, r.CommittedAtMS)
	if err != nil || contract.ReceiptV2Digest(receipt) != r.ReceiptDigest {
		return empty, ErrUnauthorized
	}
	return registrarReceiptHistory{Receipt: receipt, Digest: r.ReceiptDigest}, nil
}
