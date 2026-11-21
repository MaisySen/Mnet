// Package enrollmentproof is a disconnected experimental-v4 crypto engine.
// It authenticates no person, source, service, channel, config or eligibility.
// No production caller, bootstrap route, permit or durable consumption exists.
package enrollmentproof

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

var ErrInvalid = errors.New("inactive v4 key proof refused")

// Server is a copy-safe ephemeral owner. It issues NO authority and does NOT
// consume proof on verification; future Store consumption must be atomic with
// its own transaction. Retries use this exact challenge and original expiry.
type Server struct{ *server }
type server struct {
	mu         sync.Mutex
	challenge  contract.Challenge
	private    *ecdh.PrivateKey
	closed     bool
	admittedMS int64
}

func (Server) String() string   { return "[private inactive v4 proof server]" }
func (Server) GoString() string { return "[private inactive v4 proof server]" }

// NewServer requires DATA plus explicit challenge expiry/lifetime bound. It is
// not a verified-binding constructor. An eventual authenticated owner must first
// admit request/capacity/current policy; this helper supplies none of those.
// Global live/consumed ID collision policy belongs to that future owner.
func NewServer(binding contract.Binding, now, expires time.Time, maxLifetime time.Duration) (*Server, contract.Challenge, error) {
	if now.IsZero() || maxLifetime <= 0 || maxLifetime > time.Minute*15 || !expires.After(now) || expires.Sub(now) > maxLifetime {
		return nil, contract.Challenge{}, ErrInvalid
	}
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, contract.Challenge{}, ErrInvalid
	}
	id, err := opaque()
	if err != nil {
		return nil, contract.Challenge{}, err
	}
	nonce, err := opaque()
	if err != nil {
		return nil, contract.Challenge{}, err
	}
	c := contract.Challenge{Binding: binding, ID: id, Nonce: nonce, EphemeralPublicKey: encode(private.PublicKey().Bytes()), ExpiresAtMS: expires.UnixMilli()}
	if !live(c, binding, now, maxLifetime) {
		return nil, contract.Challenge{}, ErrInvalid
	}
	peer, err := decodeKey(binding.DevicePublicKey)
	if err != nil {
		return nil, contract.Challenge{}, err
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		return nil, contract.Challenge{}, ErrInvalid
	}
	clear(shared)
	return &Server{&server{challenge: c, private: private, admittedMS: now.UnixMilli()}}, c, nil
}
func (s *Server) Close() {
	if s == nil || s.server == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.private = nil
}

