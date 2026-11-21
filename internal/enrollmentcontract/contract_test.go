package enrollmentcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func vector(t *testing.T) (Challenge, []byte) {
	t.Helper()
	raw, err := os.ReadFile("testdata/transcript-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		FieldCount int       `json:"field_count"`
		ByteLength int       `json:"byte_length"`
		SHA256     string    `json:"sha256_hex"`
		Challenge  Challenge `json:"challenge"`
	}
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("testdata/transcript-v3.hex")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(v.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(encoded)
	if v.FieldCount != 26 || len(encoded) != v.ByteLength || !bytes.Equal(encoded, expected) || hex.EncodeToString(hash[:]) != v.SHA256 {
		t.Fatal("Go differs from independent Python transcript")
	}
	return v.Challenge, expected
}
func TestTranscriptIndependentPythonBytes(t *testing.T) {
	c, expected := vector(t)
	if !bytes.Contains(expected, []byte("<>&-é-\u2028-\u2029")) {
		t.Fatal("exact Unicode/HTML bytes lost")
	}
	// Every primitive field must either change the exact bytes or be refused.
	// Reflection is TEST-ONLY; production uses an explicit ordered encoding.
	for _, target := range []string{"challenge", "binding"} {
		original := reflect.ValueOf(c)
		if target == "binding" {
			original = reflect.ValueOf(c.Binding)
		}
		for i := 0; i < original.NumField(); i++ {
			if original.Field(i).Kind() == reflect.Struct {
				continue
			}
			changed := c
			value := reflect.ValueOf(&changed).Elem()
			if target == "binding" {
				value = value.FieldByName("Binding")
			}
			field := value.Field(i)
			switch field.Kind() {
			case reflect.String:
				field.SetString(field.String() + "x")
			case reflect.Int, reflect.Int64:
				field.SetInt(field.Int() + 1)
			default:
				t.Fatal("test omitted field kind")
			}
			actual, err := Encode(changed)
			if err == nil && bytes.Equal(actual, expected) {
				t.Fatalf("unbound %s.%s", target, original.Type().Field(i).Name)
			}
		}
	}
}
func TestTranscriptPrincipalNamespaceNeverOIDCInference(t *testing.T) {
	c, _ := vector(t)
	for _, namespace := range []string{"", "oidc-subject", "OIDC", "multies-local-user-v1 "} {
		changed := c
		changed.Binding.PrincipalNamespace = namespace
		if _, err := Encode(changed); err == nil {
			t.Fatal("unconfigured/inferred principal namespace accepted")
		}
	}
	// Distinct authority and issuer label are intentional data, not equivalence.
	if c.Binding.PrincipalAuthority == c.Binding.IssuerLabel {
		t.Fatal("fixture conflates namespaces")
	}
}
func TestTranscriptInvalidIdentityAndDeadlineSyntax(t *testing.T) {
	c, _ := vector(t)
	for _, mutate := range []func(*Challenge){
		func(c *Challenge) { c.Binding.PrincipalAuthority = "http://multies.test" },
		func(c *Challenge) { c.Binding.RegistrarOrigin = "https://registrar.test/path" },
		func(c *Challenge) { c.Binding.PrincipalID = "person\n" },
		func(c *Challenge) { c.Binding.DescriptorEncodingVersion = 2 },
		func(c *Challenge) {
			c.Binding.CredentialDigest = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
		},
		func(c *Challenge) { c.Binding.CheckedAtMS = 0 },
		func(c *Challenge) { c.Binding.ReserveDeadlineMS = c.Binding.CheckedAtMS },
		func(c *Challenge) { c.Binding.AccessUntilMS = c.Binding.ReserveDeadlineMS + 1 },
		func(c *Challenge) { c.ExpiresAtMS = c.Binding.AccessUntilMS + 1 },
	} {
		changed := c
		mutate(&changed)
		if _, err := Encode(changed); err == nil {
			t.Fatal("invalid untrusted contract data accepted")
		}
	}
}
func TestFixtureTimeBoundariesPauseSkewAndOverflow(t *testing.T) {
	c, _ := vector(t)
	p := FixtureTiming{MaxObservationAgeMS: 20000, MaxLeaseMS: 60000, ClockUncertaintyMS: 100}
	if err := CheckFixtureTime(c, c.Binding.CheckedAtMS, p); err != nil {
		t.Fatal(err)
	}
	for _, now := range []int64{c.Binding.CheckedAtMS - 1, c.Binding.CheckedAtMS + 20001, c.ExpiresAtMS - 100, c.ExpiresAtMS, c.Binding.ReserveDeadlineMS, math.MaxInt64} {
		if err := CheckFixtureTime(c, now, p); err == nil {
			t.Fatal("clock jump/stale/deadline boundary accepted", now)
		}
	}
	if err := CheckFixtureTime(c, c.ExpiresAtMS-101, FixtureTiming{MaxObservationAgeMS: 60000, MaxLeaseMS: 60000, ClockUncertaintyMS: 100}); err != nil {
		t.Fatal("strict deadline margin rejected", err)
	}
	for _, p := range []FixtureTiming{{}, {-1, 60000, 0}, {20000, 0, 0}, {20000, 60000, -1}, {20000, 60000, 60000}, {20000, 100, 0}} {
		if err := CheckFixtureTime(c, c.Binding.CheckedAtMS, p); err == nil {
			t.Fatal("missing/unbounded fixture policy accepted")
		}
	}
	// A process pause between observations is rechecked, never implicitly renewed.
	if err := CheckFixtureTime(c, c.Binding.CheckedAtMS+1, p); err != nil {
		t.Fatal(err)
	}
	if err := CheckFixtureTime(c, c.Binding.AccessUntilMS+1, p); err == nil {
		t.Fatal("pause revived fixture lease")
	}
}
