package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fintex.au/mnet/internal/controlclient"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	"fintex.au/mnet/internal/netmap"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

// Private/inactive sole-State publication. Requires SAME actual native proof
// confirmation retained in preparation, never caller-supplied receipt DATA.
func (d *Daemon) finalizeNativeEnrollment(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	start := time.Now()
	p := d.enrollmentPreparation
	if ctx == nil || ctx.Err() != nil || p == nil || (!p.historical && p.confirmed == nil) || d.checkEnrollmentContextLocked(p.context) != nil || !d.enrollmentIdentityUnclaimedLocked() {
		return errEnrollmentContextStale
	}
	cfg, ok := pending.SourceConfigFor(p.context.policy)
	if !ok {
		return errEnrollmentContextStale
	}
	original := base64.RawURLEncoding.EncodeToString(p.context.stateRevision[:])
	policy := base64.RawURLEncoding.EncodeToString(p.context.policyRevision[:])
	publish := func(doc contract.NativeCredentialDocument) error {
		if ctx.Err() != nil || d.checkEnrollmentContextLocked(p.context) != nil || !d.enrollmentIdentityUnclaimedLocked() || time.Now().UnixMilli() >= doc.Receipt.AccessExpiresAtMS {
			return errEnrollmentContextStale
		}
		candidate := cloneState(d.stateOwner.saved)
		// Remove only unused legacy target metadata. Never reinterpret legacy tokens.
		candidate.Server = ""
		candidate.Network = ""
		candidate.native = &nativeSavedCredential{Document: doc, OriginalStateRevision: original, PolicyRevision: policy}
		// Recheck original context immediately before publication and after file sync.
		live := func() bool {
			wall := time.Now()
			elapsed := start.Add(time.Since(start))
			return ctx.Err() == nil && wall.UnixMilli() >= start.UnixMilli() && wall.UnixMilli() < doc.Receipt.AccessExpiresAtMS && elapsed.UnixMilli() < doc.Receipt.AccessExpiresAtMS
		}
		before := func() error {
			if !live() {
				return errEnrollmentContextStale
			}
			return d.checkEnrollmentContextLocked(p.context)
		}
		if err := before(); err != nil {
			return err
		}
		after := func() error {
			if !live() {
				return errEnrollmentContextStale
			}
			raw, err := readOwnedEnrollmentFile(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-policy-v2.json"), 65536)
			if err != nil || sha256.Sum256(raw) != p.context.policyRevision {
				return errEnrollmentContextStale
			}
			return nil
		}
		if err := d.stateOwner.saveChecked(&candidate, before, after); err != nil {
			if d.stateOwner.fault {
				d.freezeStateLocked()
			}
			return err
		}
		*d.state = cloneState(candidate)
		// State is sole usable image, but still needs completed pending correlation.
		return d.checkInstalledNativeLocked()
	}
	var err error
	if p.historical {
		err = p.journal.FinalizeHistory(original, policy, cfg, publish)
	} else {
		err = p.journal.FinalizeWorkflow(*p.confirmed, original, policy, cfg, publish)
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil || d.checkInstalledNativeLocked() != nil || time.Now().UnixMilli() >= d.stateOwner.saved.native.Document.Receipt.AccessExpiresAtMS {
		return errEnrollmentContextStale
	}
	return nil
}

// Native State is not silently activated by New/Up/Prepare. Only private actual
// current operations call this and correlate SAME retained pending history.
func (d *Daemon) checkInstalledNativeLocked() error {
	if d.stateHealthLocked() != nil || d.stateOwner.saved.native == nil || !reflect.DeepEqual(*d.state, d.stateOwner.saved) {
		return errEnrollmentContextStale
	}
	raw, err := readOwnedEnrollmentFile(d.stateOwner.path, 1<<20)
	if err != nil || sha256.Sum256(raw) != d.stateOwner.saved.revision {
		return errEnrollmentContextStale
	}
	n := d.stateOwner.saved.native
	policyRaw, err := readOwnedEnrollmentFile(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-policy-v2.json"), 65536)
	if err != nil {
		return errEnrollmentContextStale
	}
	digest := sha256.Sum256(policyRaw)
	if base64.RawURLEncoding.EncodeToString(digest[:]) != n.PolicyRevision {
		return errEnrollmentContextStale
	}
	p, err := config.ParseLocalV2(policyRaw)
	if err != nil {
		return errEnrollmentContextStale
	}
	b := n.Document.Challenge.Binding
	m := contract.SourceMapping{WorkspaceID: b.WorkspaceID, Issuer: b.IssuerLabel, RegistrarOrigin: b.RegistrarOrigin, Audience: b.Audience, MappingRevision: b.MappingRevision, NetworkID: b.NetworkID}
	if !p.AllowsObservedDATA(m, b.PrincipalAuthority, b.PrincipalNamespace, b.PrincipalID, b.ServiceID) {
		return errEnrollmentContextStale
	}
	return nil
}
func (d *Daemon) withNativeJournalLocked(fn func(*pending.Journal) error) error {
	if d.enrollmentPreparation != nil {
		return fn(d.enrollmentPreparation.journal)
	}
	dir := filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending")
	if _, err := os.Lstat(filepath.Join(dir, "enrollment-pending-v1.json")); err != nil {
		return err
	}
	j, err := pending.Open(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending"))
	if err != nil {
		return err
	}
	defer j.Close()
	return fn(j)
}
func (d *Daemon) reconcileNativeFinalization() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkInstalledNativeLocked(); err != nil {
		return err
	}
	n := d.stateOwner.saved.native
	return d.withNativeJournalLocked(func(j *pending.Journal) error {
		return j.CorrelateFinalization(n.Document, n.OriginalStateRevision, n.PolicyRevision, true)
	})
}

// Each private operation checks sole State + exact completed pending correlation
// and original policy before disclosure and after I/O. No legacy client/rejoin,
// automatic restart, tunnel or production ingress is added.

// Requires d.mu. Client is retained for finite shared request budgets and
// permanent-denial semantics across operations, disposed on Close/state fault.
func (d *Daemon) nativeClientLocked(limits controlclient.RegistrarLimits) (*controlclient.RegistrarClient, [32]byte, error) {
	var zero [32]byte
	if err := d.checkInstalledNativeLocked(); err != nil {
		return nil, zero, err
	}
	n := *d.stateOwner.saved.native
	rev := d.stateOwner.saved.revision
	if err := d.withNativeJournalLocked(func(j *pending.Journal) error {
		return j.CorrelateFinalization(n.Document, n.OriginalStateRevision, n.PolicyRevision, false)
	}); err != nil {
		return nil, zero, err
	}
	if d.nativeClient != nil && d.nativeClientRevision != rev {
		d.nativeClient.Close()
		d.nativeClient = nil
	}
	if d.nativeClient == nil {
		c, err := controlclient.NewRegistrarClient(n.Document.Receipt.RegistrarOrigin, n.Document, limits)
		if err != nil {
			return nil, zero, err
		}
		d.nativeClient = c
		d.nativeClientRevision = rev
	}
	return d.nativeClient, rev, nil
}
func (d *Daemon) checkNativeCurrentAfterLocked(ctx context.Context, rev [32]byte) error {
	if ctx.Err() != nil || d.checkInstalledNativeLocked() != nil || d.stateOwner.saved.revision != rev {
		return errEnrollmentContextStale
	}
	n := d.stateOwner.saved.native
	if time.Now().UnixMilli() < n.Document.Receipt.CommittedAtMS || time.Now().UnixMilli() >= n.Document.Receipt.AccessExpiresAtMS {
		return errEnrollmentContextStale
	}
	return d.withNativeJournalLocked(func(j *pending.Journal) error {
		return j.CorrelateFinalization(n.Document, n.OriginalStateRevision, n.PolicyRevision, false)
	})
}
func (d *Daemon) nativeCurrentMap(ctx context.Context, limits controlclient.RegistrarLimits) (*netmap.Map, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errEnrollmentContextStale
	}
	d.mu.Lock()
	c, rev, err := d.nativeClientLocked(limits)
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	result, err := c.CurrentMap(ctx)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkNativeCurrentAfterLocked(ctx, rev); err != nil {
		return nil, err
	}
	return result, nil
}
func (d *Daemon) nativeCurrentEndpoints(ctx context.Context, endpoints []string, limits controlclient.RegistrarLimits) error {
	if ctx == nil || ctx.Err() != nil {
		return errEnrollmentContextStale
	}
	d.mu.Lock()
	c, rev, err := d.nativeClientLocked(limits)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if err := c.RegistrarEndpoints(ctx, endpoints); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.checkNativeCurrentAfterLocked(ctx, rev)
}

// Private native current-credential publication. This is an explicit bounded
// compatibility declaration, not remote kernel attestation or access renewal.
func (d *Daemon) nativeCurrentEndpointProfile(ctx context.Context, endpoints []string, limits controlclient.RegistrarLimits, capability *netmap.EndpointCapability) error {
	if ctx == nil || ctx.Err() != nil {
		return errEnrollmentContextStale
	}
	d.mu.Lock()
	c, rev, err := d.nativeClientLocked(limits)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if err := c.RegistrarEndpointProfile(ctx, endpoints, capability); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.checkNativeCurrentAfterLocked(ctx, rev)
}
