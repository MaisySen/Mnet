//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence

package osownership

import (
	"golang.org/x/sys/unix"
	"strings"
	"testing"
)

func fakeClientEvidence() ClientBootstrapEvidence {
	return ClientBootstrapEvidence{PID: 100, ParentPID: 99, UID: 0, Status: map[string]string{"CapInh": "0000000000000000", "CapPrm": "0000000000000000", "CapEff": "0000000000000000", "CapBnd": "0000000000000000", "CapAmb": "0000000000000000", "NoNewPrivs": "1", "Seccomp": "2", "Seccomp_filters": "1"}, FDs: []int{0, 1, 2}, Namespace: Namespace{Device: 7, Inode: 9}, ProbeDenied: map[string]int{"owner-signal0": int(unix.EPERM)}}
}
func TestReadOnlyDriverEvidenceGuardsNoProcess(t *testing.T) {
	for _, name := range []string{"valid", "cap", "nnp", "seccomp", "fd3", "namespace", "parent", "signal-probe"} {
		c := fakeClientEvidence()
		switch name {
		case "cap":
			c.Status["CapEff"] = "1"
		case "nnp":
			c.Status["NoNewPrivs"] = "0"
		case "seccomp":
			c.Status["Seccomp"] = "0"
		case "fd3":
			c.FDs = append(c.FDs, 3)
		case "namespace":
			c.Namespace.Inode++
		case "parent":
			c.ParentPID = 1
		case "signal-probe":
			c.ProbeDenied["owner-signal0"] = 0
		}
		e := validateClientEvidence(c, Namespace{Device: 7, Inode: 9})
		if (e == nil) != (name == "valid") {
			t.Fatal(name, e)
		}
	}
}
func TestReadOnlyDriverCannotReleaseMutationConstruction(t *testing.T) {
	if kernelExecutionReleased || !readOnlyEvidenceBuild {
		t.Fatal("read-only build widened execution guard")
	}
	s := sourceFunction(t, "rtnetlink_linux.go", "applyExact")
	if !strings.Contains(s, "if !kernelExecutionReleased {") {
		t.Fatal("mutation guard widened")
	}
	s = sourceFunction(t, "kernel_linux.go", "openExclusiveNonpersistentTUN")
	if !strings.Contains(s, "if !kernelExecutionReleased {") {
		t.Fatal("TUN guard widened")
	}
	s = sourceFunction(t, "custody_linux.go", "disableOwnedIPv6")
	if !strings.Contains(s, "if !kernelExecutionReleased {") {
		t.Fatal("sysctl write guard widened")
	}
}
func TestReadOnlyDriverOutputCap(t *testing.T) {
	var b cappedEvidenceBuffer
	if _, e := b.Write(make([]byte, 8192)); e != nil {
		t.Fatal(e)
	}
	if _, e := b.Write([]byte{1}); e == nil {
		t.Fatal("output cap ignored")
	}
}
func TestReadOnlyFailureVerdictContainsBaselineAndDiagnosticSource(t *testing.T) {
	s := sourceFunction(t, "bootstrap_evidence_linux.go", "runReadOnlyBootstrapEvidence")
	baseline := strings.Index(s, "v.Bootstrap = &BootstrapReadOnlyEvidence")
	client := strings.Index(s, "runReadOnlyClientAndJoin(")
	if baseline < 0 || client < baseline {
		t.Fatal("baseline evidence not retained before failing client")
	}
	for _, term := range []string{"v.Failure = &child.evidence", "v.FailureClass = child.evidence.Class"} {
		if !strings.Contains(s, term) {
			t.Fatal("typed failure retention missing", term)
		}
	}
	s = sourceFunction(t, "bootstrap_evidence_linux.go", "runReadOnlyClientAndJoin")
	for _, term := range []string{"failure.StdoutBytes = stdout.Len()", "failure.StdoutJSON = json.Valid(stdout.Bytes())", "failure.Reaped = c.ProcessState != nil", "readProcessStartTicks(c.Process.Pid)", "scanSanitizedStderr("} {
		if !strings.Contains(s, term) {
			t.Fatal("failure pin missing", term)
		}
	}
}
func TestEvidencePathActuallyUsesHonestyGuards(t *testing.T) {
	s := sourceFunction(t, "bootstrap_evidence_linux.go", "runReadOnlyClientAndJoin")
	for _, term := range []string{"rejectClientLifecycleClaim(out)", "validateClientWaitEvidence(identityErr, runErr, failure.ExitObserved)", "failure.StderrComplete = pipeEvidenceComplete(stderr.overflow, errors.Is(runErr, exec.ErrWaitDelay))"} {
		if !strings.Contains(s, term) {
			t.Fatal("honesty guard detached from evidence path", term)
		}
	}
}
func TestFixedDispatchModesAndErrorExitSource(t *testing.T) {
	s := sourceFunction(t, "bootstrap_evidence_linux.go", "BootstrapEvidenceMain")
	for _, mode := range []string{"--bootstrap-readonly", "--osownership-resident-no-tun-source", "--osownership-client-capdrop-bootstrap-source", "--osownership-client-unprivileged-source"} {
		if strings.Count(s, "case \""+mode+"\":") != 1 {
			t.Fatal("fixed dispatch missing/duplicate", mode)
		}
	}
	s = sourceFunction(t, "client_sandbox_linux.go", "execUnprivilegedClientSource")
	if !strings.Contains(s, "--osownership-client-unprivileged-source") {
		t.Fatal("client exec target not dispatchable")
	}
	s = sourceFunction(t, "cmd/bootstrap-evidence/main_linux.go", "main")
	if !strings.Contains(s, "os.Exit(1)") || strings.Contains(s, "os.Exit(2)") {
		t.Fatal("fixed main exit convention changed")
	}
}
