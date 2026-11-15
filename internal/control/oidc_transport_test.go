package control

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOIDCTokenExchangeDoesNotForwardCredentialsOnRedirect(t *testing.T) {
	var received atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Store(true); w.WriteHeader(400) }))
	defer target.Close()
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer issuer.Close()
	p := &OIDCProvider{cfg: OIDCConfig{ClientID: "fixture-client", ClientSecret: "fixture-only"}, tokenURL: issuer.URL}
	if _, err := p.Exchange(context.Background(), "fixture-code", "https://control.test/oidc/callback", "nonce"); err == nil {
		t.Fatal("redirected exchange accepted")
	}
	if received.Load() {
		t.Fatal("followed token redirect with client credentials")
	}
}

func TestOIDCURLPolicy(t *testing.T) {
	for _, raw := range []string{"https://issuer.example/tenant", "http://localhost:8080/auth?tenant=test", "http://127.0.0.1:8080", "http://[::1]:8080/jwks"} {
		if err := validateOIDCURL(raw, false); err != nil {
			t.Fatalf("valid endpoint rejected %q: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "/relative", "http://issuer.example", "http://10.0.0.1", "ftp://issuer.example", "https://user:pw@issuer.example", "https://issuer.example/#fragment"} {
		if err := validateOIDCURL(raw, false); err == nil {
			t.Fatalf("unsafe endpoint accepted %q", raw)
		}
	}
	if err := validateOIDCURL("https://issuer.example?tenant=test", true); err == nil {
		t.Fatal("issuer with query accepted")
	}
}

func TestOIDCDiscoveryBindsIssuerAndChecksEndpoints(t *testing.T) {
	for _, mode := range []string{"valid", "mismatch", "missing", "insecure-endpoint"} {
		t.Run(mode, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				issuer := server.URL
				if mode == "mismatch" {
					issuer += "/other"
				}
				if mode == "missing" {
					issuer = ""
				}
				tokenURL := server.URL + "/token"
				if mode == "insecure-endpoint" {
					tokenURL = "http://untrusted.example/token"
				}
				json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "authorization_endpoint": server.URL + "/auth", "token_endpoint": tokenURL, "jwks_uri": server.URL + "/jwks"})
			}))
			defer server.Close()
			p := &OIDCProvider{cfg: OIDCConfig{Issuer: server.URL}}
			err := p.discover(context.Background())
			if (err == nil) != (mode == "valid") {
				t.Fatalf("discovery mode %s: %v", mode, err)
			}
		})
	}
}

func TestOIDCProviderErrorsDoNotEchoResponseBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, "private upstream diagnostics")
	}))
	defer server.Close()
	p := &OIDCProvider{tokenURL: server.URL}
	_, err := p.Exchange(context.Background(), "code", "https://control.test/oidc/callback", "nonce")
	if err == nil || strings.Contains(err.Error(), "private upstream diagnostics") {
		t.Fatalf("provider body disclosed: %v", err)
	}
}

func TestOIDCAuthURLPreservesEndpointQuery(t *testing.T) {
	p := &OIDCProvider{cfg: OIDCConfig{ClientID: "mesh-client"}, authURL: "https://issuer.example/auth?tenant=example"}
	u, err := url.Parse(p.AuthURL("login-nonce", "https://control.test/oidc/callback"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("tenant") != "example" || q.Get("nonce") != "login-nonce" || q.Get("state") != "login-nonce" || q.Get("client_id") != "mesh-client" {
		t.Fatalf("bad authorization URL: %s", u)
	}
}

func TestOIDCProviderCallsAreBounded(t *testing.T) {
	if oidcHTTPClient.Timeout <= 0 || oidcHTTPClient.Timeout > 15*time.Second {
		t.Fatalf("provider timeout %v", oidcHTTPClient.Timeout)
	}
	done := make(chan struct{})
	stop := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
			close(done)
		case <-stop:
		}
	}))
	defer server.Close()
	defer close(stop)
	p := &OIDCProvider{tokenURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.Exchange(ctx, "code", "https://control.test/oidc/callback", "nonce")
	if err == nil {
		t.Fatal("stalled provider succeeded")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("provider request not canceled")
	}
}
