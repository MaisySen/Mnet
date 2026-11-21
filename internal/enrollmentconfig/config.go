// Package enrollmentconfig validates an inactive operator handoff contract.
// It has no implicit file loader, environment defaults, route, network client, membership
// writer or enrollment caller. A validated mapping is configuration, not consent
// or proof of a person's current authority.
package enrollmentconfig

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

var ErrInvalid = errors.New("invalid enrollment operator mapping")

// Principal is an immutable authenticated identity, never an email match.
type Principal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}
type Binding struct {
	WorkspaceID string `json:"workspace_id"`
	NetworkID   string `json:"network_id"`
	// Explicit principals are a proposed minimum policy, not workspace membership.
	// More permissive role policies require a separate design and review.
	Principals []Principal `json:"principals"`
}
type Config struct {
	Version         int       `json:"version"`
	Revision        string    `json:"revision"`
	MultiesOrigin   string    `json:"multies_origin"`
	Issuer          string    `json:"issuer"`
	RegistrarOrigin string    `json:"registrar_origin"`
	Audience        string    `json:"audience"`
	Bindings        []Binding `json:"bindings"`
}

// Mapping holds a private copy so a caller cannot change validated policy later.
// Its fields are not an authorization grant and must never enable enrollment.
type Mapping struct {
	revision, multiesOrigin, issuer, registrarOrigin, audience string
	bindings                                                   map[string]Binding
}

func text(s string, max int) bool {
	if s == "" || len(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
func endpoint(s string, origin bool) bool {
	if !text(s, 2048) || strings.ContainsAny(s, "?#") || strings.HasSuffix(s, ":") {
		return false
	}
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Port() != "" {
		n, e := strconv.Atoi(u.Port())
		if e != nil || n < 1 || n > 65535 {
			return false
		}
	}
	if origin && (u.Path != "" || u.RawPath != "") {
		return false
	}
	// Require an exact operator spelling, not client normalization or redirects.
	return u.String() == s
}
func Validate(c Config) (*Mapping, error) {
	if c.Version != 1 || !text(c.Revision, 128) || !endpoint(c.MultiesOrigin, true) || !endpoint(c.RegistrarOrigin, true) || !endpoint(c.Issuer, false) || !text(c.Audience, 256) || len(c.Bindings) == 0 || len(c.Bindings) > 1024 {
		return nil, ErrInvalid
	}
	m := &Mapping{revision: c.Revision, multiesOrigin: c.MultiesOrigin, issuer: c.Issuer, registrarOrigin: c.RegistrarOrigin, audience: c.Audience, bindings: make(map[string]Binding, len(c.Bindings))}
	for _, b := range c.Bindings {
		if !text(b.WorkspaceID, 128) || !text(b.NetworkID, 128) || len(b.Principals) == 0 || len(b.Principals) > 1024 {
			return nil, ErrInvalid
		}
		if _, ok := m.bindings[b.WorkspaceID]; ok {
			return nil, ErrInvalid
		}
		seen := map[Principal]bool{}
		for _, p := range b.Principals {
			if p.Issuer != c.Issuer || !text(p.Subject, 256) || seen[p] {
				return nil, ErrInvalid
			}
			seen[p] = true
		}
		b.Principals = append([]Principal(nil), b.Principals...)
		m.bindings[b.WorkspaceID] = b
	}
	return m, nil
}

// Lookup returns an exact configured tuple and a defensive copy. It proves no
// live workspace/network existence, current membership, session or key ownership.
// Zero values and unknown workspaces return nothing; there is no legacy fallback.
func (m *Mapping) Lookup(workspace string) (Binding, bool) {
	if m == nil {
		return Binding{}, false
	}
	b, ok := m.bindings[workspace]
	if !ok {
		return Binding{}, false
	}
	b.Principals = append([]Principal(nil), b.Principals...)
	return b, true
}
