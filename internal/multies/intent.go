package multies

// This is an inactive, Mnet-side handoff-binding primitive. It is not a Multies
// bridge or an authorization code issuer. An intent carries no user identity,
// membership, approval, session, grant or device credential. A successful Take
// proves only possession of the original native client's PKCE verifier.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
)

const IntentLifetime = 2 * time.Minute
const MaxPendingIntents = 1024

// Callback is a compiled-in consumer route, never an arbitrary URL. Final
// callback URLs/app association still require the actual bridge contract.
type Callback string

const (
	AndroidCallback Callback = "android"
	IOSCallback     Callback = "ios"
	DesktopCallback Callback = "desktop-loopback"
)

type Intent struct {
	WorkspaceID string
	DeviceKey   string
	Challenge   string // canonical S256 SHA-256 challenge, no plain PKCE
	Callback    Callback
}

type pendingIntent struct {
	intent  Intent
	expires time.Time
}
type Intents struct {
	mu       sync.Mutex
	now      func() time.Time
	capacity int
	pending  map[[32]byte]pendingIntent
}

// IntentToken is an opaque correlation secret. It must travel only through a
// protected handoff, never logs, telemetry, referrers or a public query string.
// Its String/GoString representation is redacted; Reveal is for that future
// transport only. No production caller exists.
type IntentToken struct{ value string }

func (IntentToken) String() string   { return "[redacted intent]" }
func (IntentToken) GoString() string { return "[redacted intent]" }
func (t IntentToken) Reveal() string { return t.value }

func NewIntents(capacity int, now func() time.Time) (*Intents, error) {
	if capacity < 1 || capacity > MaxPendingIntents {
		return nil, ErrDenied
	}
	if now == nil {
		now = time.Now
	}
	return &Intents{now: now, capacity: capacity, pending: make(map[[32]byte]pendingIntent)}, nil
}
func validIntent(i Intent) bool {
	if !idPattern.MatchString(i.WorkspaceID) || !validDeviceKey(i.DeviceKey) || !validDeviceKey(i.Challenge) {
		return false
	}
	return i.Callback == AndroidCallback || i.Callback == IOSCallback || i.Callback == DesktopCallback
}
func (s *Intents) prune(now time.Time) {
	for key, value := range s.pending {
		if !value.expires.After(now) {
			delete(s.pending, key)
		}
	}
}

// Begin stores only the intent and a hash of its unguessable correlation token.
// Queue capacity does not evict a different unexpired user's pending intent.
func (s *Intents) Begin(i Intent) (IntentToken, error) {
	if s == nil || s.now == nil || s.capacity < 1 || s.pending == nil || !validIntent(i) {
		return IntentToken{}, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now)
	if len(s.pending) >= s.capacity {
		return IntentToken{}, ErrDenied
	}
	for attempts := 0; attempts < 3; attempts++ {
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return IntentToken{}, ErrDenied
		}
		token := base64.RawURLEncoding.EncodeToString(raw[:])
		key := sha256.Sum256([]byte(token))
		if _, exists := s.pending[key]; exists {
			continue
		}
		s.pending[key] = pendingIntent{intent: i, expires: now.Add(IntentLifetime)}
		return IntentToken{value: token}, nil
	}
	return IntentToken{}, ErrDenied
}
func validVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
			return false
		}
	}
	return true
}

// Take atomically consumes the exact bound intent. Wrong proof/binding cannot
// consume somebody else's attempt. It is not evidence of browser approval or
// current Multies authority: the trusted bridge and Policy are still required.
func (s *Intents) Take(token, verifier string, expected Intent) (Intent, error) {
	if s == nil || s.now == nil || s.capacity < 1 || s.pending == nil || !validDeviceKey(token) || !validVerifier(verifier) || !validIntent(expected) {
		return Intent{}, ErrDenied
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(expected.Challenge)) != 1 {
		return Intent{}, ErrDenied
	}
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now)
	stored, ok := s.pending[key]
	if !ok || stored.intent != expected {
		return Intent{}, ErrDenied
	}
	delete(s.pending, key)
	return stored.intent, nil
}

// Cancel is idempotent and has no external effects. It does not revoke a device.
func (s *Intents) Cancel(token string) {
	if s == nil || s.now == nil || s.capacity < 1 || s.pending == nil || !validDeviceKey(token) {
		return
	}
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, key)
}

var _ fmt.Stringer = IntentToken{}
