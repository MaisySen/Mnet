package control

import (
	"fmt"
	"html"
	"net/http"
	"time"
)

// oidcState binds one login round trip to the node waiting for approval.
type oidcState struct {
	NodeID   string
	Exp      time.Time
	Provider *OIDCProvider
	claimed  bool // retained through exchange so provider replacement can retire it
}

const maxPendingOIDCStates = 1024

// SetOIDC enables SSO login: joins without an auth key send the user
// through the provider instead of waiting for manual approval.
func (s *Server) SetOIDC(p *OIDCProvider) {
	s.oidcMu.Lock()
	if s.oidc != p {
		// Provider replacement is a permanent revocation boundary. Merely
		// comparing pointers on reads would revive old sessions if the old
		// provider object were installed again later.
		s.accounts.mu.Lock()
		s.accounts.byToken = nil
		s.accounts.pending = nil
		s.accounts.mu.Unlock()
		// Retirement is permanent even if the same provider object is later
		// installed again. In-flight exchanges retain a claimed state until
		// completion, so clearing this map also revokes their final approval.
		s.oidcStatesMu.Lock()
		s.oidcStates = make(map[string]oidcState)
		s.oidcStatesMu.Unlock()
	}
	s.oidc = p
	s.oidcMu.Unlock()
}

func (s *Server) oidcProvider() *OIDCProvider {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	return s.oidc
}

// publicURL uses the operator-configured origin, never caller-controlled
// Host or forwarding headers. Join URLs can contain node credentials.
func (s *Server) publicURL(path string) string { return s.publicBase + path }

// handleOIDCStart kicks off the login: the node must prove its identity
// with its join token, then the browser goes to the provider.
func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if !oidcGET(w, r) {
		return
	}
	// Keep provider selection and state insertion in one retirement scope.
	// Provider calls are not made here; AuthURL only formats local metadata.
	s.oidcMu.Lock()
	p := s.oidc
	if p == nil {
		s.oidcMu.Unlock()
		writeErr(w, http.StatusNotFound, "sso is not configured")
		return
	}
	node, err := s.store.AuthorizeNode(r.URL.Query().Get("node_id"), r.URL.Query().Get("token"))
	if err != nil {
		s.oidcMu.Unlock()
		writeErr(w, http.StatusUnauthorized, "unknown node or token")
		return
	}
	state := newOIDCState()
	s.oidcStatesMu.Lock()
	for k, st := range s.oidcStates {
		if !time.Now().Before(st.Exp) {
			delete(s.oidcStates, k)
		}
	}
	if len(s.oidcStates) >= maxPendingOIDCStates {
		s.oidcStatesMu.Unlock()
		s.oidcMu.Unlock()
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many pending logins; try again later")
		return
	}
	s.oidcStates[state] = oidcState{NodeID: node.ID, Exp: time.Now().Add(10 * time.Minute), Provider: p}
	s.oidcStatesMu.Unlock()
	s.oidcMu.Unlock()
	http.Redirect(w, r, p.AuthURL(state, s.publicURL("/oidc/callback")), http.StatusFound)
}

// handleOIDCCallback finishes the login: verified identity approves the
// waiting node.
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if !oidcGET(w, r) {
		return
	}
	p := s.oidcProvider()
	if p == nil {
		writeErr(w, http.StatusNotFound, "sso is not configured")
		return
	}
	state := r.URL.Query().Get("state")
	s.oidcStatesMu.Lock()
	st, ok := s.oidcStates[state]
	if ok && !st.claimed {
		st.claimed = true
		s.oidcStates[state] = st
	} else {
		ok = false
	}
	s.oidcStatesMu.Unlock()
	if ok {
		defer func() {
			s.oidcStatesMu.Lock()
			if s.oidcStates[state] == st {
				delete(s.oidcStates, state)
			}
			s.oidcStatesMu.Unlock()
		}()
	}
	if !ok || !time.Now().Before(st.Exp) || st.Provider != p {
		writeErr(w, http.StatusBadRequest, "login state unknown or expired; try connecting again")
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		writeErr(w, http.StatusForbidden, "identity provider refused: "+e)
		return
	}
	claims, err := p.Exchange(r.Context(), r.URL.Query().Get("code"), s.publicURL("/oidc/callback"), state)
	if err != nil {
		writeErr(w, http.StatusForbidden, "login failed: "+err.Error())
		return
	}
	if err := s.approveOIDCState(state, st, claims); err != nil {
		writeErr(w, http.StatusForbidden, "login state retired or approval unavailable; try connecting again")
		return
	}
	user := claims.Email
	if user == "" {
		user = claims.Subject
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, deviceApprovedHTML, html.EscapeString(user))
}

// Only the query-based authorization-code flow is supported. Reject other
// methods before allocating or consuming state.
func oidcGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return false
	}
	return true
}

// Final authorization and persistence serialize with SetOIDC. No provider
// network request runs under these locks. A completed approval is not undone
// by a later provider replacement; a replacement that wins rejects the flow.
func (s *Server) approveOIDCState(state string, attempt oidcState, claims *OIDCClaims) error {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if attempt.Provider == nil || attempt.Provider != s.oidc || claims == nil {
		return ErrUnauthorized
	}
	s.oidcStatesMu.Lock()
	current, ok := s.oidcStates[state]
	valid := ok && current == attempt && current.claimed && time.Now().Before(current.Exp)
	s.oidcStatesMu.Unlock()
	if !valid {
		return ErrUnauthorized
	}
	user := claims.Email
	if user == "" {
		user = claims.Subject
	}
	return s.store.ApproveNodeIdentity(attempt.NodeID, user, attempt.Provider.cfg.Issuer, claims.Subject)
}
