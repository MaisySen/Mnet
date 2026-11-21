//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence

package osownership

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Standalone experiment only. No app entry/caller references this function.
// This source is prepared for review; compilation is not permission to run.
func BootstrapEvidenceMain(args []string) error {
	if len(args) != 1 {
		return errors.New("exactly one fixed evidence mode required")
	}
	switch args[0] {
	case "--bootstrap-readonly":
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		outer, e := numericCurrentNamespace()
		if e != nil {
			return e
		}
		v, runErr := SuperviseResident(ctx, residentSpawner{})
		if runErr == nil {
			runErr = validateBootstrapEvidence(v.Bootstrap, outer)
		}
		after, afterErr := numericCurrentNamespace()
		runErr = errors.Join(runErr, afterErr)
		unchanged := afterErr == nil && after == outer
		if !unchanged {
			runErr = errors.Join(runErr, ErrConflict)
		}
		result := struct {
			Version        int       `json:"version"`
			Outcome        string    `json:"outcome"`
			OuterInitial   Namespace `json:"outer_initial"`
			OuterFinal     Namespace `json:"outer_final"`
			OuterUnchanged bool      `json:"outer_unchanged"`
			Verdict        Verdict   `json:"verdict"`
			Error          string    `json:"error,omitempty"`
		}{Version: 1, Outcome: "pass", OuterInitial: outer, OuterFinal: after, OuterUnchanged: unchanged, Verdict: v}
		if runErr != nil {
			result.Outcome = "fail"
			result.Error = runErr.Error()
		}
		return errors.Join(runErr, json.NewEncoder(os.Stdout).Encode(result))
	case "--osownership-resident-no-tun-source":
		return residentBootstrapSource(context.Background(), runReadOnlyBootstrapEvidence)
	case "--osownership-client-capdrop-bootstrap-source":
		return execUnprivilegedClientSource()
	case "--osownership-client-unprivileged-source":
		return emitClientBootstrapEvidence()
	}
	return errors.New("unknown fixed evidence mode")
}
func numericCurrentNamespace() (Namespace, error) {
	f, e := os.Open("/proc/thread-self/ns/net")
	if e != nil {
		return Namespace{}, e
	}
	defer f.Close()
	var s unix.Stat_t
	if e = unix.Fstat(int(f.Fd()), &s); e != nil {
		return Namespace{}, e
	}
	return Namespace{Device: uint64(s.Dev), Inode: s.Ino}, nil
}
func runReadOnlyBootstrapEvidence(ctx context.Context, ns *namespaceCustody) (v Verdict, err error) {
	defer func() {
		if err != nil {
			var child *childRunFailure
			if errors.As(err, &child) {
				v.Failure = &child.evidence
				v.FailureClass = child.evidence.Class
			} else {
				d := DiagnosticForError(err)
				v.Failure = &FailureEvidence{Operation: d.Operation, Class: d.Class, Errno: d.Errno}
			}
		}
	}()
	v = Verdict{Version: 1, Stage: "read-only-baseline", Namespace: ns.identity, Sequence: 1, Result: "fail", FailureClass: "runner"}
	raw, e := ReadStableInventory(ctx, residentNetlink{ns}, ns.checkCurrent)
	if e != nil {
		v.FailureClass = "decoder"
		return v, e
	}
	sys, e := readPolicySysctlsSource(ctx, ns, raw)
	if e != nil {
		v.FailureClass = "sysctl-applicability"
		return v, e
	}
	parent, e := parseSourceParentNamespace()
	if e != nil {
		return v, e
	}
	baseline, e := NewBaselineCandidate(parent, ns.identity, raw, sys)
	if e != nil {
		v.FailureClass = "baseline"
		return v, e
	}
	if _, e = baseline.Compose(raw, sys, nil, false); e != nil {
		return v, e
	}
	v.BaselineHash = bootstrapEvidenceDigest(raw, sys)
	dirsInitial := map[string]string{}
	for k, val := range sys {
		if strings.HasPrefix(k, ipv6DirPrefix) {
			dirsInitial[strings.TrimPrefix(k, ipv6DirPrefix)] = val
		}
	}
	v.Bootstrap = &BootstrapReadOnlyEvidence{IPv6Directories: dirsInitial, OwnerNamespace: ns.identity, OuterNamespace: parent, LinkCount: len(raw.Links)}
	client, e := runReadOnlyClientAndJoin(ctx, ns)
	if e != nil {
		v.Stage = "client-bootstrap"
		var failure *StageFailure
		if errors.As(e, &failure) {
			v.FailureClass = failure.Class
		}
		return v, e
	}
	after, e := ReadStableInventory(ctx, residentNetlink{ns}, ns.checkCurrent)
	if e != nil {
		v.FailureClass = "decoder"
		return v, e
	}
	afterSys, e := readPolicySysctlsSource(ctx, ns, after)
	if e != nil {
		v.FailureClass = "sysctl-applicability"
		return v, e
	}
	v.FinalHash = bootstrapEvidenceDigest(after, afterSys)
	if _, e = baseline.Compose(after, afterSys, nil, false); e != nil || v.BaselineHash != v.FinalHash {
		return v, errors.Join(e, errors.New("read-only baseline changed"))
	}
	dirs := map[string]string{}
	for k, val := range afterSys {
		if strings.HasPrefix(k, ipv6DirPrefix) {
			dirs[strings.TrimPrefix(k, ipv6DirPrefix)] = val
		}
	}
	v.Bootstrap = &BootstrapReadOnlyEvidence{Client: client, IPv6Directories: dirs, OwnerNamespace: ns.identity, OuterNamespace: parent, ClientJoined: true, BaselineUnchanged: true, LinkCount: len(raw.Links)}
	v.Stage = "final"
	v.Result = "pass"
	v.FailureClass = ""
	v.CleanupConfirmed = true
	return v, nil
}
func bootstrapEvidenceDigest(raw RawInventory, s SysctlSnapshot) string {
	b := append(mustJSON(raw), mustJSON(s)...)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Bounded client stdout is evidence only. No network connection or mutation.
// Original owner thread stays pinned through Wait, preserving Pdeathsig.
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

type cappedEvidenceBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *cappedEvidenceBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 8192 {
		b.overflow = true
		return 0, errors.New("client evidence output cap")
	}
	return b.Buffer.Write(p)
}
func emitClientBootstrapEvidence() error {
	if e := emitEvidencePhase("final-entry"); e != nil {
		return e
	}
	// FIRST final-client evidence operation: raw fd enumeration before any
	// os.ReadFile/os.Open/JSON stdout can initialize runtime poll descriptors.
	fds, e := clientFDInventorySource()
	if e != nil {
		return opFailure("client-fd-inventory", "runner", e)
	}
	if e = validateFDAllowlist(fds); e != nil {
		_ = json.NewEncoder(os.Stdout).Encode(ClientBootstrapEvidence{FailureClass: "harness-fd-inventory", FDs: fds})
		return e
	}
	if e := emitEvidencePhase("fd-validated"); e != nil {
		return e
	}
	b, e := rawReadBounded("/proc/self/status", 16384)
	if e != nil {
		return opFailure("client-status", "runner", e)
	}
	if e = validateClientStatus(string(b)); e != nil {
		return e
	}
	status := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb", "NoNewPrivs", "Seccomp", "Seccomp_filters":
			status[kv[0]] = strings.TrimSpace(kv[1])
		}
	}
	ns, e := rawCurrentNamespace()
	if e != nil {
		return opFailure("client-namespace", "runner", e)
	}
	finalFDs, e := clientFDInventorySource()
	if e != nil {
		return e
	}
	if e = validateFDAllowlist(finalFDs); e != nil {
		return e
	}
	if !reflect.DeepEqual(fds, finalFDs) {
		return &FDInventoryHarnessError{}
	}
	filterBaseline, e := strconv.Atoi(os.Getenv("MNO_CLIENT_FILTER_BASELINE"))
	if e != nil || filterBaseline < 0 {
		return opFailure("client-filter-count", "runner", ErrConflict)
	}
	out := ClientBootstrapEvidence{FilterBaseline: filterBaseline, PID: os.Getpid(), ParentPID: os.Getppid(), UID: os.Getuid(), Status: status, FDs: fds, Namespace: ns, ProbeDenied: map[string]int{}}
	// Signal0 cannot deliver any signal. EPERM proves this specific real filter
	// path refuses owner probing; no crash/termination signal is used. No unshare,
	// setns, namespace creation or network mutation probes are attempted.
	e = unix.Kill(os.Getppid(), 0)
	if e != unix.EPERM {
		return opFailure("owner-signal0", "custody", errors.New("owner signal0 unexpectedly permitted"))
	}
	out.ProbeDenied["owner-signal0"] = int(unix.EPERM)
	return opFailure("client-output", "runner", json.NewEncoder(os.Stdout).Encode(out))
}
func validateClientEvidence(c ClientBootstrapEvidence, target Namespace) error {
	var status strings.Builder
	for k, v := range c.Status {
		fmt.Fprintf(&status, "%s: %s\n", k, v)
	}
	if e := validateClientStatus(status.String()); e != nil {
		return e
	}
	if e := validateFDAllowlist(c.FDs); e != nil {
		return e
	}
	filterCount, e := strconv.Atoi(c.Status["Seccomp_filters"])
	if e != nil || c.FilterBaseline < 0 || filterCount != c.FilterBaseline+1 {
		return errors.New("new signal barrier not accounted")
	}
	if c.FailureClass != "" {
		return ErrConflict
	}
	if c.PID <= 1 || c.ParentPID <= 1 || c.UID != 0 || c.Namespace.Device != target.Device || c.Namespace.Inode != target.Inode || c.ProbeDenied["owner-signal0"] != int(unix.EPERM) {
		return ErrConflict
	}
	return nil
}
func validateBootstrapEvidence(b *BootstrapReadOnlyEvidence, outer Namespace) error {
	if b == nil || !b.ClientJoined || !b.BaselineUnchanged || b.LinkCount <= 0 || b.OuterNamespace != outer || b.OwnerNamespace.Device == outer.Device && b.OwnerNamespace.Inode == outer.Inode {
		return ErrConflict
	}
	if e := validateClientEvidence(b.Client, b.OwnerNamespace); e != nil {
		return e
	}
	if len(b.IPv6Directories) < 3 {
		return ErrConflict
	}
	for _, v := range b.IPv6Directories {
		if v != "present" && v != "ENOENT" {
			return ErrConflict
		}
	}
	return nil
}

func rawCurrentNamespace() (Namespace, error) {
	fd, e := unix.Open("/proc/thread-self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return Namespace{}, e
	}
	defer unix.Close(fd)
	var s unix.Stat_t
	if e = unix.Fstat(fd, &s); e != nil {
		return Namespace{}, e
	}
	return Namespace{Device: uint64(s.Dev), Inode: s.Ino}, nil
}
