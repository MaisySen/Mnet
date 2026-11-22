package control

// Disconnected private verifier for EXISTING strict v2 consumed-proof history.
// No route, external scheme/parser, node creator, permit or native consumer.
import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"time"

	"fintex.au/mnet/internal/netmap"
)

// Exact qualified historical record reference DATA. Caller strings authenticate
// nothing; secret must match SAME owned record and current node must be usable.
// Full scope prevents moving a credential to another receipt/service/network.
type registrarCredentialReference struct {
	NodeID, NetworkID, DevicePublicKey                           string
	Scope                                                        registrarScope
	ReceiptID, ReceiptDigest, DescriptorDigest, TranscriptDigest string
}

func (registrarCredentialReference) String() string {
	return "[private registrar credential reference DATA]"
}
func (registrarCredentialReference) GoString() string {
	return "[private registrar credential reference DATA]"
}

// authorizeRegistrarCredential returns a detached current node snapshot only.
// A snapshot is not a continuous access lease; eventual map/relay use must
// recheck under its owning lifecycle, as legacy authority reads already do.
func (s *Store) authorizeRegistrarCredential(ref registrarCredentialReference, secret string) (*netmap.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authorizeRegistrarCredentialLocked(ref, secret)
}

// Requires Store.mu. Returned clone never substitutes for holding owner lock
// across a subsequent protected operation.
func (s *Store) authorizeRegistrarCredentialLocked(ref registrarCredentialReference, secret string) (*netmap.Node, error) {
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	if !registrarCredentialSecret(secret) {
		return nil, ErrUnauthorized
	}
	n, err := s.currentRegistrarPublicationOwnerLocked(ref)
	if err != nil {
		return nil, err
	}
	r := s.registrarNodeLocked(ref.NodeID)
	expected, err := base64.RawURLEncoding.Strict().DecodeString(r.CredentialDigest)
	if err != nil || len(expected) != 32 {
		return nil, ErrUnauthorized
	}
	// Matches actual Multies SHA256(TextEncoder canonical43ASCII secret), NOT
	// decoded secret bytes, digest-keyed HMAC or legacy token equality.
	actual := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return nil, ErrUnauthorized
	}
	return n, nil
}

// Requires Store.mu. This validates an already authenticated, privately retained
// publication's owner. It is NOT a credential verifier or an externally callable
// authorization path. Only authorizeRegistrarCredentialLocked checks the secret.
func (s *Store) currentRegistrarPublicationOwnerLocked(ref registrarCredentialReference) (*netmap.Node, error) {
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	if s.data.Registrar == nil || s.data.Registrar.Version != 2 {
		return nil, ErrUnauthorized
	}
	// Revalidate the actual existing namespace, including mixed account identity,
	// routes and permanent indexes. This currently costs O(total history); a
	// production request path needs reviewed bounds/caching, not a skipped check.
	if s.validateRegistrarState() != nil {
		return nil, ErrUnauthorized
	}
	r := s.registrarNodeLocked(ref.NodeID)
	if r == nil || r.ReceiptEncodingVersion != 2 || r.Consumption == nil || r.Phase != "active" {
		return nil, ErrUnauthorized
	}
	if ref.NodeID != r.NodeID || ref.NetworkID != r.NetworkID || ref.DevicePublicKey != r.DevicePublicKey || ref.Scope != r.Scope || ref.ReceiptID != r.ReceiptID || ref.ReceiptDigest != r.ReceiptDigest || ref.DescriptorDigest != r.DescriptorDigest || ref.TranscriptDigest != r.Consumption.TranscriptDigest {
		return nil, ErrUnauthorized
	}
	// Recheck exact authoritative historical indexes/receipt/binding, not a bare
	// digest taken from caller or a source response. No silent repair on failure.
	index := registrarClaimKey(r.NetworkID, r.DevicePublicKey)
	if s.data.Registrar.Records[index] != r || validateRegistrarConsumption(s.data.Registrar, index, r) != nil || registrarReceiptDigest(*r) != r.ReceiptDigest {
		return nil, ErrUnauthorized
	}
	if _, mixed := s.data.NodeTokens[r.NodeID]; mixed {
		return nil, ErrUnauthorized
	}
	n := s.data.Nodes[r.NodeID]
	now := time.Now().UnixMilli()
	if n == nil || s.data.Networks[r.NetworkID] == nil || n.ID != r.NodeID || n.NetworkID != r.NetworkID || n.PubKey != r.DevicePublicKey || !n.Approved || n.KeyExpiry == nil || n.KeyExpiry.UnixMilli() != r.AccessExpiresAtMS || now < r.CommittedAtMS || now >= r.AccessExpiresAtMS {
		return nil, ErrUnauthorized
	}
	copy := cloneNode(n)
	return &copy, nil
}

// Match the actual native journal token grammar, not the looser historical
// digest grammar. All-zero32-byte values are not generated native credentials.
func registrarCredentialSecret(secret string) bool {
	if !registrarDigest(secret) {
		return false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(secret)
	if err != nil {
		return false
	}
	var combined byte
	for _, v := range raw {
		combined |= v
	}
	return combined != 0
}
