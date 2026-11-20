// Package multies defines Mnet's fail-closed authorization boundary.
// Its experimental BridgeClient speaks only to an explicitly configured origin;
// no shipped default, VPN enrollment or view/input/file authority is installed.
// The Multies-owned bridge must authenticate the person, resolve current
// membership and bind the exact device/capability before returning an Evidence.
// An identity-provider subject or caller-supplied Evidence is never sufficient.
package multies

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"
)

type Capability string

const (
	Enroll   Capability = "enroll"
	List     Capability = "list"
	View     Capability = "view"
	Control  Capability = "control"
	Transfer Capability = "transfer"
	Record   Capability = "record"
)

// Request is an exact authorization check, not a request to switch workspace.
// Credential is opaque to this policy and must never be logged or persisted.
// DeviceKey is the base64url SHA-256 thumbprint of the target/enrolling key.
// List is workspace-scoped and must not supply a DeviceKey. Every other action
// is device-scoped; view never implies control, transfer, record or enrollment.
type Request struct {
	Credential  string
	WorkspaceID string
	DeviceKey   string
	Capability  Capability
}

// Evidence may be returned only by the configured trusted bridge verifier.
// Account/workspace state and membership must be freshly resolved by that
// verifier, not inferred from an email address, OIDC login or local cache.
// Allowed contains only grants for this exact workspace and DeviceKey.
type Evidence struct {
	UserID          string       `json:"userId"`
	WorkspaceID     string       `json:"workspaceId"`
	DeviceKey       string       `json:"deviceKey"`
	Audience        string       `json:"audience"`
	AccountState    string       `json:"accountState"`
	WorkspaceState  string       `json:"workspaceState"`
	MembershipState string       `json:"membershipState"`
	VerifiedEmail   bool         `json:"verifiedEmail"`
	Demo            bool         `json:"demo"`
	Guest           bool         `json:"guest"`
	SSOBlocked      bool         `json:"ssoBlocked"`
	Allowed         []Capability `json:"allowed"`
	CheckedAt       time.Time    `json:"checkedAt"`
	ExpiresAt       time.Time    `json:"expiresAt"`
}

// Verifier is the future bridge's trust boundary. Implementations must perform
// cryptographic/transport verification and current membership/capability checks.
// A JSON decoder or identity-provider token verifier is not an implementation.
// The gated BridgeClient performs a fresh HTTP check; it is not enabled by default.
type Verifier interface {
	Verify(context.Context, Request) (Evidence, error)
}

// Policy checks one operation. Its zero value is deliberately unusable.
// No cached authority, enrollment endpoint or legacy-key fallback is installed.
type Policy struct {
	Verifier Verifier
	Now      func() time.Time
}

// Grant is scoped authority for the current operation only. It is not a bearer
// credential, persisted membership, a session consent or an enrollment result.
// Recheck for each privileged operation. The host's separate session consent
// and supported capabilities are still required for view/control operations.
type Grant struct {
	UserID      string
	WorkspaceID string
	DeviceKey   string
	Capability  Capability
	ExpiresAt   time.Time
}

var ErrDenied = errors.New("Multies authorization required")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func validDeviceKey(s string) bool {
	if len(s) != 43 {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}

// These are conservative local policy ceilings, not production latency or
// revocation promises. The final bridge must agree its measured contract.
const MaxEvidenceAge = 15 * time.Second
const MaxGrantLifetime = 60 * time.Second

func validCapability(c Capability) bool {
	switch c {
	case Enroll, List, View, Control, Transfer, Record:
		return true
	}
	return false
}

func (p Policy) Authorize(ctx context.Context, req Request) (Grant, error) {
	deny := func() (Grant, error) { return Grant{}, ErrDenied }
	if ctx == nil || ctx.Err() != nil || p.Verifier == nil {
		return deny()
	}
	if strings.TrimSpace(req.Credential) != req.Credential || strings.ContainsAny(req.Credential, "\r\n\x00") || len(req.Credential) < 16 || len(req.Credential) > 8192 || !idPattern.MatchString(req.WorkspaceID) || !validCapability(req.Capability) {
		return deny()
	}
	if req.Capability == List {
		if req.DeviceKey != "" {
			return deny()
		}
	} else if !validDeviceKey(req.DeviceKey) {
		return deny()
	}
	ev, err := p.Verifier.Verify(ctx, req)
	// Deliberately discard verifier errors: they may contain upstream credentials
	// or account details. Callers receive no identity/existence oracle.
	if err != nil || ctx.Err() != nil {
		return deny()
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	if !idPattern.MatchString(ev.UserID) || ev.Audience != "mnet" || ev.WorkspaceID != req.WorkspaceID || ev.DeviceKey != req.DeviceKey {
		return deny()
	}
	if ev.AccountState != "active" || ev.WorkspaceState != "active" || ev.MembershipState != "active" || !ev.VerifiedEmail || ev.Demo || ev.Guest || ev.SSOBlocked {
		return deny()
	}
	if ev.CheckedAt.IsZero() || ev.CheckedAt.After(now) || now.Sub(ev.CheckedAt) > MaxEvidenceAge || !ev.ExpiresAt.After(now) || ev.ExpiresAt.After(now.Add(MaxGrantLifetime)) || ev.ExpiresAt.Before(ev.CheckedAt) {
		return deny()
	}
	allowed := false
	for _, c := range ev.Allowed {
		if !validCapability(c) {
			return deny()
		}
		if c == req.Capability {
			allowed = true
		}
	}
	if !allowed {
		return deny()
	}
	return Grant{UserID: ev.UserID, WorkspaceID: ev.WorkspaceID, DeviceKey: ev.DeviceKey, Capability: req.Capability, ExpiresAt: ev.ExpiresAt}, nil
}
