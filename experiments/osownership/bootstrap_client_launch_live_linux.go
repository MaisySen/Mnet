//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_process_launch

package osownership

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func runReadOnlyClientAndJoin(ctx context.Context, ns *namespaceCustody) (ClientBootstrapEvidence, error) {
	var out ClientBootstrapEvidence
	if e := ns.checkCurrent(); e != nil {
		return out, e
	}
	exe, e := os.Executable()
	if e != nil {
		return out, e
	}
	c := exec.CommandContext(ctx, exe, "--osownership-client-capdrop-bootstrap-source")
	c.Env = []string{"PATH=/usr/bin:/bin", "MNO_CLIENT_PARENT_PID=" + strconv.Itoa(os.Getpid())}
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	var stdout, stderr cappedEvidenceBuffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	c.WaitDelay = 250 * time.Millisecond
	failure := FailureEvidence{Operation: "client-start", Class: "runner"}
	if e = c.Start(); e != nil {
		return out, &childRunFailure{failure, e}
	}
	failure.PID = c.Process.Pid
	ticks, identityErr := readProcessStartTicks(c.Process.Pid)
	failure.StartTicks = ticks
	failure.IdentityObserved = identityErr == nil
	runErr := c.Wait()
	failure.Operation = "client-wait"
	failure.Reaped = c.ProcessState != nil
	failure.ExitObserved = c.ProcessState != nil
	if state := c.ProcessState; state != nil {
		failure.ExitCode = state.ExitCode()
		if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			failure.Signal = int(status.Signal())
		}
	}
	failure.StdoutBytes = stdout.Len()
	failure.StdoutComplete = pipeEvidenceComplete(stdout.overflow, errors.Is(runErr, exec.ErrWaitDelay))
	failure.StderrComplete = pipeEvidenceComplete(stderr.overflow, errors.Is(runErr, exec.ErrWaitDelay))
	failure.StderrBytes, failure.StderrSHA256, failure.ChildDiagnostic = summarizedStderr(stderr.Bytes(), failure.StderrComplete)
	runtimeSummary := classifyRuntimeStderr(stderr.Bytes(), failure.StderrComplete)
	failure.Runtime = &runtimeSummary
	failure.ChildDiagnostic, failure.Phase = scanSanitizedStderr(stderr.Bytes(), failure.StderrComplete)
	if failure.ChildDiagnostic == nil && runtimeSummary.LeadingDiagnostic != nil {
		failure.ChildDiagnostic = runtimeSummary.LeadingDiagnostic
	}
	if failure.Phase == "unknown" {
		failure.Phase = runtimeSummary.LastStrictPhase
	}
	if failure.ChildDiagnostic != nil {
		failure.Operation = failure.ChildDiagnostic.Operation
		failure.Class = failure.ChildDiagnostic.Class
		failure.Errno = failure.ChildDiagnostic.Errno
	}
	failure.StdoutJSON = json.Valid(stdout.Bytes())
	wrap := func(cause error) (ClientBootstrapEvidence, error) { return out, &childRunFailure{failure, cause} }
	if e = validateClientWaitEvidence(identityErr, runErr, failure.ExitObserved); e != nil {
		return wrap(e)
	}
	failure.Operation = "client-evidence-decode"
	d := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	d.DisallowUnknownFields()
	if e = d.Decode(&out); e != nil {
		return wrap(e)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return wrap(ErrConflict)
	}
	if out.FailureClass != "" {
		failure.Class = "harness"
		failure.Operation = "client-fd-inventory"
		return wrap(&FDInventoryHarnessError{})
	}
	if out.ParentPID != os.Getpid() || out.PID != c.Process.Pid {
		return wrap(ErrConflict)
	}
	if e = rejectClientLifecycleClaim(out); e != nil {
		return wrap(e)
	}
	if e = validateClientEvidence(out, ns.identity); e != nil {
		return wrap(e)
	}
	out.Lifecycle = &LifecycleEvidence{PID: c.Process.Pid, StartTicks: ticks, IdentityObserved: true, ReapAttempted: true, Reaped: true, ExitCode: 0}
	return out, ns.checkCurrent()
}
