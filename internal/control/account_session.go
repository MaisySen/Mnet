package control

import (
	"fmt"
	"sync"
	"time"
)

const maxAccountSessions = 1024
const accountSessionLifetime = 8 * time.Hour

// Account sessions are separate from both operator sessions and device join
// credentials. Only a verified OIDC callback may eventually issue one. They
// authorize the existing own-device query, never mesh access or membership.
type accountSession struct {
	NetworkID, Issuer, Subject string
	Provider                   *OIDCProvider
	Expires                    time.Time
	BrowserHash                [32]byte // private callback-flow revocation binding
}
type accountSessions struct {
	mu      sync.Mutex
	byToken map[string]accountSession
	pending map[string]accountLogin
}

// Only the verified account OIDC callback issues tokens. Caller claims must be
// verified by the exact current provider, not supplied by JSON or display email.
func (s *Server) newAccountSession(networkID string, p *OIDCProvider, claims *OIDCClaims) (string, error) {
	return s.newAccountSessionFromLogin(networkID, p, claims, "", nil)
}
func (s *Server) newAccountSessionFromLogin(networkID string, p *OIDCProvider, claims *OIDCClaims, state string, login *accountLogin) (string, error) {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if p == nil || p != s.oidc || claims == nil || claims.Subject == "" {
		return "", ErrUnauthorized
	}
	if _, err := s.store.IdentityDevices(networkID, p.cfg.Issuer, claims.Subject); err != nil {
		return "", ErrUnauthorized
	}
	now := time.Now()
	s.accounts.mu.Lock()
	defer s.accounts.mu.Unlock()
	var browserHash [32]byte
	if login != nil {
		current, ok := s.accounts.pending[state]
		if !ok || current != *login || !current.claimed || !now.Before(current.Expires) {
			return "", ErrUnauthorized
		}
		browserHash = current.BrowserHash
	}
	if s.accounts.byToken == nil {
		s.accounts.byToken = make(map[string]accountSession)
	}
	for token, session := range s.accounts.byToken {
		if !now.Before(session.Expires) {
			delete(s.accounts.byToken, token)
		}
	}
	if len(s.accounts.byToken) >= maxAccountSessions {
		return "", fmt.Errorf("too many account sessions")
	}
	token := newOIDCState()
	s.accounts.byToken[token] = accountSession{NetworkID: networkID, Issuer: p.cfg.Issuer, Subject: claims.Subject, Provider: p, Expires: now.Add(accountSessionLifetime), BrowserHash: browserHash}
	return token, nil
}

func (s *Server) accountDevices(token string) ([]IdentityDevice, error) {
	_, devices, err := s.accountDeviceView(token)
	return devices, err
}

func (s *Server) accountDeviceView(token string) (string, []IdentityDevice, error) {
	if token == "" {
		return "", nil, ErrUnauthorized
	}
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	s.accounts.mu.Lock()
	session, ok := s.accounts.byToken[token]
	if ok && !time.Now().Before(session.Expires) {
		delete(s.accounts.byToken, token)
		ok = false
	}
	s.accounts.mu.Unlock()
	if !ok || session.Provider != s.oidc || session.Provider == nil || session.Issuer != session.Provider.cfg.Issuer {
		return "", nil, ErrUnauthorized
	}
	// Every query checks the current explicit membership, not a cached grant.
	devices, err := s.store.IdentityDevices(session.NetworkID, session.Issuer, session.Subject)
	if err != nil {
		return "", nil, err
	}
	return session.NetworkID, devices, nil
}

func (s *Server) revokeAccountSession(token string) {
	s.accounts.mu.Lock()
	delete(s.accounts.byToken, token)
	s.accounts.mu.Unlock()
}

// Caller holds accounts.mu. Proofs and their hashes are private server state,
// never account/device authority and never exposed in a response or log.
func (a *accountSessions) retireBrowserProof(hash [32]byte) {
	if hash == ([32]byte{}) {
		return
	}
	for state, login := range a.pending {
		if login.BrowserHash == hash {
			delete(a.pending, state)
		}
	}
	for token, session := range a.byToken {
		if session.BrowserHash == hash {
			delete(a.byToken, token)
		}
	}
}
