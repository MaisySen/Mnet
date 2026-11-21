package daemon

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"

	config "fintex.au/mnet/internal/enrollmentconfig"
	"fintex.au/mnet/internal/stateowner"
)

var errEnrollmentPolicyOff = errors.New("native enrollment policy is not installed; enrollment is off")
var errEnrollmentContextStale = errors.New("native enrollment policy or identity context unavailable or changed")

// nativeEnrollmentContext is private owned snapshot evidence, NOT an enrollment
// permit. It has no credential/private-key getter. Every future consuming effect
// must revalidate under d.mu; neither this pointer nor its policy syntax suffices.
// There is deliberately no production caller, installer, reload loop or startup
// read. A missing policy never causes a key or replacement policy to be created.
type nativeEnrollmentContext struct {
	owner          *nativeStateOwner
	policy         *config.LocalPolicyV2
	policyRevision [32]byte
	stateRevision  [32]byte
	publicKey      string
}

func (nativeEnrollmentContext) String() string   { return "[private native enrollment context]" }
func (nativeEnrollmentContext) GoString() string { return "[private native enrollment context]" }

// enrollmentContextLocked and checkEnrollmentContextLocked require d.mu.
func (d *Daemon) enrollmentContextLocked() (*nativeEnrollmentContext, error) {
	if err := d.stateHealthLocked(); err != nil {
		return nil, err
	}
	if err := d.checkEnrollmentStateLocked(); err != nil {
		return nil, err
	}
	raw, err := readOwnedEnrollmentFile(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-policy-v2.json"), config.MaxBytes)
	if os.IsNotExist(err) {
		return nil, errEnrollmentPolicyOff
	}
	if err != nil {
		return nil, errEnrollmentContextStale
	}
	policy, err := config.ParseLocalV2(raw)
	if err != nil {
		return nil, errEnrollmentContextStale
	}
	if err := d.stateHealthLocked(); err != nil {
		return nil, err
	}
	if err := d.checkEnrollmentStateLocked(); err != nil {
		return nil, err
	}
	return &nativeEnrollmentContext{owner: d.stateOwner, policy: policy, policyRevision: sha256.Sum256(raw), stateRevision: d.stateOwner.saved.revision, publicKey: d.stateOwner.saved.PublicKey}, nil
}
func (d *Daemon) checkEnrollmentContextLocked(original *nativeEnrollmentContext) error {
	if original == nil || original.owner != d.stateOwner || original.policy == nil {
		return errEnrollmentContextStale
	}
	current, err := d.enrollmentContextLocked()
	if err != nil {
		return err
	}
	if current.policyRevision != original.policyRevision || current.stateRevision != original.stateRevision || current.publicKey != original.publicKey {
		return errEnrollmentContextStale
	}
	// Even package-internal mutation of the policy pointer cannot replace the
	// byte-revision-bound original snapshot with another syntactically valid one.
	expected, _ := current.policy.Snapshot()
	observed, ok := original.policy.Snapshot()
	if !ok || !reflect.DeepEqual(expected, observed) {
		return errEnrollmentContextStale
	}
	return nil
}
func (d *Daemon) checkEnrollmentStateLocked() error {
	o := d.stateOwner
	if d.state == nil || !o.saved.revisionExists || o.saved.PrivateKey == "" || o.saved.PublicKey == "" || o.saved.validateIdentity() != nil || !reflect.DeepEqual(*d.state, o.saved) {
		return errEnrollmentContextStale
	}
	raw, err := readOwnedEnrollmentFile(o.path, 1<<20)
	if err != nil || sha256.Sum256(raw) != o.saved.revision {
		return errEnrollmentContextStale
	}
	return nil
}

// Check path, descriptor and named file agree before AND after bounded read.
// This is defense against accidental replacement and noncooperating writers,
// not a claim of isolation against an adversarial same-UID filesystem writer.
// Any eventual installer must coordinate through the retained native owner.
func readOwnedEnrollmentFile(path string, max int) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > int64(max) || stateowner.SingleLink(path, before) != nil {
		return nil, errEnrollmentContextStale
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errEnrollmentContextStale
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil || len(raw) > max {
		return nil, errEnrollmentContextStale
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) || after.Size() != int64(len(raw)) || stateowner.SingleLink(path, after) != nil {
		return nil, errEnrollmentContextStale
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return raw, nil
}
