package control

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/multies"
)

const (
	inventoryA          = "workspace_fixture_a"
	inventoryB          = "workspace_fixture_b"
	inventoryAdmin      = "test-opaque-admin-credential"
	inventoryMember     = "test-opaque-member-credential"
	inventoryOtherAdmin = "test-opaque-other-admin"
)

type inventoryFixtureVerifier struct {
	now     time.Time
	blocked bool
	fault   bool
}

func (v *inventoryFixtureVerifier) Verify(_ context.Context, r multies.Request) (multies.Evidence, error) {
	if v.fault {
		return multies.Evidence{}, errors.New("private verifier detail")
	}
	allowed := []multies.Capability{multies.List}
	actor := "fixture_member"
	switch r.Credential {
	case inventoryMember:
	case inventoryAdmin, inventoryOtherAdmin:
		allowed = append(allowed, multies.Enroll, multies.Remove)
		actor = "fixture_admin"
		if r.Credential == inventoryOtherAdmin {
			actor = "fixture_other_admin"
		}
	default:
		return multies.Evidence{}, multies.ErrDenied
	}
	// Only fixture credentials; the real verifier must resolve workspace-specific
	// roles, session binding, and SSO rather than infer them from a request.
	return multies.Evidence{UserID: actor, WorkspaceID: r.WorkspaceID, DeviceKey: r.DeviceKey,
		Audience: "mnet", AccountState: "active", WorkspaceState: "active", MembershipState: "active", VerifiedEmail: true,
		SSOBlocked: v.blocked, Allowed: allowed, CheckedAt: v.now, ExpiresAt: v.now.Add(30 * time.Second)}, nil
}

func inventoryFixture(t *testing.T) (*Store, *inventoryFixtureVerifier, *MultiesInventory) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	v := &inventoryFixtureVerifier{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return s, v, inventoryFor(t, s, v, inventoryA)
}

func (v *inventoryFixtureVerifier) policy() multies.Policy {
	return multies.Policy{Verifier: v, Now: func() time.Time { return v.now }}
}

