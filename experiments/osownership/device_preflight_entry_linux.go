//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"reflect"
	"time"
)

// Parser dependencies default to inert refusal. Unit/mutation code never owns
// concrete namespace/launcher/device callbacks. Live binding is separate-tagged.
type devicePreflightEntryOps struct {
	uid       func() int
	namespace func() (Namespace, error)
	supervise func(context.Context) (Verdict, error)
	resident  func(context.Context) error
	output    func(any) error
}

func inertDevicePreflightEntryOps() devicePreflightEntryOps {
	return devicePreflightEntryOps{
		uid: func() int { return 1000 }, namespace: func() (Namespace, error) { return Namespace{}, ErrExecutorUnreleased },
		supervise: func(context.Context) (Verdict, error) { return Verdict{}, ErrExecutorUnreleased },
		resident:  func(context.Context) error { return ErrExecutorUnreleased }, output: func(any) error { return ErrExecutorUnreleased },
	}
}
func DevicePreflightMain(args []string) error {
	return devicePreflightDispatch(args, inertDevicePreflightEntryOps())
}

// Separate binary and exact modes. Existing BootstrapEvidenceMain unchanged.
func devicePreflightDispatch(args []string, ops devicePreflightEntryOps) error {
	if len(args) != 1 {
		return errors.New("one exact device-preflight mode required")
	}
	switch args[0] {
	case "--device-open-preflight":
		if ops.uid() == 0 {
			return errors.New("unprivileged outer required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		outer, e := ops.namespace()
		if e != nil {
			return e
		}
		v, runErr := ops.supervise(ctx)
		if runErr == nil {
			runErr = validateDevicePreflightVerdict(v, outer)
		}
		after, e := ops.namespace()
		runErr = errors.Join(runErr, e)
		unchanged := e == nil && after == outer
		if !unchanged {
			runErr = errors.Join(runErr, ErrConflict)
		}
		outcome := "feasibility-recorded"
		if runErr != nil {
			outcome = "error"
		}
		out := struct {
			Version        int       `json:"version"`
			Outcome        string    `json:"outcome"`
			OuterInitial   Namespace `json:"outer_initial"`
			OuterFinal     Namespace `json:"outer_final"`
			OuterUnchanged bool      `json:"outer_unchanged"`
			Verdict        Verdict   `json:"verdict"`
		}{1, outcome, outer, after, unchanged, v}
		return errors.Join(runErr, ops.output(out))
	case "--osownership-resident-device-open-preflight":
		return ops.resident(context.Background())
	default:
		return errors.New("unknown device-preflight mode")
	}
}
func runDevicePreflightVerdict(ctx context.Context, ns *namespaceCustody) (v Verdict, err error) {
	if ns == nil {
		return v, ErrConflict
	}
	r, err := runDevicePreflightSource(ctx, ns)
	v = Verdict{Version: 1, Sequence: 1, Namespace: ns.identity, Stage: "device-open-preflight", Result: "fail", FailureClass: "runner", DevicePreflight: &r, BaselineHash: r.BaselineHash, FinalHash: r.FinalHash}
	if err != nil {
		return v, err
	}
	if err = validateDevicePreflightResult(r, ns.identity); err != nil {
		return
	}
	v.Stage = "final"
	v.Result = "pass"
	v.FailureClass = ""
	v.CleanupConfirmed = true
	return
}
func validateDevicePreflightResult(r DevicePreflightEvidence, ns Namespace) error {
	if r.Version != 1 || r.Path != preflightDevicePath || r.Namespace != ns || !ns.Isolated || !r.NamespaceRechecked || !r.FDInventoryUnchanged || !reflect.DeepEqual(r.FDsBefore, r.FDsAfter) || !r.BaselineUnchanged || len(r.BaselineHash) != 64 || r.BaselineHash != r.FinalHash {
		return ErrConflict
	}
	switch r.Outcome {
	case "opened-closed":
		if !r.OpenAttempted || r.FD < 0 || !r.CloseAttempted || !r.Closed || !r.CLOEXEC || !r.Nonblock || r.Metadata == nil || r.OpenedMetadata == nil || *r.Metadata != *r.OpenedMetadata || !isTUNDevice(*r.Metadata) {
			return ErrConflict
		}
	case "missing", "denied":
		if r.FD != -1 || r.CloseAttempted || r.Closed || (r.Outcome == "missing" && r.Errno != int(unix.ENOENT) || r.Outcome == "denied" && r.Errno != int(unix.EPERM) && r.Errno != int(unix.EACCES)) {
			return ErrConflict
		}
	case "refused-object":
		if r.OpenAttempted || r.FD != -1 || r.CloseAttempted || r.Metadata == nil || isTUNDevice(*r.Metadata) {
			return ErrConflict
		}
	default:
		return ErrConflict
	}
	return nil
}
func validateDevicePreflightVerdict(v Verdict, outer Namespace) error {
	if v.DevicePreflight == nil || v.Bootstrap != nil || v.Failure != nil || v.Result != "pass" || v.Stage != "final" || !v.CleanupConfirmed || !v.Namespace.Isolated || outer.Device == 0 || outer.Inode == 0 || (v.Namespace.Device == outer.Device && v.Namespace.Inode == outer.Inode) || v.Lifecycle == nil || !v.Lifecycle.Reaped || !v.Lifecycle.CustodyClosed || v.Lifecycle.CustodyRetained {
		return ErrConflict
	}
	if v.BaselineHash != v.DevicePreflight.BaselineHash || v.FinalHash != v.DevicePreflight.FinalHash {
		return ErrConflict
	}
	return validateDevicePreflightResult(*v.DevicePreflight, v.Namespace)
}
