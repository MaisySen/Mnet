// Package keypossession is an inactive enrollment protocol experiment for
// security review. No network route, daemon caller, person verification,
// registration or authorization is implemented. A verified response proves
// possession of a key only, never permission to join or control a device.
package keypossession

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const Lifetime = 2 * time.Minute
const domain = "mnet/enrollment-key-possession/experimental-v1"

var ErrInvalid = errors.New("key possession proof refused")

// Binding must come from independent trusted configuration and a fresh person
// decision. The experiment does not establish any of those facts. Identifiers
// and public keys are transcript data, not credentials or membership evidence.
type Binding struct {
	RegistrarOrigin, Issuer, Subject, WorkspaceID, NetworkID, AuthorizationID string
	DevicePublicKey                                                           [32]byte
}

// Challenge contains public transcript data only. It has no user authorization.
type Challenge struct {
	ID, Nonce, EphemeralPublicKey [32]byte
	Binding                       Binding
	ExpiresUnixMilli              int64
}

// Pending owns one experimental challenge. It is not persisted. Consumption
// here is NOT atomic with a future registrar transaction. Do not wire this to
// node creation without designing durable consumption and lost-response recovery.
type Pending struct {
	mu        sync.Mutex
	challenge Challenge
	private   *ecdh.PrivateKey
	used      bool
}

func (*Pending) String() string   { return "[private key-possession experiment]" }
func (*Pending) GoString() string { return "[private key-possession experiment]" }
func (*Pending) MarshalJSON() ([]byte, error) {
	return []byte(`"[private key-possession experiment]"`), nil
}

func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || u.Path != "" || u.Opaque != "" {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))
}
func validBinding(b Binding) bool {
	if !validOrigin(b.RegistrarOrigin) || !validOrigin(b.Issuer) {
		return false
	}
	for _, s := range []string{b.Subject, b.WorkspaceID, b.NetworkID, b.AuthorizationID} {
		if len(s) < 1 || len(s) > 256 || !utf8.ValidString(s) || strings.TrimSpace(s) != s || strings.ContainsAny(s, "\x00\r\n") {
			return false
		}
	}
	return b.DevicePublicKey != [32]byte{}
}
func validChallenge(c Challenge, expected Binding, now time.Time) bool {
	left := time.UnixMilli(c.ExpiresUnixMilli).Sub(now)
	return validBinding(expected) && c.Binding == expected && c.ID != [32]byte{} && c.Nonce != [32]byte{} && left > 0 && left <= Lifetime
}
func transcript(c Challenge) []byte {
	// A fixed field-order tuple gives unambiguous boundaries for every string,
	// including unusual subject identifiers. No delimiter concatenation.
	b, _ := json.Marshal([]any{domain, c.ID, c.Nonce, c.EphemeralPublicKey, c.Binding.RegistrarOrigin, c.Binding.Issuer, c.Binding.Subject, c.Binding.WorkspaceID, c.Binding.NetworkID, c.Binding.AuthorizationID, c.Binding.DevicePublicKey, c.ExpiresUnixMilli})
	return b
}
func authenticator(shared []byte, c Challenge) ([32]byte, error) {
	var out [32]byte
	context := transcript(c)
	key, err := hkdf.Key(sha256.New, shared, c.Nonce[:], domain+"/derived-mac-key", 32)
	if err != nil {
		return out, ErrInvalid
	}
	defer clear(key)
	mac := hmac.New(sha256.New, key)
	mac.Write(context)
	copy(out[:], mac.Sum(nil))
	return out, nil
}

// New creates only an ephemeral proof challenge, with no store or HTTP effects.
func New(b Binding, now time.Time) (*Pending, Challenge, error) {
	if !validBinding(b) || now.IsZero() {
		return nil, Challenge{}, ErrInvalid
	}
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, Challenge{}, ErrInvalid
	}
	peer, err := ecdh.X25519().NewPublicKey(b.DevicePublicKey[:])
	if err != nil {
		return nil, Challenge{}, ErrInvalid
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		return nil, Challenge{}, ErrInvalid
	}
	clear(shared)
	c := Challenge{Binding: b, ExpiresUnixMilli: now.Add(Lifetime).UnixMilli()}
	copy(c.EphemeralPublicKey[:], private.PublicKey().Bytes())
	if _, err = rand.Read(c.ID[:]); err != nil {
		return nil, Challenge{}, ErrInvalid
	}
	if _, err = rand.Read(c.Nonce[:]); err != nil {
		return nil, Challenge{}, ErrInvalid
	}
	return &Pending{challenge: c, private: private}, c, nil
}

// Respond must receive the expected binding independently of the challenge.
// A future caller must first authenticate the exact trusted registrar channel;
// this function does not fetch a URL, open a browser or prove server identity.
func Respond(privateKey [32]byte, c Challenge, expected Binding, now time.Time) ([32]byte, error) {
	if !validChallenge(c, expected, now) {
		return [32]byte{}, ErrInvalid
	}
	private, err := ecdh.X25519().NewPrivateKey(privateKey[:])
	if err != nil {
		return [32]byte{}, ErrInvalid
	}
	if !hmac.Equal(private.PublicKey().Bytes(), expected.DevicePublicKey[:]) {
		return [32]byte{}, ErrInvalid
	}
	peer, err := ecdh.X25519().NewPublicKey(c.EphemeralPublicKey[:])
	if err != nil {
		return [32]byte{}, ErrInvalid
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		return [32]byte{}, ErrInvalid
	}
	defer clear(shared)
	return authenticator(shared, c)
}

// Verify consumes the exact ephemeral challenge on success. Wrong proofs do
// not consume it. This is not a grant or durable registration transaction.
func (p *Pending) Verify(c Challenge, response [32]byte, now time.Time) error {
	if p == nil {
		return ErrInvalid
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.used && p.private != nil && !time.UnixMilli(p.challenge.ExpiresUnixMilli).After(now) {
		p.used = true
		p.private = nil
	}
	if p.used || p.private == nil || c != p.challenge || !validChallenge(c, p.challenge.Binding, now) {
		return ErrInvalid
	}
	peer, err := ecdh.X25519().NewPublicKey(c.Binding.DevicePublicKey[:])
	if err != nil {
		return ErrInvalid
	}
	shared, err := p.private.ECDH(peer)
	if err != nil {
		return ErrInvalid
	}
	defer clear(shared)
	expected, err := authenticator(shared, c)
	if err != nil || !hmac.Equal(expected[:], response[:]) {
		return ErrInvalid
	}
	p.used = true
	p.private = nil
	return nil
}

// Cancel retires the in-memory experiment. It grants/revokes no account or node
// authority. Dropping the reference is not a secure-memory-erasure guarantee.
func (p *Pending) Cancel() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used = true
	p.private = nil
}
