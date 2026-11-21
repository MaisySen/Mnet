package daemon

import (
	"encoding/base64"
	"path/filepath"
	"runtime"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
)

// Private and inactive. No production entrypoint or browser/source action.
// Context and journal/socket belong to this daemon's retained native owner.
// Pending DATA never authorizes node credential installation or a tunnel.
type nativeEnrollmentPreparation struct {
	historical bool
	confirmed  *contract.WorkflowReceiptWire
	context    *nativeEnrollmentContext
	journal    *pending.Journal
	callback   *pending.Callback
	launch     contract.SourceLaunch
}

func (nativeEnrollmentPreparation) String() string   { return "[private native pending preparation]" }
func (nativeEnrollmentPreparation) GoString() string { return "[private native pending preparation]" }

// prepareNativeEnrollment owns identity/current policy, saved intent and socket
// together. It does NOT implement credential finalization across two files.
// The deadline is explicit. Actual time.Now is used, not request-supplied time.
func (d *Daemon) prepareNativeEnrollment(expires time.Time) (contract.SourceLaunch, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.enrollmentPreparation != nil {
		return contract.SourceLaunch{}, pending.ErrPending
	}
	original, err := d.enrollmentContextLocked()
	if err != nil {
		return contract.SourceLaunch{}, err
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	if platform != "linux" && platform != "windows" && platform != "macos" {
		return contract.SourceLaunch{}, pending.ErrInvalid
	}
	// Native State uses standard base64 while source launch uses canonical raw
	// base64url over the SAME already-owned public key bytes. Never derive from a
	// response or rotate/recreate a missing key here.
	key, err := base64.StdEncoding.Strict().DecodeString(original.publicKey)
	if err != nil || len(key) != 32 {
		return contract.SourceLaunch{}, errEnrollmentContextStale
	}
	j, err := pending.Open(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending"))
	if err != nil {
		return contract.SourceLaunch{}, err
	}
	good := false
	defer func() {
		if !good {
			j.Close()
		}
	}()
	preparation := &nativeEnrollmentPreparation{context: original, journal: j}
	check := func() error {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.enrollmentPreparation != preparation {
			return errEnrollmentContextStale
		}
		return d.checkEnrollmentContextLocked(original)
	}
	callback, launch, err := j.PrepareCheckedCallback(base64.RawURLEncoding.EncodeToString(key), platform, original.policy, time.Now, expires, check)
	if err != nil {
		return contract.SourceLaunch{}, err
	}
	preparation.callback = callback
	preparation.launch = launch
	if err := d.checkEnrollmentContextLocked(original); err != nil {
		return contract.SourceLaunch{}, err
	}
	// Install only the private in-memory lifecycle handle. State/key/config are
	// unchanged. Saved uncertainty is never erased when preparation fails.
	d.enrollmentPreparation = preparation
	good = true
	return launch, nil
}