func inventoryFor(t *testing.T, s *Store, v *inventoryFixtureVerifier, workspace string) *MultiesInventory {
	t.Helper()
	i, err := NewMultiesInventory(s, "staging", true, workspace, v.policy())
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func newInventoryKey(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := k.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return k, pub
}

func signInventory(t *testing.T, k *ecdsa.PrivateKey, c EnrollmentConsent) []byte {
	t.Helper()
	digest := sha256.Sum256(c.SigningBytes())
	sig, err := ecdsa.SignASN1(rand.Reader, k, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func beginInventory(t *testing.T, i *MultiesInventory, k []byte) EnrollmentConsent {
	t.Helper()
	c, err := i.BeginEnrollment(t.Context(), i.workspaceID, inventoryAdmin, "Fixture laptop", k)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func listInventory(t *testing.T, i *MultiesInventory, want ...InventoryDevice) {
	t.Helper()
	got, err := i.List(t.Context(), i.workspaceID, inventoryMember)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("List = %v, %v, want %v", got, err, want)
	}
}

func redeemInventory(t *testing.T, i *MultiesInventory, c EnrollmentConsent, pub, sig []byte) InventoryDevice {
	t.Helper()
	d, err := i.RedeemEnrollment(t.Context(), i.workspaceID, inventoryAdmin, c.ID, pub, sig)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func inventoryDenied(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, multies.ErrDenied) {
		t.Fatalf("want denial, got %v", err)
	}
}

func TestMultiesInventoryDisabledOrUnauthenticated(t *testing.T) {
	s, v, i := inventoryFixture(t)
	for _, tc := range []struct {
		env, workspace string
		enabled        bool
		policy         multies.Policy
	}{
		{"", inventoryA, true, v.policy()}, {"production", inventoryA, true, v.policy()},
		{"staging", inventoryA, false, v.policy()}, {"staging", "", true, v.policy()},
		{"staging", inventoryA, true, multies.Policy{}},
	} {
		got, err := NewMultiesInventory(s, tc.env, tc.enabled, tc.workspace, tc.policy)
		if got != nil {
			t.Fatal("initialized unsafe configuration")
		}
		inventoryDenied(t, err)
	}
	if got, err := (&MultiesInventory{}).List(t.Context(), inventoryA, inventoryMember); got != nil {
		t.Fatal(got)
	} else {
		inventoryDenied(t, err)
	}
	if got, err := i.List(t.Context(), inventoryB, inventoryAdmin); got != nil {
		t.Fatal(got)
	} else {
		inventoryDenied(t, err)
	}
	if got, err := i.List(t.Context(), inventoryA, "fixture-legacy-key-not-a-grant"); got != nil {
		t.Fatal(got)
	} else {
		inventoryDenied(t, err)
	}
	_, pub := newInventoryKey(t)
	_, err := i.BeginEnrollment(t.Context(), inventoryA, inventoryMember, "Fixture laptop", pub)
	inventoryDenied(t, err)
	_, err = i.BeginEnrollment(t.Context(), inventoryA, inventoryAdmin, "Fixture laptop", []byte("not a public key"))
	inventoryDenied(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = i.List(ctx, inventoryA, inventoryMember)
	inventoryDenied(t, err)
	listInventory(t, i)
}

func TestMultiesInventoryConsentIsExactFreshAndSingleUse(t *testing.T) {
	_, v, i := inventoryFixture(t)
	k, pub := newInventoryKey(t)
	k2, pub2 := newInventoryKey(t)
	c := beginInventory(t, i, pub)
	listInventory(t, i) // beginning without consent changes no inventory
	sig := signInventory(t, k, c)
	changed := c
	changed.Label = "Other label"
	for _, attempt := range []struct {
		credential, workspace string
		pub, sig              []byte
	}{
		{inventoryAdmin, inventoryA, pub, nil},
		{inventoryAdmin, inventoryA, pub, signInventory(t, k, changed)},
		{inventoryAdmin, inventoryA, pub2, signInventory(t, k2, c)},
		{inventoryAdmin, inventoryB, pub, sig},
		{inventoryOtherAdmin, inventoryA, pub, sig},
		{inventoryMember, inventoryA, pub, sig},
	} {
		d, err := i.RedeemEnrollment(t.Context(), attempt.workspace, attempt.credential, c.ID, attempt.pub, attempt.sig)
		if d != (InventoryDevice{}) {
			t.Fatal(d)
		}
		inventoryDenied(t, err)
	}
	v.blocked = true
	_, err := i.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, c.ID, pub, sig)
	inventoryDenied(t, err)
	v.blocked, v.fault = false, true
	_, err = i.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, c.ID, pub, sig)
	inventoryDenied(t, err)
	v.fault = false
	listInventory(t, i)
	d := redeemInventory(t, i, c, pub, sig)
	listInventory(t, i, d)
	_, err = i.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, c.ID, pub, sig)
	inventoryDenied(t, err)
	_, err = i.BeginEnrollment(t.Context(), inventoryA, inventoryAdmin, "Another label", pub)
	inventoryDenied(t, err)
	expired := beginInventory(t, i, pub2)
	v.now = expired.ExpiresAt
	_, err = i.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, expired.ID, pub2, signInventory(t, k2, expired))
	inventoryDenied(t, err)
	listInventory(t, i, d)
}

