package control

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminSessionBoundRotationAndLogout(t *testing.T) {
	st, err := openTestStore(t, t.TempDir()+"/state.json")
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := NewServer(st, nil, "https://control.invalid", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	// Invoke the already independently tested login handler directly so this
	// bounded-state fixture does not depend on the unrelated per-IP limiter.
	login := func(token, password string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "https://control.invalid/admin/api/login", strings.NewReader(`{"password":"`+password+`"}`))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: token})
		}
		rec := httptest.NewRecorder()
		app.handleAdminLogin(rec, req)
		return rec
	}
	for i := 0; i < maxAdminSessions; i++ {
		app.sessions[fmt.Sprint(i)] = time.Now().Add(time.Hour)
	}
	if rec := login("", "fixture"); rec.Code != 429 || rec.Header().Get("Set-Cookie") != "" || len(app.sessions) != maxAdminSessions {
		t.Fatal("full session table admitted another session")
	}
	// A failed password cannot invalidate an existing token.
	if rec := login("1", "wrong"); rec.Code != 401 {
		t.Fatal("wrong password accepted")
	}
	if _, ok := app.sessions["1"]; !ok {
		t.Fatal("failed login revoked session")
	}
	// A valid existing session rotates at capacity without evicting others.
	rec := login("1", "fixture")
	cookies := rec.Result().Cookies()
	if rec.Code != 200 || len(cookies) != 1 || cookies[0].Value == "1" {
		t.Fatalf("rotation failed %d", rec.Code)
	}
	rotated := cookies[0]
	if _, ok := app.sessions["1"]; ok || len(app.sessions) != maxAdminSessions {
		t.Fatal("rotation retained old token or changed size")
	}
	app.sessions["0"] = time.Now()
	if rec := login("", "fixture"); rec.Code != 200 || len(app.sessions) != maxAdminSessions {
		t.Fatal("expired session not reclaimed")
	}
	if _, ok := app.sessions["0"]; ok {
		t.Fatal("expired session survived reclaim")
	}
	request := func(method, path, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://control.invalid"+path, strings.NewReader(`{}`))
		req.AddCookie(cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := request("POST", "/admin/api/logout", "https://other.invalid", rotated); rec.Code != 403 {
		t.Fatal("cross-site logout accepted")
	}
	if _, ok := app.sessions[rotated.Value]; !ok {
		t.Fatal("cross-site logout revoked session")
	}
	rec = request("POST", "/admin/api/logout", "https://control.invalid", rotated)
	cleared := rec.Result().Cookies()
	if rec.Code != 200 || len(cleared) != 1 || cleared[0].MaxAge != -1 || cleared[0].Value != "" || !cleared[0].Secure || !cleared[0].HttpOnly {
		t.Fatal("logout did not clear protected cookie")
	}
	if rec := request("GET", "/admin/api/state", "", rotated); rec.Code != 401 {
		t.Fatal("logged-out token replay accepted")
	}
	if rec := request("GET", "/admin/api/state", "", &http.Cookie{Name: "mnet_admin", Value: "1"}); rec.Code != 401 {
		t.Fatal("rotated token replay accepted")
	}
	app.sessions["expired"] = time.Now()
	if rec := request("GET", "/admin/api/state", "", &http.Cookie{Name: "mnet_admin", Value: "expired"}); rec.Code != 401 {
		t.Fatal("expired session accepted")
	}
	if _, ok := app.sessions["expired"]; ok {
		t.Fatal("expired authorization lookup did not clean token")
	}
}
