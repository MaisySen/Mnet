package control_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

// fakeOIDC is a minimal provider: discovery, an authorization endpoint
// that immediately redirects back with a code, a token endpoint issuing
// a signed id_token, and JWKS. evil makes it sign with a key it does
// not publish, simulating a forged token.
type fakeOIDC struct {
	t            *testing.T
	srv          *httptest.Server
	key          *rsa.PrivateKey
	evil         bool
	email        string
	badNonce     bool
	denyIdentity bool
}

func newFakeOIDC(t *testing.T, evil bool) *fakeOIDC {
	t.Helper()
	f := &fakeOIDC{t: t, evil: evil, email: "dev@fintex.au"}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.key = k
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 f.srv.URL,
			"authorization_endpoint": f.srv.URL + "/auth",
			"token_endpoint":         f.srv.URL + "/token",
			"jwks_uri":               f.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != "mnet-test" || q.Get("response_type") != "code" {
			t.Errorf("bad auth request: %s", r.URL)
		}
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+url.QueryEscape(q.Get("nonce"))+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") == "" || r.Form.Get("client_secret") != "test-secret" {
			http.Error(w, "bad code", 400)
			return
		}
		head := b64([]byte(`{"alg":"RS256","kid":"testkey"}`))
		nonce := r.Form.Get("code")
		if f.badNonce {
			nonce = "wrong-nonce"
		}
		subject := "user-1"
		if f.denyIdentity {
			subject = "unlisted-user"
		}
		payload, _ := json.Marshal(map[string]any{
			"iss": f.srv.URL, "aud": "mnet-test",
			"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
			"sub": subject, "nonce": nonce, "email": f.email, "name": "Dev",
		})
		sum := sha256.Sum256([]byte(head + "." + b64(payload)))
		signer := f.key
		if f.evil {
			signer, _ = rsa.GenerateKey(rand.Reader, 2048)
		}
		sig, err := rsa.SignPKCS1v15(rand.Reader, signer, crypto.SHA256, sum[:])
		if err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"id_token": head + "." + b64(payload) + "." + b64(sig),
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "testkey",
			"n": b64(f.key.PublicKey.N.Bytes()),
			"e": b64(big.NewInt(int64(f.key.PublicKey.E)).Bytes()),
		}}})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func testEnvOIDC(t *testing.T, fake *fakeOIDC) (*httptest.Server, string) {
	t.Helper()
	ts, pw, _ := testEnvOIDCStore(t, fake)
	return ts, pw
}

func testEnvOIDCStore(t *testing.T, fake *fakeOIDC) (*httptest.Server, string, *control.Store) {
	t.Helper()
	store, err := control.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	t.Cleanup(ts.Close)
	srv, _, err := control.NewServer(store, relay.NewHub(), "http://"+ts.Listener.Addr().String(), "test-admin-pw")
	if err != nil {
		t.Fatal(err)
	}
	p, err := control.NewOIDCProvider(context.Background(), control.OIDCConfig{
		Issuer: fake.srv.URL, ClientID: "mnet-test", ClientSecret: "test-secret", AllowedSubjects: []string{"user-1"}})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetOIDC(p)
	ts.Config.Handler = srv.Handler()
	ts.Start()
	return ts, "test-admin-pw", store
}

// followLogin drives the browser: join approval URL through oidc/start,
// the provider, and back to the callback. Returns the final page.
func followLogin(t *testing.T, startURL string) (int, string) {
	t.Helper()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	u := startURL
	for hops := 0; hops < 6; hops++ {
		resp, err := client.Get(u)
		if err != nil {
			t.Fatalf("get %s: %v", u, err)
		}
		b, _ := io.ReadAll(resp.Body)
		loc, _ := resp.Location()
		resp.Body.Close()
		if loc == nil {
			return resp.StatusCode, string(b)
		}
		u = loc.String()
	}
	t.Fatal("redirect chain too long")
	return 0, ""
}

