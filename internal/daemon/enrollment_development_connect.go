//go:build mnet_enrollment_development

package daemon

// Explicit new-setup-only, one-shot LOCAL DEVELOPMENT WORKFLOW limit.
// A receipt contains no quota. This record never supplies network authority.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"fintex.au/mnet/internal/controlclient"
)

const developmentConnectionRecordName = "local-development-connection-v1.json"
const developmentConnectionLimitKind = "local-development-workflow-limit-not-receipt-quota"

type developmentConnectionRecord struct {
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	Phase      string `json:"phase"`
	State      string `json:"state_sha256"`
	Policy     string `json:"policy_sha256"`
	Journal    string `json:"journal_sha256"`
	Limits     string `json:"workflow_limits_sha256"`
	Total      int    `json:"local_total"`
	Setup      int    `json:"local_setup_reserved"`
	Connection int    `json:"local_connection_reserved"`
}
type developmentConnectionOwner struct {
	d      *Daemon
	path   string
	record developmentConnectionRecord
	raw    []byte
	frozen bool
	// Per-owner test seam. No environment/public hook or normal-startup caller.
	fault func(string) error
}

func developmentHash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func developmentConnectionBinding(d *Daemon, root string) (developmentConnectionRecord, error) {
	var r developmentConnectionRecord
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkInstalledNativeLocked(); err != nil {
		return r, err
	}
	// Completed journal is independently correlated by checkNativeCurrentAfterLocked.
	if err := d.checkNativeCurrentAfterLocked(context.Background(), d.stateOwner.saved.revision); err != nil {
		return r, err
	}
	paths := []string{d.stateOwner.path, filepath.Join(root, "native", "enrollment-policy-v2.json"), filepath.Join(root, "native", "enrollment-pending", "enrollment-pending-v1.json"), filepath.Join(root, "workflow-limits.json")}
	hashes := make([]string, len(paths))
	for i, p := range paths {
		raw, e := readDisposableEnrollmentFile(p, 1<<20)
		if e != nil {
			return r, e
		}
		hashes[i] = developmentHash(raw)
	}
	if hashes[0] != hex.EncodeToString(d.stateOwner.saved.revision[:]) {
		return r, errEnrollmentContextStale
	}
	r = developmentConnectionRecord{Version: 1, Kind: developmentConnectionLimitKind, State: hashes[0], Policy: hashes[1], Journal: hashes[2], Limits: hashes[3], Total: 16, Setup: 2}
	return r, nil
}
func newDevelopmentConnectionRecord(d *Daemon, root string) (*developmentConnectionOwner, error) {
	r, e := developmentConnectionBinding(d, root)
	if e != nil {
		return nil, e
	}
	o := &developmentConnectionOwner{d: d, path: filepath.Join(root, "native", developmentConnectionRecordName), record: r}
	if _, e := os.Lstat(o.path); !os.IsNotExist(e) {
		return nil, errEnrollmentContextStale
	}
	o.record.Phase = "setup-reserved"
	if e := o.publish(true); e != nil {
		return nil, e
	}
	return o, nil
}
func openDevelopmentConnectionRecord(d *Daemon, root string) (*developmentConnectionOwner, error) {
	expected, e := developmentConnectionBinding(d, root)
	if e != nil {
		return nil, e
	}
	path := filepath.Join(root, "native", developmentConnectionRecordName)
	raw, e := readDisposableEnrollmentFile(path, 4096)
	if e != nil {
		return nil, e
	}
	var r developmentConnectionRecord
	if json.Unmarshal(raw, &r) != nil {
		return nil, errEnrollmentContextStale
	}
	canonical, e := json.Marshal(r)
	if e != nil || !bytes.Equal(raw, canonical) {
		return nil, errEnrollmentContextStale
	}
	expected.Phase = r.Phase
	expected.Connection = r.Connection
	if r != expected || !developmentConnectionPhaseValid(r) {
		return nil, errEnrollmentContextStale
	}
	return &developmentConnectionOwner{d: d, path: path, record: r, raw: raw}, nil
}
func (o *developmentConnectionOwner) check() error {
	if o.frozen || o.d.stateOwner.lease.Check() != nil {
		o.frozen = true
		return errEnrollmentContextStale
	}
	raw, e := readDisposableEnrollmentFile(o.path, 4096)
	if e != nil || !bytes.Equal(raw, o.raw) {
		o.frozen = true
		return errEnrollmentContextStale
	}
	return nil
}
func (o *developmentConnectionOwner) transition(from, to string) error {
	if !((from == "setup-reserved" && to == "ready") || (from == "ready" && to == "connection-reserved")) {
		return errEnrollmentContextStale
	}
	if o.check() != nil || o.record.Phase != from {
		return errEnrollmentContextStale
	}
	o.record.Phase = to
	if to == "connection-reserved" {
		o.record.Connection = 14
	}
	return o.publish(false)
}

