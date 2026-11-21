//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestSanitizedDiagnosticTypedErrnoNeverRawError(t *testing.T) {
	secret := "private output credential value"
	e := opFailure("capset-zero", "runner", errors.Join(errors.New(secret), syscall.EPERM))
	d := DiagnosticForError(e)
	if d.Operation != "capset-zero" || d.Errno != int(syscall.EPERM) || strings.Contains(string(mustJSON(d)), secret) {
		t.Fatal(d)
	}
	d = DiagnosticForError(opFailure("outside-allowlist", "runner", errors.New(secret)))
	if d.Operation != "resident-work" || strings.Contains(string(mustJSON(d)), secret) {
		t.Fatal("external operation/error leaked")
	}
}
func TestChildStderrHashClassificationStrictNoRaw(t *testing.T) {
	valid := mustJSON(ChildDiagnostic{Version: 1, Operation: "filter-install", Class: "runner", Errno: 1})
	for _, c := range []struct {
		name     string
		b        []byte
		complete bool
		want     bool
	}{{"valid", valid, true, true}, {"raw", []byte("private text"), true, false}, {"trailing", append(append([]byte(nil), valid...), 'x'), true, false}, {"duplicate", append(append([]byte(nil), valid...), valid...), true, false}, {"truncated", valid, false, false}, {"unknown-op", mustJSON(ChildDiagnostic{Version: 1, Operation: "send raw secrets", Class: "runner"}), true, false}, {"unknown-class", mustJSON(ChildDiagnostic{Version: 1, Operation: "filter-install", Class: "private"}), true, false}, {"bad-errno", mustJSON(ChildDiagnostic{Version: 1, Operation: "filter-install", Class: "runner", Errno: 5000}), true, false}} {
		t.Run(c.name, func(t *testing.T) {
			n, hash, d := summarizedStderr(c.b, c.complete)
			if n != len(c.b) || len(hash) != 64 || (d != nil) != c.want {
				t.Fatal(n, hash, d)
			}
			if strings.Contains(string(mustJSON(d)), "private text") {
				t.Fatal("raw stderr disclosed")
			}
		})
	}
}
func TestSupervisorFailureRetainsReapAndCustodyOutcome(t *testing.T) {
	p := &fakeResident{ns: validVerdict().Namespace, r: bytes.NewReader([]byte("invalid"))}
	v, e := SuperviseResident(context.Background(), fakeSpawn{p})
	if e == nil || v.Lifecycle == nil || !v.Lifecycle.AbortAttempted || !v.Lifecycle.ReapAttempted || !v.Lifecycle.Reaped || !v.Lifecycle.CustodyCloseAttempted || !v.Lifecycle.CustodyClosed || v.Lifecycle.CustodyRetained {
		t.Fatal("failure lifecycle lost", e, v.Lifecycle)
	}
}
func TestSupervisorPendingFailureRetainsCustodyEvidence(t *testing.T) {
	p := &fakeResident{ns: validVerdict().Namespace, r: bytes.NewReader([]byte("invalid"))}
	v, e := SuperviseResident(context.Background(), stuckSpawn{stuckResident{p}})
	var pending *PendingResidentError
	if !errors.As(e, &pending) || v.Lifecycle == nil || !v.Lifecycle.CustodyRetained || v.Lifecycle.CustodyCloseAttempted || v.Lifecycle.Reaped {
		t.Fatal("pending custody hidden", e, v.Lifecycle)
	}
}
func TestSupervisorSuccessLifecycleEvidence(t *testing.T) {
	v := validVerdict()
	p := &fakeResident{ns: v.Namespace, r: bytes.NewReader(mustJSON(v))}
	v, e := SuperviseResident(context.Background(), fakeSpawn{p})
	if e != nil || v.Lifecycle == nil || !v.Lifecycle.Reaped || !v.Lifecycle.CustodyClosed || v.Lifecycle.AbortAttempted {
		t.Fatal("success lifecycle lost", e, v.Lifecycle)
	}
}
func TestEveryOperationDiagnosticRetainedSanitized(t *testing.T) {
	for op := range allowedDiagnosticOps {
		e := opFailure(op, "runner", syscall.EPERM)
		d := DiagnosticForError(e)
		if d.Operation != op || d.Errno != 1 || !validChildDiagnostic(d) {
			t.Fatal("step lost", op, d)
		}
	}
}
func TestStderrPhasesPlusTypedErrorNoRaw(t *testing.T) {
	phase := mustJSON(phaseRecord{Version: 1, Phase: "filter-installed"})
	diagnostic := mustJSON(DiagnosticForError(opFailure("client-exec", "runner", syscall.EPERM)))
	raw := append(append(phase, '\n'), diagnostic...)
	d, p := scanSanitizedStderr(raw, true)
	if d == nil || d.Operation != "client-exec" || p != "filter-installed" {
		t.Fatal("sanitized phase lost", d, p)
	}
	for _, bad := range [][]byte{append(append([]byte(nil), phase...), phase...), append(raw, []byte("\nprivate error content")...), append(append([]byte(nil), raw...), diagnostic...)} {
		d, p := scanSanitizedStderr(bad, true)
		if d != nil || p != "unknown" {
			t.Fatal("malformed/extra diagnostics trusted")
		}
	}
	if d, p := scanSanitizedStderr(raw, false); d != nil || p != "unknown" {
		t.Fatal("incomplete diagnostic trusted")
	}
}

