//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"syscall"
)

// No arbitrary error strings or stderr text cross into verdicts. Operation is
// chosen by source, errno is typed, exit facts come from owned process Wait.
type FailureEvidence struct {
	Runtime          *RuntimeStderrEvidence `json:"runtime_stderr,omitempty"`
	Phase            string                 `json:"phase"`
	StdoutBytes      int                    `json:"stdout_bytes"`
	StdoutComplete   bool                   `json:"stdout_complete"`
	StdoutJSON       bool                   `json:"stdout_json"`
	Operation        string                 `json:"operation"`
	Class            string                 `json:"class"`
	Errno            int                    `json:"errno,omitempty"`
	ExitCode         int                    `json:"exit_code,omitempty"`
	ExitObserved     bool                   `json:"exit_observed"`
	Signal           int                    `json:"signal,omitempty"`
	PID              int                    `json:"pid,omitempty"`
	StartTicks       uint64                 `json:"start_ticks,omitempty"`
	IdentityObserved bool                   `json:"identity_observed"`
	Reaped           bool                   `json:"reaped"`
	StderrBytes      int                    `json:"stderr_bytes"`
	StderrSHA256     string                 `json:"stderr_sha256,omitempty"`
	StderrComplete   bool                   `json:"stderr_complete"`
	ChildDiagnostic  *ChildDiagnostic       `json:"child_diagnostic,omitempty"`
}
type ChildDiagnostic struct {
	Phase     string `json:"phase,omitempty"`
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Class     string `json:"class"`
	Errno     int    `json:"errno,omitempty"`
}
type LifecycleEvidence struct {
	PID                   int    `json:"pid,omitempty"`
	StartTicks            uint64 `json:"start_ticks,omitempty"`
	IdentityObserved      bool   `json:"identity_observed"`
	ReapAttempted         bool   `json:"reap_attempted"`
	Reaped                bool   `json:"reaped"`
	AbortAttempted        bool   `json:"abort_attempted"`
	CustodyRetained       bool   `json:"custody_retained"`
	CustodyCloseAttempted bool   `json:"custody_close_attempted"`
	CustodyClosed         bool   `json:"custody_closed"`
	ExitCode              int    `json:"exit_code,omitempty"`
	Signal                int    `json:"signal,omitempty"`
}
type operationError struct {
	op, class string
	cause     error
}

func (e *operationError) Error() string {
	return "bounded operation failed: " + e.op + " (" + e.class + ")"
}
func (e *operationError) Unwrap() error { return e.cause }

var allowedDiagnosticOps = map[string]bool{"client-precheck": true, "no-new-privs": true, "ambient-clear": true, "cap-last-read": true, "bounding-drop": true, "securebits-lock": true, "capset-zero": true, "filter-baseline": true, "filter-install": true, "descriptor-cloexec": true, "client-exec": true, "client-fd-inventory": true, "client-status": true, "client-namespace": true, "client-filter-count": true, "owner-signal0": true, "client-output": true, "client-start": true, "client-wait": true, "client-evidence-decode": true, "resident-work": true}

func opFailure(op, class string, e error) error {
	if e == nil {
		return nil
	}
	if !allowedDiagnosticOps[op] {
		op = "resident-work"
	}
	return &operationError{op: op, class: class, cause: e}
}
func DiagnosticForError(e error) ChildDiagnostic {
	d := ChildDiagnostic{Version: 1, Operation: "resident-work", Class: "runner"}
	var op *operationError
	if errors.As(e, &op) {
		d.Operation = op.op
		d.Class = op.class
	}
	var fd *FDInventoryHarnessError
	if errors.As(e, &fd) {
		d.Operation = "client-fd-inventory"
		d.Class = "harness"
	}
	var errno syscall.Errno
	if errors.As(e, &errno) {
		d.Errno = int(errno)
	}
	d.Phase = phaseForOperation(d.Operation)
	if !validChildDiagnostic(d) {
		return ChildDiagnostic{Version: 1, Operation: "resident-work", Class: "runner", Phase: "unknown"}
	}
	return d
}
func validChildDiagnostic(d ChildDiagnostic) bool {
	if d.Phase != "" && d.Phase != phaseForOperation(d.Operation) {
		return false
	}
	if d.Version != 1 || !allowedDiagnosticOps[d.Operation] || d.Errno < 0 || d.Errno > 4095 {
		return false
	}
	switch d.Class {
	case "runner", "harness", "custody", "decoder", "cleanup", "sysctl-applicability", "baseline":
		return true
	}
	return false
}
func summarizedStderr(b []byte, complete bool) (int, string, *ChildDiagnostic) {
	sum := sha256.Sum256(b)
	var d ChildDiagnostic
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	e := dec.Decode(&d)
	var extra any
	if e != nil || dec.Decode(&extra) != io.EOF || !validChildDiagnostic(d) || !complete {
		return len(b), hex.EncodeToString(sum[:]), nil
	}
	return len(b), hex.EncodeToString(sum[:]), &d
}

type childRunFailure struct {
	evidence FailureEvidence
	cause    error
}

func (e *childRunFailure) Error() string { return "child run failed (sanitized evidence retained)" }
func (e *childRunFailure) Unwrap() error { return e.cause }

func phaseForOperation(op string) string {
	switch op {
	case "client-precheck", "no-new-privs", "ambient-clear", "cap-last-read", "bounding-drop", "securebits-lock", "capset-zero", "filter-baseline", "filter-install":
		return "bootstrap-pre-filter"
	case "descriptor-cloexec", "client-exec":
		return "bootstrap-filter-installed"
	case "client-fd-inventory", "client-status", "client-namespace", "client-filter-count", "owner-signal0", "client-output":
		return "final-client"
	}
	return "unknown"
}

type phaseRecord struct {
	Version int    `json:"version"`
	Phase   string `json:"phase"`
}

func allowedPhase(s string) bool {
	switch s {
	case "bootstrap-entry", "capset-zero", "filter-installed", "final-entry", "fd-validated":
		return true
	}
	return false
}
func scanSanitizedStderr(b []byte, complete bool) (*ChildDiagnostic, string) {
	if !complete || len(b) > 8192 {
		return nil, "unknown"
	}
	lines := bytes.Split(bytes.TrimSpace(b), []byte{'\n'})
	if len(lines) > 8 {
		return nil, "unknown"
	}
	phase := "unknown"
	var diagnostic *ChildDiagnostic
	for _, line := range lines {
		if len(line) == 0 || len(line) > 1024 {
			return nil, "unknown"
		}
		var rec phaseRecord
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if e := dec.Decode(&rec); e == nil && dec.Decode(&struct{}{}) == io.EOF && rec.Version == 1 && allowedPhase(rec.Phase) {
			if diagnostic != nil {
				return nil, "unknown"
			}
			phase = rec.Phase
			continue
		}
		var d ChildDiagnostic
		dec = json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var extra any
		if e := dec.Decode(&d); e != nil || dec.Decode(&extra) != io.EOF || !validChildDiagnostic(d) || diagnostic != nil {
			return nil, "unknown"
		}
		diagnostic = &d
	}
	return diagnostic, phase
}

func pipeEvidenceComplete(overflow, waitDelay bool) bool { return !overflow && !waitDelay }
func validateClientWaitEvidence(identityErr, runErr error, observed bool) error {
	if identityErr != nil || runErr != nil {
		return errors.Join(identityErr, runErr)
	}
	if !observed {
		return errors.New("client Wait unobserved")
	}
	return nil
}
func rejectClientLifecycleClaim(c ClientBootstrapEvidence) error {
	if c.Lifecycle != nil {
		return ErrConflict
	}
	return nil
}
