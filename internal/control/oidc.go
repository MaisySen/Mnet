package control

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OIDCConfig enables SSO login for node joins. All three fields are
// required; a nil provider keeps the approval-only flow.
type OIDCConfig struct {
	Issuer       string // for example https://accounts.google.com
	ClientID     string
	ClientSecret string
}

// OIDCProvider is a discovered OpenID Connect issuer: authorization code
// flow with RS256 ID tokens checked against the issuer's JWKS.
type OIDCProvider struct {
	cfg OIDCConfig

	mu       sync.Mutex
	authURL  string
	tokenURL string
	jwksURL  string
	keys     map[string]*rsa.PublicKey
	keysAt   time.Time
}

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSUri               string `json:"jwks_uri"`
}

// NewOIDCProvider discovers the issuer's endpoints.
func NewOIDCProvider(ctx context.Context, cfg OIDCConfig) (*OIDCProvider, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("oidc needs issuer, client id and client secret")
	}
	p := &OIDCProvider{cfg: cfg, keys: map[string]*rsa.PublicKey{}}
	if err := p.discover(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *OIDCProvider) discover(ctx context.Context) error {
	u := strings.TrimRight(p.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("oidc discovery: %s", resp.Status)
	}
	var d oidcDiscovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return fmt.Errorf("oidc discovery: %w", err)
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSUri == "" {
		return fmt.Errorf("oidc discovery: incomplete metadata from %s", u)
	}
	p.mu.Lock()
	p.authURL, p.tokenURL, p.jwksURL = d.AuthorizationEndpoint, d.TokenEndpoint, d.JWKSUri
	p.mu.Unlock()
	return nil
}

// AuthURL is where the user's browser goes to log in. state binds the
// round trip; redirectURL is our own callback.
func (p *OIDCProvider) AuthURL(state, redirectURL string) string {
	p.mu.Lock()
	base := p.authURL
	p.mu.Unlock()
	q := url.Values{
		"client_id":     {p.cfg.ClientID},
		"redirect_uri":  {redirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
	}
	return base + "?" + q.Encode()
}

// Claims is the verified identity from an ID token.
type OIDCClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}

// Exchange trades an authorization code for the verified user identity.
func (p *OIDCProvider) Exchange(ctx context.Context, code, redirectURL string) (*OIDCClaims, error) {
	p.mu.Lock()
	tokenURL := p.tokenURL
	p.mu.Unlock()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURL},
		"client_id":     {p.cfg.ClientID},
		"client_secret": {p.cfg.ClientSecret},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange: %s: %s", resp.Status, body)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.IDToken == "" {
		return nil, fmt.Errorf("token exchange: no id_token in response")
	}
	return p.verifyIDToken(ctx, tok.IDToken)
}

// verifyIDToken checks signature (RS256 against the issuer JWKS), issuer,
// audience and expiry, then returns the claims.
func (p *OIDCProvider) verifyIDToken(ctx context.Context, idToken string) (*OIDCClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("id token: not a JWT")
	}
	decode := func(s string) ([]byte, error) {
		return base64.RawURLEncoding.DecodeString(s)
	}
	head, err := decode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("id token header: %w", err)
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(head, &hdr); err != nil || hdr.Alg != "RS256" {
		return nil, fmt.Errorf("id token: unsupported alg %q", hdr.Alg)
	}
	key, err := p.key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := decode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("id token signature: %w", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return nil, fmt.Errorf("id token: bad signature")
	}
	payload, err := decode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("id token payload: %w", err)
	}
	var claims struct {
		OIDCClaims
		Iss string `json:"iss"`
		Aud any    `json:"aud"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("id token payload: %w", err)
	}
	if !strings.EqualFold(strings.TrimRight(claims.Iss, "/"), strings.TrimRight(p.cfg.Issuer, "/")) {
		return nil, fmt.Errorf("id token: issuer %q does not match", claims.Iss)
	}
	audOK := false
	switch a := claims.Aud.(type) {
	case string:
		audOK = a == p.cfg.ClientID
	case []any:
		for _, v := range a {
			if v == p.cfg.ClientID {
				audOK = true
			}
		}
	}
	if !audOK {
		return nil, fmt.Errorf("id token: not issued for this client")
	}
	if claims.Exp != 0 && time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("id token: expired")
	}
	if claims.OIDCClaims.Email == "" && claims.OIDCClaims.Subject == "" {
		return nil, fmt.Errorf("id token: no identity claims")
	}
	return &claims.OIDCClaims, nil
}

// key returns the JWKS key by id, fetching and caching the set.
func (p *OIDCProvider) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.mu.Lock()
	if k := p.keys[kid]; k != nil && time.Since(p.keysAt) < time.Hour {
		p.mu.Unlock()
		return k, nil
	}
	jwksURL := p.jwksURL
	p.mu.Unlock()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	defer resp.Body.Close()
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nb, err1 := base64.RawURLEncoding.DecodeString(k.N)
		eb, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		p.keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}
	}
	p.keysAt = time.Now()
	if k := p.keys[kid]; k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("jwks: no key with kid %q", kid)
}

// newOIDCState is a random login round-trip token.
func newOIDCState() string {
	b := make([]byte, 24)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