type closeFailResident struct{ *fakeResident }

func (p closeFailResident) CloseCustody() error {
	p.calls = append(p.calls, "close-fail")
	return errors.New("close failed")
}

type closeFailSpawn struct{ p closeFailResident }

func (s closeFailSpawn) Spawn(context.Context) (ResidentProcess, error) { return s.p, nil }
func TestCustodyCloseErrorNeverClaimsClosed(t *testing.T) {
	v := validVerdict()
	v.Lifecycle = &LifecycleEvidence{PID: 777, Reaped: true, CustodyClosed: true}
	p := &fakeResident{ns: v.Namespace, r: bytes.NewReader(mustJSON(v))}
	got, e := SuperviseResident(context.Background(), closeFailSpawn{closeFailResident{p}})
	if e == nil || got.Lifecycle == nil || got.Lifecycle.CustodyClosed || !got.Lifecycle.CustodyRetained || !got.Lifecycle.CustodyCloseAttempted {
		t.Fatal("failed close claimed success", e, got.Lifecycle)
	}
	if got.Lifecycle.PID == 777 {
		t.Fatal("child lifecycle trusted")
	}
}
func TestParentOverwritesChildLifecycle(t *testing.T) {
	v := validVerdict()
	v.Lifecycle = &LifecycleEvidence{PID: 777, StartTicks: 123, Reaped: false, CustodyRetained: true}
	p := &fakeResident{ns: v.Namespace, r: bytes.NewReader(mustJSON(v))}
	got, e := SuperviseResident(context.Background(), fakeSpawn{p})
	if e != nil || got.Lifecycle == nil || got.Lifecycle.PID != 0 || got.Lifecycle.StartTicks != 0 || !got.Lifecycle.Reaped || !got.Lifecycle.CustodyClosed || got.Lifecycle.CustodyRetained {
		t.Fatal("child lifecycle not overwritten", e, got.Lifecycle)
	}
}
func TestClientLifecycleClaimRejected(t *testing.T) {
	if e := rejectClientLifecycleClaim(ClientBootstrapEvidence{}); e != nil {
		t.Fatal(e)
	}
	if e := rejectClientLifecycleClaim(ClientBootstrapEvidence{Lifecycle: &LifecycleEvidence{Reaped: true, CustodyClosed: true}}); e == nil {
		t.Fatal("client self-attested lifecycle accepted")
	}
}
func TestStderrCompletenessOverflowAndWaitDelayDirect(t *testing.T) {
	for _, v := range []struct{ over, delay, want bool }{{false, false, true}, {true, false, false}, {false, true, false}, {true, true, false}} {
		if pipeEvidenceComplete(v.over, v.delay) != v.want {
			t.Fatal("incomplete pipe called complete", v)
		}
	}
}
func TestIdentityReadFailureNeverIgnored(t *testing.T) {
	identity := errors.New("identity unavailable")
	if !errors.Is(validateClientWaitEvidence(identity, nil, true), identity) {
		t.Fatal("identityErr ignored")
	}
	if validateClientWaitEvidence(nil, nil, false) == nil {
		t.Fatal("unobserved Wait accepted")
	}
	if e := validateClientWaitEvidence(nil, nil, true); e != nil {
		t.Fatal(e)
	}
}
