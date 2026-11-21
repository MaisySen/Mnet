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
		if r.Token != "" || r.Errno != 0 {
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
