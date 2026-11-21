//go:build mnet_enrollment_development

package osownership

import (
	"strings"
	"testing"
)

func TestKnownRuntimeTokenClassificationNoPrivateText(t *testing.T) {
	private := "PRIVATE user secret /home/user/private-file.go 0xdeadbeef"
	raw := "{\"version\":1,\"phase\":\"filter-installed\"}\nfatal error: newosproc\nruntime: failed to create new OS thread (have 2 already; errno=1)\n" + private + "\n"
	r := classifyRuntimeStderr([]byte(raw), true)
	if r.Kind != "go-runtime-fatal" || r.Token != "newosproc" || r.Errno != 1 || r.LastStrictPhase != "filter-installed" || !r.UnparsedBytes {
		t.Fatal(r)
	}
	s := string(mustJSON(r))
	for _, secret := range []string{private, "private-file", "0xdeadbeef", "PRIVATE"} {
		if strings.Contains(s, secret) {
			t.Fatal("private runtime bytes leaked", s)
		}
	}
}
func TestRuntimeClassifierCannotAuthenticateMarkersAfterUnknownBytes(t *testing.T) {
	for _, raw := range []string{"unknown\n{\"version\":1,\"phase\":\"filter-installed\"}\n", "{\"version\":1,\"phase\":\"filter-installed\",\"extra\":1}\n", "{\"version\":1,\"phase\":\"filter-installed\"} trailing\n"} {
		r := classifyRuntimeStderr([]byte(raw), true)
		if r.LastStrictPhase != "unknown" {
			t.Fatal("late/malformed marker trusted", r)
		}
	}
}
func TestRuntimeClassifierStrictBoundedErrnoAndTokens(t *testing.T) {
	for _, raw := range []string{"runtime: failed to create new OS thread (have x already; errno=1)", "runtime: failed to create new OS thread (have 2 already; errno=99999)", "prefix fatal error: newosproc", "fatal error: newosproc private suffix"} {
		r := classifyRuntimeStderr([]byte(raw), true)
		if r.Token == "newosproc" || r.Errno != 0 {
			t.Fatal("loose runtime match", r)
		}
	}
	if r := classifyRuntimeStderr([]byte("panic: PRIVATE panic value"), true); r.Kind != "go-panic-token" || strings.Contains(string(mustJSON(r)), "PRIVATE") {
		t.Fatal(r)
	}
	if r := classifyRuntimeStderr([]byte("fatal error: newosproc"), false); r.Kind != "incomplete-stderr" {
		t.Fatal("truncated content treated complete")
	}
}
func TestRuntimeClassifierMarkerOnlyNotRuntimeFatal(t *testing.T) {
	r := classifyRuntimeStderr([]byte("{\"version\":1,\"phase\":\"final-entry\"}\n"), true)
	if r.Token != "" || r.LastStrictPhase != "final-entry" {
		t.Fatal(r)
	}
}
func TestRuntimeFiveBoundsDecisive(t *testing.T) {
	for _, line := range []string{"runtime: failed to create new OS thread (have 2 already; errno=4096)", "runtime: failed to create new OS thread (have 2 already; errno=0)", "runtime: failed to create new OS thread (have 1000001 already; errno=1)"} {
		r := classifyRuntimeStderr([]byte(line), true)
		if r.Token != "" || r.Errno != 0 {
			t.Fatal("numeric bound missing", r)
		}
	}
	marker := string(mustJSON(phaseRecord{Version: 1, Phase: "bootstrap-entry"}))
	last := string(mustJSON(phaseRecord{Version: 1, Phase: "filter-installed"}))
	r := classifyRuntimeStderr([]byte(strings.Repeat(marker+"\n", 8)+last+"\n"), true)
	if r.LastStrictPhase != "bootstrap-entry" {
		t.Fatal("ninth marker trusted", r)
	}
	r = classifyRuntimeStderr([]byte(strings.Repeat(" ", 1025-len(last))+last+"\n"), true)
	if r.LastStrictPhase != "unknown" {
		t.Fatal("over1024-byte marker trusted", r)
	}
	r = classifyRuntimeStderr([]byte(strings.Repeat(" ", 1024-len(last))+last+"\n"), true)
	if r.LastStrictPhase != "filter-installed" {
		t.Fatal("1024-byte boundary refused", r)
	}
}
func TestGenericFatalSignalGoroutineFlagsNoText(t *testing.T) {
	r := classifyRuntimeStderr([]byte("fatal error: PRIVATE runtime cause\nruntime: unexpected signal PRIVATE\n[signal PRIVATE]\ngoroutine 3 [PRIVATE]:\nPRIVATE frame.go\n"), true)
	if !r.GenericFatal || !r.UnexpectedSignal || !r.SignalHeader || r.GoroutineHeaders != 1 || r.UnparsedLines != 1 {
		t.Fatal(r)
	}
	if strings.Contains(string(mustJSON(r)), "PRIVATE") {
		t.Fatal("private generic fatal data leaked")
	}
}
func TestLeadingChildDiagnosticSurvivesUnknownTail(t *testing.T) {
	d := ChildDiagnostic{Version: 1, Operation: "securebits-lock", Class: "runner", Errno: 1, Phase: "bootstrap-pre-filter"}
	raw := append(mustJSON(d), []byte("\nfatal error: PRIVATE\nPRIVATE stack\n")...)
	r := classifyRuntimeStderr(raw, true)
	if r.LeadingDiagnostic == nil || r.LeadingDiagnostic.Operation != "securebits-lock" || r.LeadingDiagnostic.Errno != 1 {
		t.Fatal("leading diagnostic lost", r)
	}
	r = classifyRuntimeStderr(append([]byte("unknown\n"), raw...), true)
	if r.LeadingDiagnostic != nil {
		t.Fatal("late diagnostic trusted")
	}
	if strings.Contains(string(mustJSON(r)), "PRIVATE") {
		t.Fatal("raw tail leaked")
	}
}
