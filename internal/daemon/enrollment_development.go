//go:build mnet_enrollment_development

package daemon

// This adapter exists only in an explicitly tagged disposable development build.
// It is not registered in the normal CLI, local API, startup or service.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"fintex.au/mnet/internal/controlclient"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	source "fintex.au/mnet/internal/enrollmentsource"
)

type DevelopmentEnrollmentEvent struct {
	Result     *DevelopmentEnrollmentResult `json:"result,omitempty"`
	Stage      string                       `json:"stage"`
	BrowserURL string                       `json:"browser_url,omitempty"`
	Launch     *contract.SourceLaunch       `json:"launch,omitempty"`
	Peers      int                          `json:"peers,omitempty"`
}

type DevelopmentEnrollmentResult struct {
	Receipt                   contract.WorkflowReceiptWire
	NativeOwnedKeyConfirmed   bool
	NativeProofStateUnchanged bool
	NativeStateFinalized      bool
	NativeCurrentPeers        int
}

// RunDisposableEnrollment creates only root/native State and its owned journal.
// root must be a preexisting private directory containing an explicit marker,
// policy and workflow limits. Both services must be literal HTTPS127.0.0.1.
// No tunnel, endpoint publication, relay, screen or input access is started.
// An interrupted/failed attempt is retained. No retry/reset or key replacement.
func RunDisposableEnrollment(ctx context.Context, root string, emit func(DevelopmentEnrollmentEvent) error) error {
	fail := errors.New("disposable setup unavailable; original local attempt is retained")
	if ctx == nil || ctx.Err() != nil || emit == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fail
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fail
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return fail
	}
	temp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return fail
	}
	rel, err := filepath.Rel(temp, root)
	if err != nil || rel == "." || rel == ".." || len(rel) >= 3 && rel[:3] == "../" {
		return fail
	}

	marker, err := readOwnedEnrollmentFile(filepath.Join(root, "DISPOSABLE-MNET-SETUP"), 128)
	if err != nil || string(marker) != "mnet-disposable-enrollment-v1\n" {
		return fail
	}
	raw, err := readOwnedEnrollmentFile(filepath.Join(root, "enrollment-policy-v2.json"), config.MaxBytes)
	if err != nil {
		return fail
	}
	policy, err := config.ParseLocalV2(raw)
	if err != nil {
		return fail
	}
	snapshot, ok := policy.Snapshot()
	if !ok {
		return fail
	}
	for _, origin := range []string{snapshot.SourceOrigin, snapshot.RegistrarOrigin} {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
			return fail
		}
	}
	rawLimits, err := readOwnedEnrollmentFile(filepath.Join(root, "workflow-limits.json"), 4096)
	if err != nil {
		return fail
	}
	fields, ok := developmentExactObject(rawLimits, []string{"Timeout", "MaxChallengeLifetime", "MaxReserveLifetime", "Timing"})
	if !ok {
		return fail
	}
	if _, ok := developmentExactObject(fields["Timing"], []string{"MaxObservationAgeMS", "MaxLeaseMS", "ClockUncertaintyMS"}); !ok {
		return fail
	}
	var limits registrar.WorkflowLimits
	if json.Unmarshal(rawLimits, &limits) != nil || !registrar.ValidWorkflowLimits(limits) || limits.Timeout > 3*time.Second {
		return fail
	}
	stateDir := filepath.Join(root, "native")
	// Never attach this entry to an existing ordinary daemon State directory.
	if err := os.Mkdir(stateDir, 0700); err != nil {
		return fail
	}
	if err := os.WriteFile(filepath.Join(stateDir, "enrollment-policy-v2.json"), raw, 0600); err != nil {
		return fail
	}
	d, err := New(Config{StateDir: stateDir})
	if err != nil {
		return fail
	}
	defer d.Close()
	if _, err := d.PrepareIdentity(); err != nil {
		return fail
	}
	initial, err := os.ReadFile(d.stateOwner.path)
	if err != nil {
		return fail
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	launch, err := d.prepareNativeEnrollment(time.Now().Add(2 * time.Minute))
	if err != nil {
		return fail
	}
	target, err := url.Parse(snapshot.SourceOrigin)
	if err != nil {
		return fail
	}
	target.Path = "/mnet/enroll-v1"
	target.RawQuery = url.Values{"version": {"1"}, "purpose": {"mnet-enrollment"}, "attemptId": {launch.AttemptID}, "devicePublicKey": {launch.DevicePublicKey}, "credentialDigest": {launch.CredentialDigest}, "challenge": {launch.Challenge}, "state": {launch.State}, "platform": {launch.Platform}, "redirectUri": {launch.RedirectURI}}.Encode()
	if err := emit(DevelopmentEnrollmentEvent{Stage: "waiting-for-browser", BrowserURL: target.String(), Launch: &launch}); err != nil {
		return err
	}
	timer := time.NewTicker(25 * time.Millisecond)
	defer timer.Stop()
	for {
		phase, err := d.enrollmentPreparation.journal.Status()
		if err != nil {
			return fail
		}
		if phase == "declined-data" {
			emit(DevelopmentEnrollmentEvent{Stage: "declined"})
			return fail
		}
		if phase == "callback-recorded" {
			break
		}
		select {
		case <-bounded.Done():
			return fail
		case <-timer.C:
		}
	}
	if err := emit(DevelopmentEnrollmentEvent{Stage: "checking-device-proof"}); err != nil {
		return err
	}
	if _, err := d.exchangeNativeEnrollment(bounded, source.Limits{Timeout: limits.Timeout, MaxConnections: 1}); err != nil {
		return fail
	}
	proofResult, err := d.proveNativeEnrollment(bounded, limits)
	if err != nil {
		return fail
	}
	unchanged, err := os.ReadFile(d.stateOwner.path)
	if err != nil || !bytes.Equal(initial, unchanged) {
		return fail
	}
	if err := d.finalizeNativeEnrollment(bounded); err != nil {
		return fail
	}
	if err := d.reconcileNativeFinalization(); err != nil {
		return fail
	}
	current, err := d.nativeCurrentMap(bounded, controlclient.RegistrarLimits{Timeout: limits.Timeout, MaxConcurrent: 1, MaxRequests: 2})
	if err != nil || !current.Self.Approved || d.dev != nil || d.cc != nil {
		return fail
	}
	return emit(DevelopmentEnrollmentEvent{Stage: "joined-not-connected", Peers: len(current.Peers), Result: &DevelopmentEnrollmentResult{Receipt: proofResult, NativeOwnedKeyConfirmed: true, NativeProofStateUnchanged: true, NativeStateFinalized: true, NativeCurrentPeers: len(current.Peers)}})
}

// A local configuration typo must fail before creating identity or a callback.
// encoding/json alone accepts duplicate and case-aliased field names.
func developmentExactObject(raw []byte, names []string) (map[string]json.RawMessage, bool) {
	allowed := map[string]bool{}
	for _, name := range names {
		allowed[name] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	result := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name, ok := key.(string)
		if !ok || !allowed[name] || result[name] != nil {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		result[name] = value
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || len(result) != len(names) {
		return nil, false
	}
	return result, true
}
