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
}
type accountSessions struct {
	mu      sync.Mutex
	byToken map[string]accountSession
}

// No public HTTP route issues/accepts these tokens yet. Caller claims must be
// verified by the exact current provider, not supplied by JSON or display email.
func (s *Server) newAccountSession(networkID string, p *OIDCProvider, claims *OIDCClaims) (string, error) {
	if p == nil || p != s.oidcProvider() || claims == nil || claims.Subject == "" {
		return "", ErrUnauthorized
	}
	if _, err := s.store.IdentityDevices(networkID, p.cfg.Issuer, claims.Subject); err != nil {
		return "", ErrUnauthorized
	}
	now := time.Now()
	s.accounts.mu.Lock()
	defer s.accounts.mu.Unlock()
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
	s.accounts.byToken[token] = accountSession{NetworkID: networkID, Issuer: p.cfg.Issuer, Subject: claims.Subject, Provider: p, Expires: now.Add(accountSessionLifetime)}
	return token, nil
}

func (s *Server) accountDevices(token string) ([]IdentityDevice, error) {
	if token == "" {
		return nil, ErrUnauthorized
	}
	s.accounts.mu.Lock()
	session, ok := s.accounts.byToken[token]
	if ok && !time.Now().Before(session.Expires) {
		delete(s.accounts.byToken, token)
		ok = false
	}
	s.accounts.mu.Unlock()
	if !ok || session.Provider != s.oidcProvider() || session.Provider == nil || session.Issuer != session.Provider.cfg.Issuer {
		return nil, ErrUnauthorized
	}
	// Every query checks the current explicit membership, not a cached grant.
	return s.store.IdentityDevices(session.NetworkID, session.Issuer, session.Subject)
}

func (s *Server) revokeAccountSession(token string) {
	s.accounts.mu.Lock()
	delete(s.accounts.byToken, token)
	s.accounts.mu.Unlock()
}
