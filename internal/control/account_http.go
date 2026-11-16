package control

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type accountLogin struct {
	NetworkID   string
	Provider    *OIDCProvider
	BrowserHash [32]byte
	Expires     time.Time
}

func (s *Server) accountCookie(w http.ResponseWriter, name, token string, expiry time.Time, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/account/", HttpOnly: true, Secure: strings.HasPrefix(s.publicBase, "https://"), SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: maxAge})
}
func accountPrivate(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func accountMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	accountPrivate(w)
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeErr(w, 405, "method not allowed")
		return false
	}
	return true
}

// Login is separate from device approval. Starting it does not grant
// membership, approve a node or accept a caller-supplied principal.
func (s *Server) handleAccountLogin(w http.ResponseWriter, r *http.Request) {
	if !accountMethod(w, r, "POST") || !s.requireAdminJSON(w, r) {
		return
	}
	p := s.oidcProvider()
	if p == nil {
		writeErr(w, 404, "account login is not configured")
		return
	}
	var input struct {
		NetworkID string `json:"network_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&input) != nil || input.NetworkID == "" || dec.Decode(&struct{}{}) != io.EOF {
		writeErr(w, 400, "one network_id required")
		return
	}
	// Unknown networks and valid networks have the same start behavior. The
	// verified callback and each subsequent query enforce explicit membership.
	now := time.Now()
	state, browser := newOIDCState(), newOIDCState()
	s.accounts.mu.Lock()
	if s.accounts.pending == nil {
		s.accounts.pending = make(map[string]accountLogin)
	}
	for key, login := range s.accounts.pending {
		if !now.Before(login.Expires) {
			delete(s.accounts.pending, key)
		}
	}
	if len(s.accounts.pending) >= maxPendingOIDCStates {
		s.accounts.mu.Unlock()
		writeErr(w, 429, "too many pending account logins")
		return
	}
	s.accounts.pending[state] = accountLogin{NetworkID: input.NetworkID, Provider: p, BrowserHash: sha256.Sum256([]byte(browser)), Expires: now.Add(10 * time.Minute)}
	s.accounts.mu.Unlock()
	s.accountCookie(w, "mnet_account_login", browser, now.Add(10*time.Minute), 600)
	writeJSON(w, 200, map[string]string{"authorization_url": p.AuthURL(state, s.publicURL("/account/oidc/callback"))})
}

func (s *Server) handleAccountCallback(w http.ResponseWriter, r *http.Request) {
	if !accountMethod(w, r, "GET") {
		return
	}
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("mnet_account_login")
	if err != nil {
		writeErr(w, 400, "account login unknown or expired")
		return
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	s.accounts.mu.Lock()
	login, ok := s.accounts.pending[state]
	// A callback without the originating browser proof cannot consume the
	// real browser's flow. Every matching attempt is one-use, even on denial.
	matches := ok && subtle.ConstantTimeCompare(hash[:], login.BrowserHash[:]) == 1
	if matches {
		delete(s.accounts.pending, state)
	}
	s.accounts.mu.Unlock()
	if !matches {
		writeErr(w, 400, "account login unknown or expired")
		return
	}
	s.accountCookie(w, "mnet_account_login", "", time.Unix(1, 0), -1)
	if !time.Now().Before(login.Expires) || login.Provider != s.oidcProvider() || login.Provider == nil || r.URL.Query().Get("error") != "" {
		writeErr(w, 403, "account login refused")
		return
	}
	claims, err := login.Provider.Exchange(r.Context(), r.URL.Query().Get("code"), s.publicURL("/account/oidc/callback"), state)
	if err != nil {
		writeErr(w, 403, "account login refused")
		return
	}
	token, err := s.newAccountSession(login.NetworkID, login.Provider, claims)
	if err != nil {
		writeErr(w, 403, "account login refused")
		return
	}
	// Replace only this browser's previous account session, never an admin
	// session or a device token. New tokens never appear in URLs or JSON.
	if previous, err := r.Cookie("mnet_account"); err == nil {
		s.revokeAccountSession(previous.Value)
	}
	s.accountCookie(w, "mnet_account", token, time.Now().Add(accountSessionLifetime), int(accountSessionLifetime.Seconds()))
	http.Redirect(w, r, "/account/api/devices", http.StatusSeeOther)
}

func (s *Server) handleAccountDevices(w http.ResponseWriter, r *http.Request) {
	if !accountMethod(w, r, "GET") {
		return
	}
	cookie, err := r.Cookie("mnet_account")
	if err != nil {
		writeErr(w, 401, "account login required")
		return
	}
	devices, err := s.accountDevices(cookie.Value)
	if err != nil {
		writeErr(w, 401, "account login or membership required")
		return
	}
	if devices == nil {
		devices = []IdentityDevice{}
	}
	writeJSON(w, 200, map[string]any{"devices": devices})
}
func (s *Server) handleAccountLogout(w http.ResponseWriter, r *http.Request) {
	if !accountMethod(w, r, "POST") || !s.requireAdminJSON(w, r) {
		return
	}
	if cookie, err := r.Cookie("mnet_account"); err == nil {
		s.revokeAccountSession(cookie.Value)
	}
	s.accountCookie(w, "mnet_account", "", time.Unix(1, 0), -1)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