func TestMultiesInventoryPersistsAndRemovalRevokesPendingConsent(t *testing.T) {
	s, v, a := inventoryFixture(t)
	b := inventoryFor(t, s, v, inventoryB)
	k, pub := newInventoryKey(t)
	c1, c2 := beginInventory(t, a, pub), beginInventory(t, a, pub)
	cb := beginInventory(t, b, pub)
	da := redeemInventory(t, a, c1, pub, signInventory(t, k, c1))
	listInventory(t, b)
	inventoryDenied(t, b.Remove(t.Context(), inventoryB, inventoryAdmin, da.ID))
	inventoryDenied(t, a.Remove(t.Context(), inventoryA, inventoryMember, da.ID))
	listInventory(t, a, da)
	if err := a.Remove(t.Context(), inventoryA, inventoryOtherAdmin, da.ID); err != nil {
		t.Fatal(err)
	}
	listInventory(t, a)
	_, err := a.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, c2.ID, pub, signInventory(t, k, c2))
	inventoryDenied(t, err)
	db := redeemInventory(t, b, cb, pub, signInventory(t, k, cb))
	fresh := beginInventory(t, a, pub)
	_, err = a.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, fresh.ID, pub, signInventory(t, k, c2))
	inventoryDenied(t, err)
	newA := redeemInventory(t, a, fresh, pub, signInventory(t, k, fresh))
	if newA.ID == da.ID || db.ID == da.ID {
		t.Fatal("reused an inventory identity")
	}

	// Re-open from disk. Pending challenges are not part of serialized state.
	_, pendingPub := newInventoryKey(t)
	pending := beginInventory(t, a, pendingPub)
	s, err = Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	listInventory(t, inventoryFor(t, s, v, inventoryA), newA)
	listInventory(t, inventoryFor(t, s, v, inventoryB), db)
	old := s.data.MultiesInventory[da.ID]
	if old.RemovedAt == nil || old.RemovedBy != "fixture_other_admin" || old.EnrolledBy != "fixture_admin" || old.WorkspaceID != inventoryA {
		t.Fatal(old)
	}
	if len(s.inventoryPending) != 0 || len(s.data.Nodes) != 0 || len(s.data.NodeTokens) != 0 || len(s.data.AuthKeys) != 0 {
		t.Fatal("inventory leaked into auth, mesh, or persistent handoff state")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), inventoryAdmin) || strings.Contains(string(data), pending.ID) {
		t.Fatal("persisted credentials or pending handoff")
	}
	projected, err := json.Marshal(newA)
	if err != nil || strings.Contains(string(projected), "fixture_admin") || strings.Contains(string(projected), old.KeyThumbprint) {
		t.Fatal("leaked internal fields")
	}
}

func TestMultiesInventoryWriteFailureRollsBack(t *testing.T) {
	s, _, i := inventoryFixture(t)
	k, pub := newInventoryKey(t)
	c := beginInventory(t, i, pub)
	sig := signInventory(t, k, c)
	path := s.path
	s.path = t.TempDir() // directory cannot be the destination file
	d, err := i.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, c.ID, pub, sig)
	if !errors.Is(err, errInventoryWrite) || d != (InventoryDevice{}) {
		t.Fatalf("unexpected enrollment %v / %v", d, err)
	}
	listInventory(t, i)
	s.path = path
	d = redeemInventory(t, i, c, pub, sig)
	s.path = t.TempDir()
	if err = i.Remove(t.Context(), inventoryA, inventoryAdmin, d.ID); !errors.Is(err, errInventoryWrite) {
		t.Fatal(err)
	}
	listInventory(t, i, d)
	s.path = path
	if err = i.Remove(t.Context(), inventoryA, inventoryAdmin, d.ID); err != nil {
		t.Fatal(err)
	}
	listInventory(t, i)
}

func TestMultiesInventoryConcurrentRedemptionHasOneWinner(t *testing.T) {
	_, _, i := inventoryFixture(t)
	k, pub := newInventoryKey(t)
	c := beginInventory(t, i, pub)
	sig := signInventory(t, k, c)
	var wg sync.WaitGroup
	var successes atomic.Int64
	for n := 0; n < 12; n++ {
		wg.Go(func() {
			_, err := i.RedeemEnrollment(t.Context(), inventoryA, inventoryAdmin, c.ID, pub, sig)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, multies.ErrDenied) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("got %d successful redemptions", successes.Load())
	}
}

func TestMultiesInventoryInvalidStoredScopeDoesNotReplaceState(t *testing.T) {
	s, _, i := inventoryFixture(t)
	k, pub := newInventoryKey(t)
	c := beginInventory(t, i, pub)
	d := redeemInventory(t, i, c, pub, signInventory(t, k, c))
	rec := s.data.MultiesInventory[d.ID]
	rec.WorkspaceID = ""
	s.data.MultiesInventory[d.ID] = rec
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(s.path); err == nil {
		t.Fatal("accepted a record without a workspace")
	}
	after, err := os.ReadFile(s.path)
	if err != nil || !slices.Equal(before, after) {
		t.Fatalf("invalid store was not left intact: %v", err)
	}
}
