package control

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountLoginStateExpiryProviderAndCapacity(t *testing.T) {
	app, p, network, _ := accountFixture(t)
	callback := func(state, browser string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "https://control.example/account/oidc/callback?state="+state+"&error=denied", nil)
		req.AddCookie(&http.Cookie{Name: "mnet_account_login", Value: browser})
		rec := httptest.NewRecorder()
		app.handleAccountCallback(rec, req)
		return rec
	}
	for _, mode := range []string{"expired", "provider", "denied"} {

		if mode == "provider" {
			app.SetOIDC(&OIDCProvider{cfg: p.cfg})
		} else {
			app.SetOIDC(p)
		}
		app.accounts.pending = map[string]accountLogin{"fixture": {NetworkID: network, Provider: p, BrowserHash: sha256.Sum256([]byte("browser")), Expires: time.Now().Add(time.Minute)}}
		if mode == "expired" {
			v := app.accounts.pending["fixture"]
			v.Expires = time.Now()
			app.accounts.pending["fixture"] = v
		}
		if rec := callback("fixture", "other-browser"); rec.Code != 400 || len(app.accounts.pending) != 1 {
			t.Fatal("wrong browser consumed pending state")
		}
		if rec := callback("fixture", "browser"); rec.Code != 403 || len(app.accounts.pending) != 0 {
			t.Fatal("invalid callback accepted or state reusable")
		}
	}
	app.SetOIDC(p)
	app.accounts.pending = map[string]accountLogin{}
	for i := 0; i < maxPendingOIDCStates; i++ {
		app.accounts.pending[strings.Repeat("x", i+1)] = accountLogin{Expires: time.Now().Add(time.Hour)}
	}
	start := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "https://control.example/account/api/login", strings.NewReader(`{"network_id":"unknown"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		app.handleAccountLogin(rec, req)
		return rec
	}
	if rec := start(); rec.Code != 429 || rec.Header().Get("Set-Cookie") != "" {
		t.Fatal("pending capacity exceeded")
	}
	app.accounts.pending["x"] = accountLogin{Expires: time.Now()}
	rec := start()
	if rec.Code != 200 || len(app.accounts.pending) != maxPendingOIDCStates {
		t.Fatal("expired pending capacity not reclaimed")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/account/" {
		t.Fatal("account login cookie missing isolation flags")
	}
}
