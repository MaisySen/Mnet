package control

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fintex.au/mnet/internal/multies"
)

const (
	inventoryConsentDomain = "mnet.multies.inventory.enroll.v1"
	inventoryConsentTTL    = 2 * time.Minute
	maxInventoryPending    = 1024
)

var inventoryWorkspaceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
var errInventoryWrite = errors.New("could not save device inventory")

// InventoryDevice is the only public inventory projection. These are not mesh
// nodes; their IDs and signing keys never authorize joining or using the relay.
type InventoryDevice struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	EnrolledAt time.Time `json:"enrolled_at"`
}

// A removed record is retained with its provenance for audit. Re-enrollment
// creates a new ID and requires new consent. Never store credentials here.
type inventoryRecord struct {
	Device        InventoryDevice `json:"device"`
	WorkspaceID   string          `json:"workspace_id"`
	KeyThumbprint string          `json:"key_thumbprint"`
	EnrolledBy    string          `json:"enrolled_by"`
	RemovedAt     *time.Time      `json:"removed_at,omitempty"`
	RemovedBy     string          `json:"removed_by,omitempty"`
}

// EnrollmentConsent is the exact statement the native client must display the
// bound workspace and label for and sign only after explicit local approval.
// Signing uses ECDSA P-256/SHA-256, with ASN.1 DER signatures. The short-lived
// statement does not convey authorization without a fresh Multies grant.
type EnrollmentConsent struct {
	Purpose       string    `json:"purpose"`
	ID            string    `json:"id"`
	WorkspaceID   string    `json:"workspace_id"`
	UserID        string    `json:"user_id"`
	KeyThumbprint string    `json:"key_thumbprint"`
	Label         string    `json:"label"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// SigningBytes returns the statement as issued by Mnet. A transport should
// carry these exact bytes to the signer, not round-trip JSON for signing.
func (c EnrollmentConsent) SigningBytes() []byte {
	b, _ := json.Marshal(c) // only scalars and server-issued, in-range UTC time
	return b
}

// MultiesInventory is an in-process boundary; it does not register HTTP routes
// or provide a verifier. Only trusted staging bootstrap code may construct it.
// Authority must come from Multies, not from the request or legacy Mnet login.
type MultiesInventory struct {
	store       *Store
	workspaceID string
	policy      multies.Policy
}

func NewMultiesInventory(store *Store, environment string, enabled bool, workspaceID string, policy multies.Policy) (*MultiesInventory, error) {
	if store == nil || environment != "staging" || !enabled || !inventoryWorkspaceID.MatchString(workspaceID) || policy.Verifier == nil {
		return nil, multies.ErrDenied
	}
	return &MultiesInventory{store: store, workspaceID: workspaceID, policy: policy}, nil
}

func (i *MultiesInventory) authorize(ctx context.Context, workspaceID, credential, key string, capability multies.Capability) (multies.Grant, error) {
	if i == nil || i.store == nil || workspaceID != i.workspaceID {
		return multies.Grant{}, multies.ErrDenied
	}
	return i.policy.Authorize(ctx, multies.Request{Credential: credential, WorkspaceID: workspaceID, DeviceKey: key, Capability: capability})
}

// Keep the format canonical: SEC 1 uncompressed NIST P-256 points, hashed
// exactly as supplied. The standard parser validates that the point is on curve.
func inventoryKey(key []byte) (*ecdsa.PublicKey, string, error) {
	public, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), key)
	if err != nil {
		return nil, "", multies.ErrDenied
	}
	canonical, err := public.Bytes()
	if err != nil || !bytes.Equal(key, canonical) {
		return nil, "", multies.ErrDenied
	}
	sum := sha256.Sum256(canonical)
	return public, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func validInventoryLabel(label string) bool {
	if label == "" || len(label) > 80 || strings.TrimSpace(label) != label || !utf8.ValidString(label) {
		return false
	}
	for _, r := range label {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func (i *MultiesInventory) now() time.Time {
	if i.policy.Now != nil {
		return i.policy.Now().UTC()
	}
	return time.Now().UTC()
}

// List returns an exact workspace's active records after fresh authorization.
func (i *MultiesInventory) List(ctx context.Context, workspaceID, credential string) ([]InventoryDevice, error) {
	grant, err := i.authorize(ctx, workspaceID, credential, "", multies.List)
	if err != nil {
		return nil, err
	}
	s := i.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || !i.now().Before(grant.ExpiresAt) {
		return nil, multies.ErrDenied
	}
	return s.listMultiesDevicesLocked(grant.WorkspaceID), nil
}

// Shared projection; callers must authorize the exact workspace before holding the store lock.
func (s *Store) listMultiesDevicesLocked(workspaceID string) []InventoryDevice {
	result := []InventoryDevice{}
	for _, record := range s.data.MultiesInventory {
		if record.WorkspaceID == workspaceID && record.RemovedAt == nil {
			result = append(result, record.Device)
		}
	}
	sort.Slice(result, func(a, b int) bool { return result[a].ID < result[b].ID })
	return result
}

// BeginEnrollment only creates a bounded, process-local consent challenge.
// Nothing is enrolled until RedeemEnrollment verifies fresh authority and proof.
func (i *MultiesInventory) BeginEnrollment(ctx context.Context, workspaceID, credential, label string, key []byte) (EnrollmentConsent, error) {
	if !validInventoryLabel(label) {
		return EnrollmentConsent{}, multies.ErrDenied
	}
	_, thumbprint, err := inventoryKey(key)
	if err != nil {
		return EnrollmentConsent{}, err
	}
	grant, err := i.authorize(ctx, workspaceID, credential, thumbprint, multies.Enroll)
	if err != nil {
		return EnrollmentConsent{}, err
	}
	s := i.store
	s.mu.Lock()
	defer s.mu.Unlock()
	now := i.now()
	if ctx.Err() != nil || !now.Before(grant.ExpiresAt) || s.activeInventoryKeyLocked(workspaceID, grant.DeviceKey) {
		return EnrollmentConsent{}, multies.ErrDenied
	}
	for id, c := range s.inventoryPending {
		if !now.Before(c.ExpiresAt) {
			delete(s.inventoryPending, id)
		}
	}
	if len(s.inventoryPending) >= maxInventoryPending {
		return EnrollmentConsent{}, multies.ErrDenied
	}
	id := newID("consent")
	if _, exists := s.inventoryPending[id]; exists {
		return EnrollmentConsent{}, multies.ErrDenied
	}
	if s.inventoryPending == nil {
		s.inventoryPending = make(map[string]EnrollmentConsent)
	}
	c := EnrollmentConsent{Purpose: inventoryConsentDomain, ID: id, WorkspaceID: workspaceID, UserID: grant.UserID, KeyThumbprint: grant.DeviceKey, Label: label, ExpiresAt: now.Add(inventoryConsentTTL)}
	s.inventoryPending[id] = c
	return c, nil
}

func (s *Store) activeInventoryKeyLocked(workspaceID, thumbprint string) bool {
	for _, record := range s.data.MultiesInventory {
		if record.WorkspaceID == workspaceID && record.RemovedAt == nil && record.KeyThumbprint == thumbprint {
			return true
		}
	}
	return false
}

// RedeemEnrollment verifies the signature over the stored statement, not
// caller-supplied consent fields. Write and challenge burn are serialized;
// unsuccessful writes do not consume the challenge. Restart drops all pending
// challenges, while completed registrations remain in the real Store.
func (i *MultiesInventory) RedeemEnrollment(ctx context.Context, workspaceID, credential, consentID string, key []byte, signature []byte) (InventoryDevice, error) {
	if len(signature) == 0 || len(signature) > 72 {
		return InventoryDevice{}, multies.ErrDenied
	}
	public, thumbprint, err := inventoryKey(key)
	if err != nil {
		return InventoryDevice{}, err
	}
	grant, err := i.authorize(ctx, workspaceID, credential, thumbprint, multies.Enroll)
	if err != nil {
		return InventoryDevice{}, err
	}
	s := i.store
	s.mu.Lock()
	defer s.mu.Unlock()
	now := i.now()
	c, ok := s.inventoryPending[consentID]
	if !ok || ctx.Err() != nil || !now.Before(grant.ExpiresAt) || !now.Before(c.ExpiresAt) ||
		c.WorkspaceID != grant.WorkspaceID || c.UserID != grant.UserID || c.KeyThumbprint != grant.DeviceKey ||
		s.activeInventoryKeyLocked(workspaceID, grant.DeviceKey) {
		return InventoryDevice{}, multies.ErrDenied
	}
	digest := sha256.Sum256(c.SigningBytes())
	if !ecdsa.VerifyASN1(public, digest[:], signature) {
		return InventoryDevice{}, multies.ErrDenied
	}
	d := InventoryDevice{ID: newID("device"), Label: c.Label, EnrolledAt: now}
	if _, exists := s.data.MultiesInventory[d.ID]; exists {
		return InventoryDevice{}, multies.ErrDenied
	}
	before := s.data.MultiesInventory
	if before == nil {
		s.data.MultiesInventory = make(map[string]inventoryRecord)
	}
	s.data.MultiesInventory[d.ID] = inventoryRecord{Device: d, WorkspaceID: grant.WorkspaceID, KeyThumbprint: grant.DeviceKey, EnrolledBy: grant.UserID}
	if err := s.commitLocked(func() {
		delete(s.data.MultiesInventory, d.ID)
		s.data.MultiesInventory = before
	}); err != nil {
		return InventoryDevice{}, errInventoryWrite
	}
	delete(s.inventoryPending, consentID)
	return d, nil
}

// Remove disables this inventory registration and invalidates pending consent
// for its workspace/key. There are no mesh tokens or device sessions to revoke
// in this slice. The caller must require user confirmation before invoking it.
func (i *MultiesInventory) Remove(ctx context.Context, workspaceID, credential, id string) error {
	if i == nil || i.store == nil || i.workspaceID != workspaceID {
		return multies.ErrDenied
	}
	s := i.store
	s.mu.Lock()
	record, ok := s.data.MultiesInventory[id]
	s.mu.Unlock()
	if !ok || record.WorkspaceID != workspaceID || record.RemovedAt != nil {
		return multies.ErrDenied
	}
	grant, err := i.authorize(ctx, workspaceID, credential, record.KeyThumbprint, multies.Remove)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := i.now()
	if ctx.Err() != nil || !now.Before(grant.ExpiresAt) || now.Before(record.Device.EnrolledAt) || s.data.MultiesInventory[id] != record {
		return multies.ErrDenied
	}
	updated := record
	updated.RemovedAt, updated.RemovedBy = &now, grant.UserID
	s.data.MultiesInventory[id] = updated
	if err := s.commitLocked(func() { s.data.MultiesInventory[id] = record }); err != nil {
		return errInventoryWrite
	}
	for cid, c := range s.inventoryPending {
		if c.WorkspaceID == grant.WorkspaceID && c.KeyThumbprint == grant.DeviceKey {
			delete(s.inventoryPending, cid)
		}
	}
	return nil
}

func (s *Store) validateInventory() error {
	active := make(map[[2]string]bool)
	for id, record := range s.data.MultiesInventory {
		key, err := base64.RawURLEncoding.Strict().DecodeString(record.KeyThumbprint)
		if record.Device.ID != id || !strings.HasPrefix(id, "device-") || !inventoryWorkspaceID.MatchString(id) ||
			!inventoryWorkspaceID.MatchString(record.WorkspaceID) || !inventoryWorkspaceID.MatchString(record.EnrolledBy) ||
			!validInventoryLabel(record.Device.Label) || record.Device.EnrolledAt.IsZero() || err != nil || len(key) != sha256.Size ||
			base64.RawURLEncoding.EncodeToString(key) != record.KeyThumbprint {
			return fmt.Errorf("store: invalid inventory record; file was not changed")
		}
		if record.RemovedAt != nil {
			if record.RemovedAt.Before(record.Device.EnrolledAt) || !inventoryWorkspaceID.MatchString(record.RemovedBy) {
				return fmt.Errorf("store: invalid inventory removal; file was not changed")
			}
			continue
		}
		binding := [2]string{record.WorkspaceID, record.KeyThumbprint}
		if record.RemovedBy != "" || active[binding] {
			return fmt.Errorf("store: conflicting inventory record; file was not changed")
		}
		active[binding] = true
	}
	return nil
}