// The whole SSO path: join without a key returns an OIDC login URL, the
// browser round trip approves the node, the map then flows and carries
// the user identity.
func TestOIDCJoinFlow(t *testing.T) {
	fake := newFakeOIDC(t, false)
	ts, pw, store := testEnvOIDCStore(t, fake)
	base := ts.URL
	cookie := adminLogin(t, base, pw)
	post(t, base+"/admin/api/networks", map[string]string{"name": "sso"}, map[string]string{"Cookie": cookie.String()})

	code, body := post(t, base+"/api/join", netmap.JoinRequest{
		Network: "sso", Name: "devlaptop", PubKey: fakePubKey(0x42)}, nil)
	if code != 200 {
		t.Fatalf("join: %d %s", code, body)
	}
	var jr netmap.JoinResponse
	json.Unmarshal(body, &jr)
	if jr.Approved || !strings.Contains(jr.ApprovalURL, "/oidc/start") {
		t.Fatalf("expected oidc login url: %+v", jr)
	}

	code, page := followLogin(t, jr.ApprovalURL)
	if code != 200 || !strings.Contains(page, "dev@fintex.au") {
		t.Fatalf("login flow: %d %s", code, page)
	}

	if !strings.Contains(page, "Device approved") || !strings.Contains(page, "does not confirm a live VPN connection") || strings.Contains(page, "This device is now on the mesh") {
		t.Fatal("approval page overclaims live connection state")
	}

	headers := map[string]string{"X-Node-ID": jr.NodeID, "X-Node-Token": jr.NodeToken}
	code, m := getMap(t, base, jr, headers)
	if code != 200 {
		t.Fatalf("map after sso: %d", code)
	}
	if m.Self.User != "dev@fintex.au" {
		t.Fatalf("user not recorded: %+v", m.Self)
	}

	identity, ok := store.NodeApprovalIdentity(jr.NodeID)
	if !ok || identity.Issuer != fake.srv.URL || identity.Subject != "user-1" {
		t.Fatalf("callback did not bind verified issuer/subject: %+v", identity)
	}
	mapped, _ := json.Marshal(m)
	if strings.Contains(string(mapped), "approval_identities") || strings.Contains(string(mapped), "user-1") {
		t.Fatal("private OIDC principal disclosed in map")
	}

	// Without the node token the start endpoint refuses the login.
	resp, err := http.Get(base + "/oidc/start?node_id=" + jr.NodeID + "&token=wrong")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("start with bad token: %d", resp.StatusCode)
	}
}

// A provider whose token signature does not match its JWKS (forgery)
// must fail the login and leave the node unapproved.
func TestOIDCForgedToken(t *testing.T) {
	fake := newFakeOIDC(t, true)
	ts, pw := testEnvOIDC(t, fake)
	base := ts.URL
	cookie := adminLogin(t, base, pw)
	post(t, base+"/admin/api/networks", map[string]string{"name": "sso"}, map[string]string{"Cookie": cookie.String()})

	code, body := post(t, base+"/api/join", netmap.JoinRequest{
		Network: "sso", Name: "victim", PubKey: fakePubKey(0x43)}, nil)
	if code != 200 {
		t.Fatalf("join: %d %s", code, body)
	}
	var jr netmap.JoinResponse
	json.Unmarshal(body, &jr)

	code, page := followLogin(t, jr.ApprovalURL)
	if code != 403 {
		t.Fatalf("forged token must fail with 403, got %d: %s", code, page)
	}

	headers := map[string]string{"X-Node-ID": jr.NodeID, "X-Node-Token": jr.NodeToken}
	if code, _ := getMap(t, base, jr, headers); code != 403 {
		t.Fatalf("node must stay unapproved: %d", code)
	}
}

// A signed identity claim is still data, never HTML markup.
func TestOIDCIdentityIsHTMLEscaped(t *testing.T) {
	fake := newFakeOIDC(t, false)
	fake.email = "<script>untrusted()</script>"
	ts, pw := testEnvOIDC(t, fake)
	cookie := adminLogin(t, ts.URL, pw)
	post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "sso"}, map[string]string{"Cookie": cookie.String()})
	code, body := post(t, ts.URL+"/api/join", netmap.JoinRequest{Network: "sso", Name: "markup", PubKey: fakePubKey(0x44)}, nil)
	if code != 200 {
		t.Fatalf("join: %d", code)
	}
	var jr netmap.JoinResponse
	if err := json.Unmarshal(body, &jr); err != nil {
		t.Fatal(err)
	}
	code, page := followLogin(t, jr.ApprovalURL)
	if code != 200 || strings.Contains(page, "<script>") || !strings.Contains(page, "&lt;script&gt;") {
		t.Fatalf("identity not escaped: %d %s", code, page)
	}
}

func TestOIDCRejectsNonceMismatchAndUnlistedIdentity(t *testing.T) {
	for _, mode := range []string{"nonce", "identity"} {
		t.Run(mode, func(t *testing.T) {
			fake := newFakeOIDC(t, false)
			fake.badNonce = mode == "nonce"
			fake.denyIdentity = mode == "identity"
			ts, pw := testEnvOIDC(t, fake)
			cookie := adminLogin(t, ts.URL, pw)
			post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "sso"}, map[string]string{"Cookie": cookie.String()})
			code, body := post(t, ts.URL+"/api/join", netmap.JoinRequest{Network: "sso", Name: "rejected", PubKey: fakePubKey(0x45)}, nil)
			if code != 200 {
				t.Fatalf("join %d", code)
			}
			var jr netmap.JoinResponse
			if err := json.Unmarshal(body, &jr); err != nil {
				t.Fatal(err)
			}
			code, _ = followLogin(t, jr.ApprovalURL)
			if code != 403 {
				t.Fatalf("invalid %s accepted: %d", mode, code)
			}
			if code, _ := getMap(t, ts.URL, jr, map[string]string{"X-Node-ID": jr.NodeID, "X-Node-Token": jr.NodeToken}); code != 403 {
				t.Fatalf("rejected identity got map: %d", code)
			}
		})
	}
}
