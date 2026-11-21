//go:build mnet_enrollment_development

package daemon

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	config "fintex.au/mnet/internal/enrollmentconfig"
	pending "fintex.au/mnet/internal/enrollmentpending"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"fintex.au/mnet/internal/stateowner"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// DevelopmentRecoveryInspection is historical local data, not a live map or
// recovered authority. Inspection never contacts a service or starts a device.
type DevelopmentRecoveryInspection struct {
	Phase              string `json:"phase"`
	Recovery           string `json:"recovery"`
	JoiningSaved       bool   `json:"joiningSaved"`
	NetworkVerified    bool   `json:"networkVerified"`
	OriginalLeaseEnded bool   `json:"originalLeaseEnded"`
}

func InspectDisposableEnrollment(root string) (DevelopmentRecoveryInspection, error) {
	empty := DevelopmentRecoveryInspection{}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return empty, fmt.Errorf("explicit original disposable root required")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return empty, errEnrollmentContextStale
	}
	rel, err := filepath.Rel(os.TempDir(), root)
	if err != nil || rel == "." || rel == ".." || len(rel) > 3 && rel[:3] == "../" {
		return empty, errEnrollmentContextStale
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 || !developmentOwnedByEffectiveUser(info) {
		return empty, errEnrollmentContextStale
	}
	marker, err := readDisposableEnrollmentFile(filepath.Join(root, "DISPOSABLE-MNET-SETUP"), 128)
	if err != nil || string(marker) != "mnet-disposable-enrollment-v1\n" {
		return empty, errEnrollmentContextStale
	}
	policyRaw, err := readDisposableEnrollmentFile(filepath.Join(root, "enrollment-policy-v2.json"), config.MaxBytes)
	if err != nil {
		return empty, err
	}
	policy, err := config.ParseLocalV2(policyRaw)
	if err != nil {
		return empty, err
	}
	snapshot, ok := policy.Snapshot()
	if !ok {
		return empty, errEnrollmentContextStale
	}
	// Same local-only source/origin boundary as the original development caller.
	if !developmentInspectionOrigin(snapshot.SourceOrigin) || !developmentInspectionOrigin(snapshot.RegistrarOrigin) {
		return empty, errEnrollmentContextStale
	}
	limitsRaw, err := readDisposableEnrollmentFile(filepath.Join(root, "workflow-limits.json"), 4096)
	if err != nil {
		return empty, err
	}
	fields, ok := developmentExactObject(limitsRaw, []string{"Timeout", "MaxChallengeLifetime", "MaxReserveLifetime", "Timing"})
	if !ok {
		return empty, errEnrollmentContextStale
	}
	if _, ok := developmentExactObject(fields["Timing"], []string{"MaxObservationAgeMS", "MaxLeaseMS", "ClockUncertaintyMS"}); !ok {
		return empty, errEnrollmentContextStale
	}
	var limits registrar.WorkflowLimits
	if json.Unmarshal(limitsRaw, &limits) != nil || !registrar.ValidWorkflowLimits(limits) || limits.Timeout > 3*time.Second {
		return empty, errEnrollmentContextStale
	}
	dir := filepath.Join(root, "native")
	owner, err := stateowner.AcquireExistingDevelopment(filepath.Join(dir, "daemon"))
	if err != nil {
		return empty, err
	}
	defer owner.Close()
	journalOwner, err := stateowner.AcquireExistingDevelopment(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"))
	if err != nil {
		return empty, err
	}
	defer journalOwner.Close()
	raw, err := readDisposableEnrollmentFile(filepath.Join(dir, "state.json"), 1<<20)
	if err != nil {
		return empty, err
	}
	if !validStateJSON(raw) {
		return empty, errEnrollmentContextStale
	}
	var state State
	if unmarshalOwnedState(raw, &state) != nil || state.validateIdentity() != nil || state.PublicKey == "" || state.NodeToken != "" || state.AuthKey != "" {
		return empty, errEnrollmentContextStale
	}
	nativePolicy, err := readDisposableEnrollmentFile(filepath.Join(dir, "enrollment-policy-v2.json"), config.MaxBytes)
	if err != nil || !bytes.Equal(nativePolicy, policyRaw) {
		return empty, errEnrollmentContextStale
	}
	key, err := base64.StdEncoding.Strict().DecodeString(state.PublicKey)
	if err != nil || len(key) != 32 {
		return empty, errEnrollmentContextStale
	}
	pendingRaw, err := readDisposableEnrollmentFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"), 65536)
	if err != nil {
		return empty, err
	}
	cfg, ok := pending.SourceConfigFor(policy)
	if !ok {
		return empty, errEnrollmentContextStale
	}
	status, err := pending.InspectDevelopmentDATA(pendingRaw, base64.RawURLEncoding.EncodeToString(key), cfg)
	if err != nil {
		return empty, err
	}
	if status.Recovery == "prepared-original-state" {
		stateRevision := base64.RawURLEncoding.EncodeToString(sha256Sum(raw))
		if state.native != nil {
			stateRevision = state.native.OriginalStateRevision
		}
		if !pending.CheckPreparedRecoveryStateDATA(pendingRaw, stateRevision, base64.RawURLEncoding.EncodeToString(sha256Sum(nativePolicy)), base64.RawURLEncoding.EncodeToString(key), cfg) {
			return empty, errEnrollmentContextStale
		}
	}
	out := DevelopmentRecoveryInspection{Phase: status.Phase, Recovery: status.Recovery}
	if state.native != nil {
		if state.native.PolicyRevision != base64.RawURLEncoding.EncodeToString(sha256Sum(nativePolicy)) {
			return empty, errEnrollmentContextStale
		}
		out.JoiningSaved = true
		out.Recovery = "none-saved"
		out.OriginalLeaseEnded = time.Now().UnixMilli() >= state.native.Document.Receipt.AccessExpiresAtMS
	}
	if owner.Check() != nil || journalOwner.Check() != nil {
		return empty, errEnrollmentContextStale
	}
	return out, nil
}
func sha256Sum(raw []byte) []byte { h := sha256.Sum256(raw); return h[:] }

func developmentInspectionOrigin(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() == "127.0.0.1" && u.Port() != ""
}
