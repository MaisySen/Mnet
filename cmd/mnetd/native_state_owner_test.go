package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"fintex.au/mnet/internal/daemon"
)

func TestSelfTestDaemonUsesLibraryOwnerWithoutStacking(t *testing.T) {
	dir := t.TempDir()
	d, _, _, err := selfTestDaemon(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if second, _, _, err := selfTestDaemon(dir, false); err == nil {
		second.Close()
		t.Fatal("selftest bypassed native owner")
	}
	if release, err := lockDaemonState(dir); err == nil {
		release()
		t.Fatal("library owner does not exclude original process lock")
	}
	d.Close()
	fresh, _, _, err := selfTestDaemon(dir, false)
	if err != nil {
		t.Fatal("selftest Close retained owner", err)
	}
	fresh.Close()
}
func TestSessionProofFailedPrepareReleasesLibraryOwner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "fixture refusal", 403) }))
	defer server.Close()
	root := t.TempDir()
	if node, err := newProofNode(root, "failed", server.URL, "fixture", "fixture", "", false); err == nil {
		node.d.Close()
		t.Fatal("refused session proof succeeded")
	}
	fresh, err := daemon.New(daemon.Config{StateDir: filepath.Join(root, "failed-state")})
	if err != nil {
		t.Fatal("failed sessionproof leaked native owner", err)
	}
	fresh.Close()
}