// All failures freeze this owner. No I/O caller may continue after uncertain
// publication. Rename success does not suffice: parent directory is synced too.
func (o *developmentConnectionOwner) publish(initial bool) (err error) {
	defer func() {
		if err != nil {
			o.frozen = true
		}
	}()
	if o.frozen || o.d.stateOwner.lease.Check() != nil {
		return errEnrollmentContextStale
	}
	raw, err := json.Marshal(o.record)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(o.path), ".local-development-limit-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	point := func(stage string) error {
		if o.fault != nil {
			return o.fault(stage)
		}
		return nil
	}
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if err = point("write"); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = point("file-sync"); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = point("before-publish"); err != nil {
		return err
	}
	if o.d.stateOwner.lease.Check() != nil {
		return errEnrollmentContextStale
	}
	if initial { // Exclusive link publishes without replacing even an unexpected existing file.
		err = os.Link(f.Name(), o.path)
	} else {
		if err = o.check(); err != nil {
			return err
		}
		err = os.Rename(f.Name(), o.path)
	}
	if err != nil {
		return err
	}
	// Remove the initial temp hardlink before validating single-link reads.
	if initial {
		if err = os.Remove(f.Name()); err != nil {
			return err
		}
	}
	if err = point("after-publish"); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(o.path))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err = point("directory-sync"); err != nil {
		return err
	}
	if err = dir.Sync(); err != nil {
		return err
	}
	o.raw = raw
	return nil
}

// RunDisposableEnrollmentSaveConnection opts NEW setup into a one-shot later
// connection. Never attach a budget to preexisting roots. Setup reserves2;
// a later explicit process may spend the remaining14 exactly once.
func RunDisposableEnrollmentSaveConnection(ctx context.Context, root string, emit func(DevelopmentEnrollmentEvent) error) error {
	return runDisposableEnrollmentMode(ctx, root, emit, nil, true)
}
func ConnectDisposableEnrollment(ctx context.Context, root string, emit func(DevelopmentEnrollmentEvent) error, options DevelopmentNetworkOptions) error {
	if ctx == nil || ctx.Err() != nil || emit == nil || !validDevelopmentNetworkOptions(&options) || options.Limits.MaxRequests != 14 {
		return errEnrollmentContextStale
	}
	var binding developmentInspectionBinding
	i, e := inspectDisposableEnrollment(root, &binding)
	if e != nil || !i.JoiningSaved || i.BookkeepingPending || i.OriginalLeaseEnded {
		return errEnrollmentContextStale
	}
	// Refuse old roots before opening any potentially creating daemon path.
	if _, e := readDisposableEnrollmentFile(filepath.Join(root, "native", developmentConnectionRecordName), 4096); e != nil {
		return e
	}
	d, e := New(Config{StateDir: filepath.Join(root, "native")})
	if e != nil {
		return e
	}
	defer d.Close()
	if d.stateOwner.saved.revision != binding.state {
		return errEnrollmentContextStale
	}
	o, e := openDevelopmentConnectionRecord(d, root)
	if e != nil {
		return e
	}
	if o.record.Journal != hex.EncodeToString(binding.journal[:]) || o.record.Policy != hex.EncodeToString(binding.policy[:]) {
		return errEnrollmentContextStale
	}
	// Match the original configured timeout, not a new caller allowance.
	raw, e := readDisposableEnrollmentFile(filepath.Join(root, "workflow-limits.json"), 4096)
	if e != nil {
		return e
	}
	var limits struct{ Timeout time.Duration }
	if json.Unmarshal(raw, &limits) != nil || limits.Timeout != options.Limits.Timeout {
		return errEnrollmentContextStale
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e = o.transition("ready", "connection-reserved"); e != nil {
		return e
	}
	// No refund, retry, terminal rewrite or reconnect, including a crash here.
	if e := emit(DevelopmentEnrollmentEvent{Stage: "local-development-connection-slice-spent"}); e != nil {
		return e
	}
	if e = o.check(); e != nil {
		return e
	}
	current, e := d.nativeCurrentMap(ctx, controlclient.RegistrarLimits{Timeout: options.Limits.Timeout, MaxConcurrent: 1, MaxRequests: 14})
	if e != nil {
		return e
	}
	return d.runDevelopmentNetwork(ctx, current, &options)
}

func developmentConnectionPhaseValid(r developmentConnectionRecord) bool {
	return ((r.Phase == "setup-reserved" || r.Phase == "ready") && r.Connection == 0) || (r.Phase == "connection-reserved" && r.Connection == 14)
}
