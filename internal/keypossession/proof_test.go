package keypossession

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) ([32]byte, Binding, time.Time) {
	t.Helper()
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var secret, public [32]byte
	copy(secret[:], k.Bytes())
	copy(public[:], k.PublicKey().Bytes())
	return secret, Binding{RegistrarOrigin: "https://registrar.example.test", Issuer: "https://multies.example.test", Subject: "person", WorkspaceID: "workspace", NetworkID: "network", AuthorizationID: "one-approved-decision", DevicePublicKey: public}, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
}
func TestProofOneWinnerAndNoReplay(t *testing.T) {
	secret, b, now := fixture(t)
	p, c, err := New(b, now)
	if err != nil {
		t.Fatal(err)
	}
	response, err := Respond(secret, c, b, now)
	if err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if p.Verify(c, response, now) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("winners=%d", wins.Load())
	}
	if p.Verify(c, response, now) == nil {
		t.Fatal("replayed")
	}
	if p.private != nil {
		t.Fatal("consumed challenge retained private state")
	}
}
func TestProofWrongBindingDoesNotConsumeOriginal(t *testing.T) {
	secret, b, now := fixture(t)
	for _, mutate := range []func(*Challenge){
		func(c *Challenge) { c.ID[0] ^= 1 }, func(c *Challenge) { c.Nonce[0] ^= 1 }, func(c *Challenge) { c.EphemeralPublicKey[0] ^= 1 },
		func(c *Challenge) { c.Binding.RegistrarOrigin = "https://other.test" }, func(c *Challenge) { c.Binding.Issuer = "https://other.test" },
		func(c *Challenge) { c.Binding.Subject += "other" }, func(c *Challenge) { c.Binding.WorkspaceID += "other" },
		func(c *Challenge) { c.Binding.NetworkID += "other" }, func(c *Challenge) { c.Binding.AuthorizationID += "other" },
		func(c *Challenge) { c.Binding.DevicePublicKey[0] ^= 1 }, func(c *Challenge) { c.ExpiresUnixMilli++ },
	} {
		p, c, err := New(b, now)
		if err != nil {
			t.Fatal(err)
		}
		response, err := Respond(secret, c, b, now)
		if err != nil {
			t.Fatal(err)
		}
		changed := c
		mutate(&changed)
		if p.Verify(changed, response, now) == nil {
			t.Fatal("changed transcript accepted")
		}
		forged, err := Respond(secret, changed, b, now)
		if err == nil && p.Verify(c, forged, now) == nil {
			t.Fatal("changed transcript MAC accepted")
		}
		if p.Verify(c, response, now) != nil {
			t.Fatal("wrong attempt consumed valid challenge")
		}
	}
}
func TestWrongKeyExpiryAndLowOrderKeys(t *testing.T) {
	secret, b, now := fixture(t)
	p, c, err := New(b, now)
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := fixture(t)
	if _, err = Respond(other, c, b, now); err == nil {
		t.Fatal("wrong private key accepted")
	}
	response, err := Respond(secret, c, b, now)
	if err != nil {
		t.Fatal(err)
	}
	wrong := response
	wrong[0] ^= 1
	if p.Verify(c, wrong, now) == nil {
		t.Fatal("wrong MAC accepted")
	}
	for _, at := range []time.Time{now.Add(-time.Second), now.Add(Lifetime), now.Add(Lifetime + time.Second)} {
		if _, err = Respond(secret, c, b, at); err == nil {
			t.Fatal("invalid challenge lifetime accepted")
		}
		trial, same, e := New(b, now)
		if e != nil {
			t.Fatal(e)
		}
		sameResponse, e := Respond(secret, same, b, now)
		if e != nil {
			t.Fatal(e)
		}
		if trial.Verify(same, sameResponse, at) == nil {
			t.Fatal("invalid verification lifetime accepted")
		}
	}
	if p.Verify(c, response, now) != nil {
		t.Fatal("failed attempt consumed original")
	}
	for _, low := range [][32]byte{{}, {1}} {
		bad := b
		bad.DevicePublicKey = low
		if _, _, err = New(bad, now); err == nil {
			t.Fatal("low-order device key accepted")
		}
		badChallenge := c
		badChallenge.EphemeralPublicKey = low
		if _, err = Respond(secret, badChallenge, b, now); err == nil {
			t.Fatal("low-order ephemeral key accepted")
		}
	}
}
func TestPendingPrivateStateFormattingAndInvalidBindings(t *testing.T) {
	_, b, now := fixture(t)
	p, c, err := New(b, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{fmt.Sprint(p), fmt.Sprintf("%#v", p), func() string { raw, _ := json.Marshal(p); return string(raw) }()} {
		if text != "[private key-possession experiment]" && text != `"[private key-possession experiment]"` {
			t.Fatal("private state formatted")
		}
	}
	for _, origin := range []string{"http://remote.test", "https://registrar.test/", "https://user:pw@registrar.test", "https://registrar.test?", "https://registrar.test#f"} {
		bad := b
		bad.RegistrarOrigin = origin
		if _, _, err = New(bad, now); err == nil {
			t.Fatal("invalid origin accepted")
		}
	}
	if _, _, err = New(b, time.Time{}); err == nil {
		t.Fatal("zero time accepted")
	}
	if (*Pending)(nil).Verify(c, [32]byte{}, now) == nil {
		t.Fatal("nil pending accepted")
	}
	var zero Pending
	if zero.Verify(c, [32]byte{}, now) == nil {
		t.Fatal("zero pending accepted")
	}
}

func TestCancelExpiryAndIndependentChallengeCannotRevive(t *testing.T) {
	secret, b, now := fixture(t)
	for _, cancel := range []bool{false, true} {
		p, c, err := New(b, now)
		if err != nil {
			t.Fatal(err)
		}
		response, err := Respond(secret, c, b, now)
		if err != nil {
			t.Fatal(err)
		}
		if cancel {
			p.Cancel()
			p.Cancel()
		} else if p.Verify(c, response, now.Add(Lifetime)) == nil {
			t.Fatal("expired proof accepted")
		}
		if p.private != nil || p.Verify(c, response, now) == nil {
			t.Fatal("retired proof revived after clock rewind")
		}
	}
	p, c, err := New(b, now)
	if err != nil {
		t.Fatal(err)
	}
	q, d, err := New(b, now)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Cancel()
	response, err := Respond(secret, c, b, now)
	if err != nil {
		t.Fatal(err)
	}
	if q.Verify(d, response, now) == nil || q.Verify(c, response, now) == nil {
		t.Fatal("proof moved to another challenge")
	}
	if p.Verify(c, response, now) != nil {
		t.Fatal("wrong destination consumed proof")
	}
	(*Pending)(nil).Cancel()
}
