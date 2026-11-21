//go:build mnet_enrollment_development

package daemon

import (
	"context"
	"crypto/sha256"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	"os"
	"path/filepath"
	"time"
)

// DevelopmentRecoveryResult never exposes receipt blobs, keys or authority.
// HistoricalRecovered does not mean joining was installed or network verified.
type DevelopmentRecoveryResult struct {
	Inspection          DevelopmentRecoveryInspection `json:"inspection"`
	HistoricalRecovered bool                          `json:"historicalRecovered"`
	JoiningFinalized    bool                          `json:"joiningFinalized"`
	NetworkVerified     bool                          `json:"networkVerified"`
}

func RecoverDisposableEnrollment(ctx context.Context, root string) (DevelopmentRecoveryResult, error) {
	return recoverDisposableEnrollment(ctx, root, nil)
}
func recoverDisposableEnrollment(ctx context.Context, root string, historyOut *contract.HistoryReceiptWire) (DevelopmentRecoveryResult, error) {
	var out DevelopmentRecoveryResult
	var binding developmentInspectionBinding
	inspected, err := inspectDisposableEnrollment(root, &binding)
	if err != nil || ctx == nil || ctx.Err() != nil {
		return out, errEnrollmentContextStale
	}
	out.Inspection = inspected
	if inspected.JoiningSaved {
		return out, nil
	}
	if inspected.Recovery != "authenticated-history" && inspected.Recovery != "prepared-original-state" {
		return out, pending.ErrPending
	}
	dir := filepath.Join(root, "native")
	// Bind the new recovery owner to the exact inspected State/policy/journal.
	check := func(d *Daemon) error {
		if d.stateOwner.saved.revision != binding.state {
			return errEnrollmentContextStale
		}
		p, e := readDisposableEnrollmentFile(filepath.Join(dir, "enrollment-policy-v2.json"), 65536)
		if e != nil || sha256.Sum256(p) != binding.policy {
			return errEnrollmentContextStale
		}
		j, e := readDisposableEnrollmentFile(filepath.Join(dir, "enrollment-pending", "enrollment-pending-v1.json"), 65536)
		if e != nil || sha256.Sum256(j) != binding.journal {
			return errEnrollmentContextStale
		}
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if inspected.Recovery == "prepared-original-state" {
		owner, err := openNativeFinalizationRecovery(dir)
		if err != nil {
			return out, err
		}
		defer owner.Close()
		if err := check(owner.daemon); err != nil {
			return out, err
		}
		// Existing finite SAME-attempt client; no Submit or source exchange.
		if err := owner.resume(bounded, 3*time.Second, 2); err != nil {
			return out, err
		}
		out.HistoricalRecovered = true
		out.JoiningFinalized = true
		owner.Close()
		current, err := InspectDisposableEnrollment(root)
		if err != nil {
			return out, err
		}
		out.Inspection = current
		return out, nil
	}
	d, err := New(Config{StateDir: dir})
	if err != nil {
		return out, err
	}
	defer d.Close()
	if err := check(d); err != nil {
		return out, err
	}
	before, err := os.ReadFile(d.stateOwner.path)
	if err != nil {
		return out, err
	}
	history, err := d.recoverNativeHistoryDATA(bounded, 3*time.Second, 2)
	if err != nil {
		return out, err
	}
	after, err := os.ReadFile(d.stateOwner.path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		return out, errEnrollmentContextStale
	}
	if historyOut != nil {
		*historyOut = history
	}
	out.HistoricalRecovered = true
	out.Inspection.OriginalLeaseEnded = time.Now().UnixMilli() >= history.Receipt.AccessExpiresAtMS
	out.Inspection.Phase = "history-receipt-data"
	return out, nil
}
