package control

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSignedOIDCClaimValidation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &OIDCProvider{cfg: OIDCConfig{Issuer: "https://issuer.example", ClientID: "mesh-client"}, keys: map[string]*rsa.PublicKey{"key": &key.PublicKey}, keysAt: time.Now()}
	for _, tc := range []struct {
		name  string
		patch func(map[string]any)
		valid bool
	}{
		{"valid", func(m map[string]any) {}, true},
		{"single array audience", func(m map[string]any) { m["aud"] = []string{"mesh-client"} }, true},
		{"matching authorized party", func(m map[string]any) { m["azp"] = "mesh-client" }, true},
		{"missing expiry", func(m map[string]any) { delete(m, "exp") }, false},
		{"zero expiry", func(m map[string]any) { m["exp"] = 0 }, false},
		{"expired", func(m map[string]any) { m["exp"] = time.Now().Unix() - 1 }, false},
		{"expiry at current second", func(m map[string]any) { m["exp"] = time.Now().Unix() }, false},
		{"missing issued time", func(m map[string]any) { delete(m, "iat") }, false},
		{"future issued time", func(m map[string]any) { m["iat"] = time.Now().Add(5 * time.Minute).Unix() }, false},
		{"expiry before issuance", func(m map[string]any) {
			m["iat"] = time.Now().Add(30 * time.Second).Unix()
			m["exp"] = time.Now().Add(10 * time.Second).Unix()
		}, false},
		{"issuer case changed", func(m map[string]any) { m["iss"] = "https://ISSUER.example" }, false},
		{"issuer slash changed", func(m map[string]any) { m["iss"] = "https://issuer.example/" }, false},
		{"email without subject", func(m map[string]any) { delete(m, "sub"); m["email"] = "test@example.test" }, false},
		{"oversized subject", func(m map[string]any) { m["sub"] = strings.Repeat("x", 256) }, false},
		{"non ASCII subject", func(m map[string]any) { m["sub"] = "user\u00e9" }, false},
		{"wrong audience", func(m map[string]any) { m["aud"] = "another-client" }, false},
		{"untrusted extra audience", func(m map[string]any) { m["aud"] = []string{"mesh-client", "another-client"}; m["azp"] = "mesh-client" }, false},
		{"nonstring audience", func(m map[string]any) { m["aud"] = []any{"mesh-client", 42} }, false},
		{"wrong authorized party", func(m map[string]any) { m["azp"] = "another-client" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := map[string]any{"iss": p.cfg.Issuer, "aud": p.cfg.ClientID, "sub": "user-1", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix()}
			tc.patch(claims)
			payload, err := json.Marshal(claims)
			if err != nil {
				t.Fatal(err)
			}
			b64 := base64.RawURLEncoding.EncodeToString
			unsigned := b64([]byte(`{"alg":"RS256","kid":"key"}`)) + "." + b64(payload)
			sum := sha256.Sum256([]byte(unsigned))
			sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.verifyIDToken(context.Background(), unsigned+"."+b64(sig))
			if (err == nil) != tc.valid {
				t.Fatalf("accepted=%v want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}
