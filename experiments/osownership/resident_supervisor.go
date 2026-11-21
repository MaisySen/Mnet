//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const maxVerdictBytes = 64 << 10

// Verdict is one JSON record plus EOF, never streaming arbitrary stdout. Public
// evidence excludes credentials. Decoder/runner/cleanup failures are distinct.
type Verdict struct {
	DevicePreflight  *DevicePreflightEvidence   `json:"device_preflight,omitempty"`
	Failure          *FailureEvidence           `json:"failure,omitempty"`
	Lifecycle        *LifecycleEvidence         `json:"lifecycle,omitempty"`
	Bootstrap        *BootstrapReadOnlyEvidence `json:"bootstrap,omitempty"`
	Version          int                        `json:"version"`
	Stage            string                     `json:"stage"`
	Namespace        Namespace                  `json:"namespace"`
	Sequence         int                        `json:"sequence"`
	Result           string                     `json:"result"`
	FailureClass     string                     `json:"failure_class,omitempty"`
	Errno            int                        `json:"errno,omitempty"`
	BaselineHash     string                     `json:"baseline_hash"`
	FinalHash        string                     `json:"final_hash"`
	CleanupConfirmed bool                       `json:"cleanup_confirmed"`
}

func DecodeVerdict(r io.Reader, ns Namespace) (Verdict, error) {
	var v Verdict
	b, e := io.ReadAll(io.LimitReader(r, maxVerdictBytes+1))
	if e != nil || len(b) > maxVerdictBytes {
		return v, errors.New("verdict read/size failure")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&v); e != nil {
		return v, e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return v, errors.New("multiple/trailing verdict")
	}
	if v.Version != 1 || v.Sequence != 1 || !sameNamespace(v.Namespace, ns) {
		return v, ErrConflict
	}
	if v.Result == "fail" {
		switch v.FailureClass {
		case "harness", "decoder", "baseline", "sysctl-applicability", "custody", "cleanup", "runner":
			return v, fmt.Errorf("conformance failure stage=%s class=%s cleanup=%t", v.Stage, v.FailureClass, v.CleanupConfirmed)
		default:
			return v, ErrConflict
		}
	}
	if v.Stage != "final" {
		return v, ErrConflict
	}
	if _, e := hex.DecodeString(v.BaselineHash); e != nil {
		return v, e
	}
	if v.Result != "pass" || v.FailureClass != "" || !v.CleanupConfirmed || len(v.BaselineHash) != 64 || v.BaselineHash != v.FinalHash {
		return v, fmt.Errorf("conformance failed at %s (%s); cleanup=%t", v.Stage, v.FailureClass, v.CleanupConfirmed)
	}
	return v, nil
}

// ResidentProcess owns its transport/custody and reaping. Fake implementations
// prove order only. No interface method claims networking absence from exit.
type ResidentProcess interface {
	Namespace() Namespace
	VerdictReader() io.Reader
	Reap(context.Context) error
	AbortAndReap(context.Context) error
	CloseCustody() error
}
type ResidentSpawner interface {
	Spawn(context.Context) (ResidentProcess, error)
}

// Supervisor uses a bounded child pipe read; timeout requires abort+reap, then
// closes custody. These finite SOURCE bounds are conformance-only, never lease
// grants or authorization to launch. The real launcher stays mechanically off.
func SuperviseResident(ctx context.Context, s ResidentSpawner) (v Verdict, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, e := s.Spawn(ctx)
	if e != nil {
		var pending *PendingResidentError
		if errors.As(e, &pending) {
			life := LifecycleEvidence{CustodyRetained: true}
			if evidence, ok := pending.Process.(interface{ LifecycleSnapshot() LifecycleEvidence }); ok {
				life = evidence.LifecycleSnapshot()
				life.CustodyRetained = true
			}
			v.Lifecycle = &life
		}
		return v, e
	}
	life := &LifecycleEvidence{CustodyRetained: true}
	v.Lifecycle = life
	if evidence, ok := p.(interface{ LifecycleSnapshot() LifecycleEvidence }); ok {
		*life = evidence.LifecycleSnapshot()
		life.CustodyRetained = true
	}
	joined := false
	readerDone := make(chan struct{})
	defer func() {
		v.Lifecycle = life
		defer func() {
			if evidence, ok := p.(interface{ LifecycleSnapshot() LifecycleEvidence }); ok {
				snapshot := evidence.LifecycleSnapshot()
				life.ExitCode = snapshot.ExitCode
				life.Signal = snapshot.Signal
				life.Reaped = snapshot.Reaped
			}
		}()
		if !joined {
			life.AbortAttempted = true
			life.ReapAttempted = true
			cleanup, stop := context.WithTimeout(context.Background(), 500*time.Millisecond)
			reapErr := p.AbortAndReap(cleanup)
			life.Reaped = reapErr == nil
			defer stop()
			if reapErr == nil {
				select {
				case <-readerDone:
				case <-cleanup.Done():
					reapErr = errors.New("verdict reader did not join")
				}
			}
			if reapErr != nil {
				err = errors.Join(err, &PendingResidentError{Process: p, Cause: reapErr})
				return
			}
		}
		life.CustodyCloseAttempted = true
		closeErr := p.CloseCustody()
		life.CustodyClosed = closeErr == nil
		life.CustodyRetained = closeErr != nil
		err = errors.Join(err, closeErr)
	}()
	type result struct {
		v Verdict
		e error
	}
	ch := make(chan result, 1)
	go func() {
		defer close(readerDone)
		vv, ee := DecodeVerdict(p.VerdictReader(), p.Namespace())
		ch <- result{vv, ee}
	}()
	select {
	case <-ctx.Done():
		return v, ctx.Err()
	case r := <-ch:
		v, err = r.v, r.e
	}
	if err != nil {
		return v, err
	}
	select {
	case <-readerDone:
	case <-ctx.Done():
		return v, ctx.Err()
	}
	life.ReapAttempted = true
	if e = p.Reap(ctx); e != nil {
		return v, e
	}
	life.Reaped = true
	joined = true
	return v, nil
}

// Unreaped child keeps namespace custody. Caller must retain this outcome and
// arrange bounded follow-up, never turn timeout into external absence.
type PendingResidentError struct {
	Process ResidentProcess
	Cause   error
}

func (e *PendingResidentError) Error() string {
	return "resident unreaped; namespace custody retained: " + e.Cause.Error()
}
func (e *PendingResidentError) Unwrap() error { return e.Cause }
