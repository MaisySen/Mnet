package multies

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type verifyFunc func(context.Context, Request) (Evidence, error)

func (f verifyFunc) Verify(c context.Context, r Request) (Evidence, error) { return f(c, r) }

var clock = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func valid() (Request, Evidence) {
	r := Request{Credential: "opaque-test-value-only", WorkspaceID: "org_fixture", DeviceKey: strings.Repeat("A", 43), Capability: View}
	e := Evidence{UserID: "user_fixture", WorkspaceID: r.WorkspaceID, DeviceKey: r.DeviceKey, Audience: "mnet", AccountState: "active", WorkspaceState: "active", MembershipState: "active", VerifiedEmail: true, Allowed: []Capability{View}, CheckedAt: clock, ExpiresAt: clock.Add(30 * time.Second)}
	return r, e
}
func policy(e Evidence) Policy {
	return Policy{Verifier: verifyFunc(func(context.Context, Request) (Evidence, error) { return e, nil }), Now: func() time.Time { return clock }}
}
func denied(t *testing.T, g Grant, err error) {
	t.Helper()
	if !errors.Is(err, ErrDenied) || g != (Grant{}) {
		t.Fatalf("want empty denial, got %+v / %v", g, err)
	}
}

func TestPolicyAcceptsOnlyExactFreshGrant(t *testing.T) {
	r, e := valid()
	g, err := policy(e).Authorize(context.Background(), r)
	if err != nil || g.UserID != e.UserID || g.WorkspaceID != r.WorkspaceID || g.DeviceKey != r.DeviceKey || g.Capability != View {
		t.Fatalf("grant %+v / %v", g, err)
	}
}
func TestPolicyDeniesEveryMissingOrConflictingAuthority(t *testing.T) {
	tests := []struct {
		name  string
		alter func(*Request, *Evidence)
	}{
		{"no credential", func(r *Request, e *Evidence) { r.Credential = "" }},
		{"control character credential", func(r *Request, e *Evidence) { r.Credential += "\n" }},
		{"large credential", func(r *Request, e *Evidence) { r.Credential = strings.Repeat("a", 8193) }},
		{"missing user", func(r *Request, e *Evidence) { e.UserID = "" }},
		{"email is not user ID", func(r *Request, e *Evidence) { e.UserID = "owner@example.invalid" }},
		{"missing workspace", func(r *Request, e *Evidence) { r.WorkspaceID = "" }},
		{"different workspace", func(r *Request, e *Evidence) { e.WorkspaceID = "org_other" }},
		{"different device", func(r *Request, e *Evidence) { e.DeviceKey = strings.Repeat("B", 43) }},
		{"empty device", func(r *Request, e *Evidence) { r.DeviceKey = "" }},
		{"noncanonical device thumbprint", func(r *Request, e *Evidence) { r.DeviceKey = strings.Repeat("B", 43) }},
		{"wrong audience", func(r *Request, e *Evidence) { e.Audience = "mcp" }},
		{"empty account state", func(r *Request, e *Evidence) { e.AccountState = "" }},
		{"disabled account", func(r *Request, e *Evidence) { e.AccountState = "disabled" }},
		{"suspended workspace", func(r *Request, e *Evidence) { e.WorkspaceState = "suspended" }},
		{"removed member", func(r *Request, e *Evidence) { e.MembershipState = "removed" }},
		{"unverified email", func(r *Request, e *Evidence) { e.VerifiedEmail = false }},
		{"demo", func(r *Request, e *Evidence) { e.Demo = true }},
		{"guest", func(r *Request, e *Evidence) { e.Guest = true }},
		{"SSO blocked", func(r *Request, e *Evidence) { e.SSOBlocked = true }},
		{"no capability", func(r *Request, e *Evidence) { e.Allowed = nil }},
		{"view does not imply control", func(r *Request, e *Evidence) { r.Capability = Control }},
		{"view does not imply transfer", func(r *Request, e *Evidence) { r.Capability = Transfer }},
		{"view does not imply record", func(r *Request, e *Evidence) { r.Capability = Record }},
		{"view does not imply enroll", func(r *Request, e *Evidence) { r.Capability = Enroll }},
		{"unknown requested capability", func(r *Request, e *Evidence) { r.Capability = "admin" }},
		{"unknown granted capability", func(r *Request, e *Evidence) { e.Allowed = append(e.Allowed, "full") }},
		{"missing freshness", func(r *Request, e *Evidence) { e.CheckedAt = time.Time{} }},
		{"stale", func(r *Request, e *Evidence) { e.CheckedAt = clock.Add(-MaxEvidenceAge - time.Nanosecond) }},
		{"future check", func(r *Request, e *Evidence) { e.CheckedAt = clock.Add(time.Nanosecond) }},
		{"expired", func(r *Request, e *Evidence) { e.ExpiresAt = clock }},
		{"long lifetime", func(r *Request, e *Evidence) { e.ExpiresAt = clock.Add(MaxGrantLifetime + time.Nanosecond) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, e := valid()
			tc.alter(&r, &e)
			g, err := policy(e).Authorize(context.Background(), r)
			denied(t, g, err)
		})
	}
}
func TestWorkspaceListCannotCarryDeviceScope(t *testing.T) {
	r, e := valid()
	r.Capability = List
	e.Allowed = []Capability{List}
	r.DeviceKey = ""
	e.DeviceKey = ""
	if _, err := policy(e).Authorize(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	r.DeviceKey = strings.Repeat("A", 43)
	g, err := policy(e).Authorize(context.Background(), r)
	denied(t, g, err)
}
func TestZeroAndFailedVerifiersDoNotLeakErrorsOrGrant(t *testing.T) {
	r, _ := valid()
	g, err := (Policy{}).Authorize(context.Background(), r)
	denied(t, g, err)
	p := Policy{Verifier: verifyFunc(func(context.Context, Request) (Evidence, error) {
		return Evidence{}, errors.New("private upstream details")
	})}
	g, err = p.Authorize(context.Background(), r)
	denied(t, g, err)
	if strings.Contains(err.Error(), "private") {
		t.Fatal("leaked upstream error")
	}
	g, err = p.Authorize(nil, r)
	denied(t, g, err)
}
func TestCancellationBeforeOrDuringVerificationDenies(t *testing.T) {
	r, e := valid()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g, err := policy(e).Authorize(ctx, r)
	denied(t, g, err)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	p := policy(e)
	p.Verifier = verifyFunc(func(context.Context, Request) (Evidence, error) { cancel(); return e, nil })
	g, err = p.Authorize(ctx, r)
	denied(t, g, err)
}
func TestRevocationRechecksInsteadOfReusingCachedAuthority(t *testing.T) {
	r, e := valid()
	calls := 0
	p := policy(e)
	p.Verifier = verifyFunc(func(context.Context, Request) (Evidence, error) {
		calls++
		if calls > 1 {
			e.MembershipState = "removed"
		}
		return e, nil
	})
	if _, err := p.Authorize(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	g, err := p.Authorize(context.Background(), r)
	denied(t, g, err)
	if calls != 2 {
		t.Fatal("missing revalidation")
	}
}
func TestConcurrentIndependentWorkspacesStayScoped(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := valid()
			p := policy(e)
			if _, err := p.Authorize(context.Background(), r); err != nil {
				t.Error(err)
			}
			r.WorkspaceID = "org_other"
			g, err := p.Authorize(context.Background(), r)
			if g != (Grant{}) || !errors.Is(err, ErrDenied) {
				t.Error("cross-workspace grant")
			}
		}()
	}
	wg.Wait()
}
func FuzzPolicyNeverElevatesCallerScope(f *testing.F) {
	f.Add("org_fixture", strings.Repeat("A", 43), "view")
	f.Add("org_other", "", "control")
	f.Fuzz(func(t *testing.T, workspace, key, capability string) {
		r, e := valid()
		r.WorkspaceID = workspace
		r.DeviceKey = key
		r.Capability = Capability(capability)
		g, err := policy(e).Authorize(context.Background(), r)
		if err == nil && (g.WorkspaceID != e.WorkspaceID || g.DeviceKey != e.DeviceKey || g.Capability != View) {
			t.Fatalf("elevated grant %+v", g)
		}
	})
}
