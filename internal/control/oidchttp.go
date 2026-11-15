package control

import (
	"fmt"
	"net/http"
	"time"
)

// oidcState binds one login round trip to the node waiting for approval.
type oidcState struct {
	NodeID string
	Exp    time.Time
}

// SetOIDC enables SSO login: joins without an auth key send the user
// through the provider instead of waiting for manual approval.
func (s *Server) SetOIDC(p *OIDCProvider) {
	s.oidcMu.Lock()
	s.oidc = p
	s.oidcMu.Unlock()
}

func (s *Server) oidcProvider() *OIDCProvider {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	return s.oidc
}

func publicURL(r *http.Request, path string) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
		if f := r.Header.Get("X-Forwarded-Proto"); f != "" {
			scheme = f
		}
	}
	return scheme + "://" + r.Host + path
}

// handleOIDCStart kicks off the login: the node must prove its identity
// with its join token, then the browser goes to the provider.
func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	p := s.oidcProvider()
	if p == nil {
		writeErr(w, http.StatusNotFound, "sso is not configured")
		return
	}
	node, err := s.store.AuthorizeNode(r.URL.Query().Get("node_id"), r.URL.Query().Get("token"))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unknown node or token")
		return
	}
	state := newOIDCState()
	s.oidcStatesMu.Lock()
	for k, st := range s.oidcStates {
		if time.Now().After(st.Exp) {
			delete(s.oidcStates, k)
		}
	}
	s.oidcStates[state] = oidcState{NodeID: node.ID, Exp: time.Now().Add(10 * time.Minute)}
	s.oidcStatesMu.Unlock()
	http.Redirect(w, r, p.AuthURL(state, publicURL(r, "/oidc/callback")), http.StatusFound)
}

// handleOIDCCallback finishes the login: verified identity approves the
// waiting node.
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	p := s.oidcProvider()
	if p == nil {
		writeErr(w, http.StatusNotFound, "sso is not configured")
		return
	}
	state := r.URL.Query().Get("state")
	s.oidcStatesMu.Lock()
	st, ok := s.oidcStates[state]
	if ok {
		delete(s.oidcStates, state)
	}
	s.oidcStatesMu.Unlock()
	if !ok || time.Now().After(st.Exp) {
		writeErr(w, http.StatusBadRequest, "login state unknown or expired; try connecting again")
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		writeErr(w, http.StatusForbidden, "identity provider refused: "+e)
		return
	}
	claims, err := p.Exchange(r.Context(), r.URL.Query().Get("code"), publicURL(r, "/oidc/callback"))
	if err != nil {
		writeErr(w, http.StatusForbidden, "login failed: "+err.Error())
		return
	}
	user := claims.Email
	if user == "" {
		user = claims.Subject
	}
	if err := s.store.ApproveNodeAs(st.NodeID, user); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Mnet</title>
<body style="font-family:system-ui;max-width:36em;margin:4em auto;padding:0 1em">
<h1>Device connected</h1>
<p>Signed in as <b>%s</b>. This device is now on the mesh. You can close this tab.</p>
<footer><small>Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia.</small></footer>`, user)
}