// Verify returns only DH server confirmation after exact owned-ID/MAC check.
// It is deliberately side-effect-free for durable-consumption composition.
// Confirmation proves shared-key knowledge, NOT a committed/enrolled node.
func (s *Server) Verify(challengeID string, response [32]byte, now time.Time) ([32]byte, error) {
	if s == nil || s.server == nil {
		return [32]byte{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.challenge
	if s.closed || s.private == nil || challengeID != c.ID || now.IsZero() || now.UnixMilli() < s.admittedMS || now.UnixMilli() >= c.ExpiresAtMS {
		return [32]byte{}, ErrInvalid
	}
	peer, err := decodeKey(c.Binding.DevicePublicKey)
	if err != nil {
		return [32]byte{}, ErrInvalid
	}
	shared, err := s.private.ECDH(peer)
	if err != nil {
		return [32]byte{}, ErrInvalid
	}
	defer clear(shared)
	expected, err := responseMAC(shared, c)
	if err != nil || !hmac.Equal(expected[:], response[:]) {
		return [32]byte{}, ErrInvalid
	}
	return confirmationMAC(shared, c, response)
}

// Native is an outstanding exact response owner, not a channel-authentication or
// native-State owner token. A real daemon must gate all use of its durable key
// on device-initiated authenticated exact registrar delivery and own PKCE state.
type Native struct{ *native }
type native struct {
	mu         sync.Mutex
	challenge  contract.Challenge
	shared     [32]byte
	response   [32]byte
	admittedMS int64
	closed     bool
}

func (Native) String() string   { return "[private inactive v4 native response]" }
func (Native) GoString() string { return "[private inactive v4 native response]" }

// Respond compares full independent expected Binding BEFORE deriving anything.
// There is no challenge parser/network/origin inference or native key getter.
func Respond(privateKey [32]byte, c contract.Challenge, expected contract.Binding, now time.Time, maxLifetime time.Duration) (*Native, error) {
	if !live(c, expected, now, maxLifetime) {
		return nil, ErrInvalid
	}
	private, err := ecdh.X25519().NewPrivateKey(privateKey[:])
	if err != nil {
		return nil, ErrInvalid
	}
	defer clear(privateKey[:])
	if encode(private.PublicKey().Bytes()) != expected.DevicePublicKey {
		return nil, ErrInvalid
	}
	peer, err := decodeKey(c.EphemeralPublicKey)
	if err != nil {
		return nil, ErrInvalid
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		return nil, ErrInvalid
	}
	defer clear(shared)
	response, err := responseMAC(shared, c)
	if err != nil {
		return nil, err
	}
	n := &Native{&native{challenge: c, response: response, admittedMS: now.UnixMilli()}}
	copy(n.shared[:], shared)
	return n, nil
}
func (n *Native) Close() {
	if n == nil || n.native == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.closed = true
	clear(n.shared[:])
	clear(n.response[:])
}
func (n *Native) Response(now time.Time) ([32]byte, error) {
	if n == nil || n.native == nil {
		return [32]byte{}, ErrInvalid
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.usable(now) {
		return [32]byte{}, ErrInvalid
	}
	return n.response, nil
}

// Confirm checks only this outstanding exact transcript/response. Even success
// never means durable acceptance or current network access. Repeated exact
// confirmation is harmless knowledge check, not consumed enrollment authority.
func (n *Native) Confirm(confirmation [32]byte, now time.Time) error {
	if n == nil || n.native == nil {
		return ErrInvalid
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.usable(now) {
		return ErrInvalid
	}
	expected, err := confirmationMAC(n.shared[:], n.challenge, n.response)
	if err != nil || !hmac.Equal(expected[:], confirmation[:]) {
		return ErrInvalid
	}
	return nil
}
func (n *Native) usable(now time.Time) bool {
	return !n.closed && !now.IsZero() && now.UnixMilli() >= n.admittedMS && now.UnixMilli() < n.challenge.ExpiresAtMS
}
func live(c contract.Challenge, expected contract.Binding, now time.Time, maxLifetime time.Duration) bool {
	if now.IsZero() || maxLifetime <= 0 || maxLifetime > 15*time.Minute || c.Binding != expected || now.UnixMilli() < c.Binding.CheckedAtMS || c.ExpiresAtMS <= now.UnixMilli() {
		return false
	}
	// CheckedAt and now must both be within supplied maximum. This does not
	// authenticate source clock or choose an eligibility/lease policy.
	if c.ExpiresAtMS-c.Binding.CheckedAtMS > maxLifetime.Milliseconds() || c.ExpiresAtMS-now.UnixMilli() > maxLifetime.Milliseconds() {
		return false
	}
	_, err := contract.Encode(c)
	return err == nil
}
func decodeKey(s string) (*ecdh.PublicKey, error) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(raw) != 32 || encode(raw) != s {
		return nil, ErrInvalid
	}
	p, err := ecdh.X25519().NewPublicKey(raw)
	if err != nil {
		return nil, ErrInvalid
	}
	return p, nil
}
func encode(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }
func opaque() (string, error) {
	for i := 0; i < 4; i++ {
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", ErrInvalid
		}
		if b != [32]byte{} {
			return encode(b[:]), nil
		}
	}
	return "", ErrInvalid
}
func derive(shared []byte, c contract.Challenge, label string) ([]byte, error) {
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(c.Nonce)
	if err != nil || len(nonce) != 32 {
		return nil, ErrInvalid
	}
	key, err := hkdf.Key(sha256.New, shared, nonce, contract.Domain+label, 32)
	if err != nil {
		return nil, ErrInvalid
	}
	return key, nil
}
func responseMAC(shared []byte, c contract.Challenge) ([32]byte, error) {
	var out [32]byte
	raw, err := contract.Encode(c)
	if err != nil {
		return out, ErrInvalid
	}
	key, err := derive(shared, c, "/response-mac-key")
	if err != nil {
		return out, err
	}
	defer clear(key)
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	copy(out[:], mac.Sum(nil))
	return out, nil
}
func confirmationMAC(shared []byte, c contract.Challenge, response [32]byte) ([32]byte, error) {
	var out [32]byte
	raw, err := contract.Encode(c)
	if err != nil {
		return out, ErrInvalid
	}
	digest := sha256.Sum256(raw)
	key, err := derive(shared, c, "/server-confirmation-key")
	if err != nil {
		return out, err
	}
	defer clear(key)
	mac := hmac.New(sha256.New, key)
	for _, part := range [][]byte{[]byte(contract.Domain + "/server-confirmation"), digest[:], response[:]} {
		mac.Write(binary.BigEndian.AppendUint32(nil, uint32(len(part))))
		mac.Write(part)
	}
	copy(out[:], mac.Sum(nil))
	return out, nil
}
