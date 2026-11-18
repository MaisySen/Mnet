//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
)

func TestDaemonCleanStopWaitsForLocalAPITeardown(t *testing.T) {
	for attempt := 0; attempt < 10; attempt++ {
		state, err := os.MkdirTemp("", "mnet-stop-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(state)
		ready := make(chan error, 1)
		done := make(chan error, 1)
		stop := make(chan struct{})
		go func() {
			done <- runDaemon(options{stateDir: state, ifname: daemon.DefaultInterfaceName(), ready: ready, sessHost: true}, stop)
		}()
		select {
		case err := <-ready:
			if err != nil {
				t.Fatal(err)
			}
		case err := <-done:
			t.Fatalf("daemon exited before readiness: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("daemon did not become ready")
		}
		close(stop)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("daemon did not stop")
		}
		if _, err := os.Lstat(filepath.Join(state, "mnetd.sock")); !os.IsNotExist(err) {
			t.Fatal("daemon returned before local API socket teardown")
		}
	}
}
