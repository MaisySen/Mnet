package enrollmentproof

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func fixtureChallenge(t *testing.T) contract.Challenge {
	t.Helper()
	raw, err := os.ReadFile("../enrollmentcontract/testdata/transcript-v4.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Challenge contract.Challenge `json:"challenge"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v.Challenge
}
func proofPair(t *testing.T) (*Server, contract.Challenge, *Native, time.Time) {
	t.Helper()
	c := fixtureChallenge(t)
	now := time.UnixMilli(c.Binding.CheckedAtMS)
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c.Binding.DevicePublicKey = encode(private.PublicKey().Bytes())
	server, challenge, err := NewServer(c.Binding, now, now.Add(time.Second*10), time.Second*15)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	var raw [32]byte
	copy(raw[:], private.Bytes())
	native, err := Respond(raw, challenge, c.Binding, now, 15*time.Second)
	clear(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(native.Close)
	return server, challenge, native, now
}
func TestActualDHResponseAndSeparateConfirmationNoConsumption(t *testing.T) {
	server, c, native, now := proofPair(t)
	response, err := native.Response(now)
	if err != nil {
		t.Fatal(err)
	}
	confirm, err := server.Verify(c.ID, response, now)
	if err != nil {
		t.Fatal(err)
	}
	if confirm == response || confirm == [32]byte{} {
		t.Fatal("key purpose collided")
	}
	if err := native.Confirm(confirm, now); err != nil {
		t.Fatal(err)
	}
	// Verification is intentionally NOT consumed outside Store transaction.
	again, err := server.Verify(c.ID, response, now.Add(time.Millisecond))
	if err != nil || again != confirm {
		t.Fatal("nonconsuming retry changed exact confirmation", err)
	}
	wrong := response
	wrong[0] ^= 1
	if _, err := server.Verify(c.ID, wrong, now); err == nil {
		t.Fatal("wrong response accepted")
	}
	if _, err := server.Verify(c.ID+"x", response, now); err == nil {
		t.Fatal("foreign ID resolved")
	}
	if err := native.Confirm(response, now); err == nil {
		t.Fatal("response reused as confirmation")
	}
	wrong = confirm
	wrong[1] ^= 1
	if err := native.Confirm(wrong, now); err == nil {
		t.Fatal("wrong confirmation accepted")
	}
	if err := native.Confirm(confirm, now); err != nil {
		t.Fatal("bad proof consumed legitimate object")
	}
	text := fmt.Sprintf("%v %#v %v %#v", server, *server, native, *native)
	if strings.Contains(text, c.ID) || strings.Contains(text, encode(native.shared[:])) {
		t.Fatal("private formatter leaked")
	}
}
func TestEveryIndependentBindingFieldMustMatchBeforeNativeKeyUse(t *testing.T) {
	_, c, _, now := proofPair(t)
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var raw [32]byte
	copy(raw[:], private.Bytes())
	c.Binding.DevicePublicKey = encode(private.PublicKey().Bytes())
	baseline, err := Respond(raw, c, c.Binding, now, time.Minute)
	if err != nil {
		t.Fatal("valid baseline for field mutations refused", err)
	}
	baseline.Close()
	fields := reflect.ValueOf(c.Binding)
	for i := 0; i < fields.NumField(); i++ {
		changed := c.Binding
		field := reflect.ValueOf(&changed).Elem().Field(i)
		switch field.Kind() {
		case reflect.String:
			field.SetString(field.String() + "x")
		case reflect.Int, reflect.Int64:
			field.SetInt(field.Int() + 1)
		default:
			t.Fatal("uncovered field")
		}
		if _, err := Respond(raw, c, changed, now, time.Minute); err == nil {
			t.Fatalf("independent field %s ignored", fields.Type().Field(i).Name)
		}
	}
}
func TestNativeKeyMismatchLowOrderAndMalformedChallengeRefuse(t *testing.T) {
	c := fixtureChallenge(t)
	now := time.UnixMilli(c.Binding.CheckedAtMS)
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var raw [32]byte
	copy(raw[:], private.Bytes())
	c.Binding.DevicePublicKey = encode(private.PublicKey().Bytes())
	for _, kind := range []string{"id", "nonce", "ephemeral", "loworder", "expiry", "namespace", "zero-clock", "negative-lifetime", "too-long"} {
		t.Run(kind, func(t *testing.T) {
			changed := c
			at := now
			life := time.Minute
			switch kind {
			case "id":
				changed.ID = "bad"
			case "nonce":
				changed.Nonce = "bad"
			case "ephemeral":
				changed.EphemeralPublicKey = "bad"
			case "loworder":
				key := [32]byte{1}
				changed.EphemeralPublicKey = encode(key[:])
			case "expiry":
				changed.ExpiresAtMS = now.UnixMilli()
			case "namespace":
				changed.Binding.PrincipalNamespace = "oidc-subject"
			case "zero-clock":
				at = time.Time{}
			case "negative-lifetime":
				life = -1
			case "too-long":
				life = time.Nanosecond
			}
			if n, err := Respond(raw, changed, changed.Binding, at, life); err == nil {
				n.Close()
				t.Fatal("invalid challenge accepted")
			}
		})
	}
	other, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c.Binding.DevicePublicKey = encode(other.PublicKey().Bytes())
	if n, err := Respond(raw, c, c.Binding, now, time.Minute); err == nil {
		n.Close()
		t.Fatal("wrong durable private key accepted")
	}
	low := [32]byte{1}
	c.Binding.DevicePublicKey = encode(low[:])
	if s, _, err := NewServer(c.Binding, now, now.Add(time.Second), time.Minute); err == nil {
		s.Close()
		t.Fatal("loworder device key admitted")
	}
}
func TestOriginalExpiryCloseAndCopyLifecycle(t *testing.T) {
	s, c, n, now := proofPair(t)
	response, _ := n.Response(now)
	confirmation, _ := s.Verify(c.ID, response, now)
	end := time.UnixMilli(c.ExpiresAtMS)
	for _, at := range []time.Time{time.Time{}, now.Add(-time.Millisecond), end, end.Add(time.Millisecond)} {
		if _, err := s.Verify(c.ID, response, at); err == nil {
			t.Fatal("server expiry/backward clock ignored")
		}
		if _, err := n.Response(at); err == nil {
			t.Fatal("native expiry/backward ignored")
		}
		if err := n.Confirm(confirmation, at); err == nil {
			t.Fatal("expired confirmation accepted")
		}
	}
	scopy := *s
	ncopy := *n
	scopy.Close()
	ncopy.Close()
	if _, err := s.Verify(c.ID, response, now); err == nil {
		t.Fatal("copied server bypassed Close")
	}
	if _, err := n.Response(now); err == nil {
		t.Fatal("copied native bypassed Close")
	}
	if n.shared != [32]byte{} || n.response != [32]byte{} {
		t.Fatal("Close retained temporary native buffers")
	}
	var emptyS Server
	var emptyN Native
	emptyS.Close()
	emptyN.Close()
	if _, err := emptyS.Verify(c.ID, response, now); err == nil {
		t.Fatal("zero server accepted")
	}
	if _, err := emptyN.Response(now); err == nil {
		t.Fatal("zero native accepted")
	}
}
func TestConcurrentWrongProofRetriesAndClose(t *testing.T) {
	s, c, n, now := proofPair(t)
	response, _ := n.Response(now)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				s.Verify(c.ID, response, now)
			case 1:
				n.Response(now)
			case 2:
				s.Close()
			case 3:
				n.Close()
			}
		}(i)
	}
	wg.Wait()
}
func TestFreshServerEntropyAndIndependentChallenges(t *testing.T) {
	s, c, _, now := proofPair(t)
	_ = s
	other, second, err := NewServer(c.Binding, now, time.UnixMilli(c.ExpiresAtMS), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if c.ID == second.ID || c.Nonce == second.Nonce || c.EphemeralPublicKey == second.EphemeralPublicKey {
		t.Fatal("fresh owner reused challenge material")
	}
}
func TestProductionScheduleMatchesIndependentPythonFixtures(t *testing.T) {
	c := fixtureChallenge(t)
	load := func(name string) map[string]string {
		raw, err := os.ReadFile("../enrollmentcontract/testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]string
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	decode := func(s string) []byte {
		raw, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	responseVector := load("response-mac-v4.json")
	confirmationVector := load("server-confirmation-v4.json")
	shared := decode(responseVector["shared_hex"])
	response, err := responseMAC(shared, c)
	if err != nil || !bytes.Equal(response[:], decode(responseVector["response_mac_hex"])) {
		t.Fatal("actual response schedule differs from independent Python", err)
	}
	confirmation, err := confirmationMAC(shared, c, response)
	if err != nil || !bytes.Equal(confirmation[:], decode(confirmationVector["mac_hex"])) {
		t.Fatal("actual confirmation schedule differs from independent Python", err)
	}
	changed := c
	changed.Binding.PrincipalID += "x"
	different, err := responseMAC(shared, changed)
	if err != nil || different == response {
		t.Fatal("transcript not MAC bound")
	}
	different, err = confirmationMAC(shared, changed, response)
	if err != nil || different == confirmation {
		t.Fatal("confirmation transcript not bound")
	}
	response[0] ^= 1
	different, err = confirmationMAC(shared, c, response)
	if err != nil || different == confirmation {
		t.Fatal("confirmation response not bound")
	}
}

func TestWrongChallengeCannotReplaceOwnedServerTranscriptOrConfirmAnotherNative(t *testing.T) {
	server, c, native, now := proofPair(t)
	response, _ := native.Response(now)
	confirmation, err := server.Verify(c.ID, response, now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, other, _ := proofPair(t)
	if err := other.Confirm(confirmation, now); err == nil {
		t.Fatal("foreign pending native accepted confirmation")
	}
	modified := c
	modified.Nonce = encode(bytes.Repeat([]byte{13}, 32))
	if server.challenge != c {
		t.Fatal("caller value aliased owned challenge")
	}
	wrong, err := responseMAC(native.shared[:], modified)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Verify(c.ID, wrong, now); err == nil {
		t.Fatal("caller replacement transcript accepted")
	}
	if _, err := server.Verify(c.ID, response, now); err != nil {
		t.Fatal("wrong replacement consumed live original")
	}
}
func TestAdmissionClockCannotMoveBeforeOriginalCapture(t *testing.T) {
	c := fixtureChallenge(t)
	base := time.UnixMilli(c.Binding.CheckedAtMS)
	admitted := base.Add(time.Millisecond * 500)
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c.Binding.DevicePublicKey = encode(private.PublicKey().Bytes())
	s, challenge, err := NewServer(c.Binding, admitted, base.Add(time.Second*10), time.Second*15)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var key [32]byte
	copy(key[:], private.Bytes())
	n, err := Respond(key, challenge, c.Binding, admitted, time.Second*15)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	response, _ := n.Response(admitted)
	if _, err := s.Verify(challenge.ID, response, base); err == nil {
		t.Fatal("server admission clock moved backward")
	}
	if _, err := n.Response(base); err == nil {
		t.Fatal("native admission clock moved backward")
	}
}
