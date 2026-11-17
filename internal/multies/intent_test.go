package multies

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func intentFixture() (Intent, string) {
	verifier := strings.Repeat("v", 43)
	challenge := sha256.Sum256([]byte(verifier))
	device := sha256.Sum256([]byte("fixture device"))
	return Intent{WorkspaceID: "workspace-one", DeviceKey: base64.RawURLEncoding.EncodeToString(device[:]), Challenge: base64.RawURLEncoding.EncodeToString(challenge[:]), Callback: AndroidCallback}, verifier
}
func TestIntentIsBoundOneUseAndNotAuthority(t *testing.T) {
	s, _ := NewIntents(2, nil)
	intent, verifier := intentFixture()
	token, err := s.Begin(intent)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Take(token.Reveal(), verifier, intent)
	if err != nil || got != intent {
		t.Fatal("binding failed")
	}
	if _, err = s.Take(token.Reveal(), verifier, intent); err != ErrDenied {
		t.Fatal("replay allowed")
	}
	// Intent redemption never produces a Grant or credentials. Zero-policy denial
	// remains in force even after valid native proof, with no verifier installed.
	if grant, err := (Policy{}).Authorize(t.Context(), Request{Credential: token.Reveal(), WorkspaceID: intent.WorkspaceID, DeviceKey: intent.DeviceKey, Capability: Enroll}); err != ErrDenied || grant != (Grant{}) {
		t.Fatal("intent became authority")
	}
}
func TestIntentRejectsCrossBindingWithoutConsumingOwner(t *testing.T) {
	for _, field := range []string{"workspace", "device", "callback", "challenge", "proof", "token"} {
		t.Run(field, func(t *testing.T) {
			s, _ := NewIntents(1, nil)
			original, verifier := intentFixture()
			token, _ := s.Begin(original)
			expected := original
			supplied := token.Reveal()
			switch field {
			case "workspace":
				expected.WorkspaceID = "different"
			case "device":
				sum := sha256.Sum256([]byte("other"))
				expected.DeviceKey = base64.RawURLEncoding.EncodeToString(sum[:])
			case "callback":
				expected.Callback = IOSCallback
			case "challenge":
				sum := sha256.Sum256([]byte("other"))
				expected.Challenge = base64.RawURLEncoding.EncodeToString(sum[:])
			case "proof":
				verifier = strings.Repeat("x", 43)
			case "token":
				sum := sha256.Sum256([]byte("other token"))
				supplied = base64.RawURLEncoding.EncodeToString(sum[:])
			}
			if got, err := s.Take(supplied, verifier, expected); err != ErrDenied || got != (Intent{}) {
				t.Fatal("binding mismatch allowed")
			}
			_, correct := intentFixture()
			if _, err := s.Take(token.Reveal(), correct, original); err != nil {
				t.Fatal("wrong attempt consumed owner")
			}
		})
	}
}
func TestIntentExpiryCapacityAndCancellation(t *testing.T) {
	now := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	s, _ := NewIntents(1, func() time.Time { return now })
	intent, verifier := intentFixture()
	first, _ := s.Begin(intent)
	if _, err := s.Begin(intent); err != ErrDenied {
		t.Fatal("capacity exceeded")
	}
	now = now.Add(IntentLifetime)
	if got, err := s.Take(first.Reveal(), verifier, intent); err != ErrDenied || got != (Intent{}) {
		t.Fatal("expired attempt redeemed")
	}
	second, err := s.Begin(intent)
	if err != nil {
		t.Fatal("expired capacity leaked")
	}
	s.Cancel(second.Reveal())
	s.Cancel(second.Reveal())
	if _, err := s.Take(second.Reveal(), verifier, intent); err != ErrDenied {
		t.Fatal("cancelled attempt revived")
	}
	if _, err := s.Begin(intent); err != nil {
		t.Fatal("cancelled capacity leaked")
	}
}
func TestIntentConcurrentRedemptionHasSingleWinner(t *testing.T) {
	s, _ := NewIntents(1, nil)
	intent, verifier := intentFixture()
	token, _ := s.Begin(intent)
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Take(token.Reveal(), verifier, intent); err == nil {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("winners%d", winners.Load())
	}
}
func TestIntentRejectsUnsafeCallbacksAndMalformedPKCE(t *testing.T) {
	intent, _ := intentFixture()
	for _, callback := range []Callback{"", "https://untrusted.invalid/callback", "android://anything", "../desktop", "Android"} {
		copy := intent
		copy.Callback = callback
		s, _ := NewIntents(1, nil)
		if _, err := s.Begin(copy); err != ErrDenied {
			t.Fatal("arbitrary callback allowed")
		}
	}
	for _, proof := range []string{"", strings.Repeat("v", 42), strings.Repeat("v", 129), strings.Repeat("v", 42) + " ", strings.Repeat("v", 42) + "\n", strings.Repeat("v", 42) + "+"} {
		if validVerifier(proof) {
			t.Fatal("unsafe verifier accepted")
		}
	}
	for _, capacity := range []int{-1, 0, MaxPendingIntents + 1} {
		if _, err := NewIntents(capacity, nil); err != ErrDenied {
			t.Fatal("unbounded capacity")
		}
	}
	var zero Intents
	if _, err := zero.Begin(intent); err != ErrDenied {
		t.Fatal("zero store usable")
	}
	if _, err := zero.Take(strings.Repeat("x", 43), strings.Repeat("v", 43), intent); err != ErrDenied {
		t.Fatal("zero store redeemable")
	}
	zero.Cancel(strings.Repeat("x", 43))
	var missing *Intents
	if _, err := missing.Begin(intent); err != ErrDenied {
		t.Fatal("nil store usable")
	}
}
func TestIntentCorrelationTokensAreRedactedAndHashed(t *testing.T) {
	s, _ := NewIntents(2, nil)
	intent, _ := intentFixture()
	first, _ := s.Begin(intent)
	second, _ := s.Begin(intent)
	if first.Reveal() == second.Reveal() || !validDeviceKey(first.Reveal()) {
		t.Fatal("invalid randomness")
	}
	for _, rendered := range []string{fmt.Sprint(first), fmt.Sprintf("%v", first), fmt.Sprintf("%#v", first)} {
		if strings.Contains(rendered, first.Reveal()) {
			t.Fatal("token logged")
		}
	}
	hash := sha256.Sum256([]byte(first.Reveal()))
	if _, ok := s.pending[hash]; !ok {
		t.Fatal("token not indexed by hash")
	}
}
func FuzzIntentCannotBroadenBinding(f *testing.F) {
	f.Add("workspace-one", "android", strings.Repeat("v", 43))
	f.Fuzz(func(t *testing.T, workspace, callback, proof string) {
		s, _ := NewIntents(1, nil)
		intent, _ := intentFixture()
		token, _ := s.Begin(intent)
		attempt := intent
		attempt.WorkspaceID = workspace
		attempt.Callback = Callback(callback)
		got, err := s.Take(token.Reveal(), proof, attempt)
		if err == nil && (got != intent || attempt != intent || !validVerifier(proof)) {
			t.Fatal("binding expanded")
		}
	})
}
