//go:build mnet_enrollment_development && multies_native_fixture

package daemon

import (
	"context"
	"fintex.au/mnet/internal/controlclient"
	"golang.zx2c4.com/wireguard/tun"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Actual public API against the real just-saved root. The emitter blocks after
// durable spend and BEFORE HTTP/device. SIGKILL must not return the slice.
func TestActualDevelopmentConnectionReservedCrash(t *testing.T) {
	root := os.Getenv("MNET_TEST_ACTUAL_CONNECTION_ROOT")
	if root == "" {
		t.Skip("explicit disposable fixture only")
	}
	options := DevelopmentNetworkOptions{Limits: controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 14}, Device: func(string) (tun.Device, error) { t.Fatal("device before kill"); return nil, nil }, Run: func(context.Context, *DevelopmentNetworkSession) error { t.Fatal("run before kill"); return nil }}
	e := ConnectDisposableEnrollment(context.Background(), root, func(ev DevelopmentEnrollmentEvent) error {
		if ev.Stage != "local-development-connection-slice-spent" {
			t.Fatal("unexpected stage")
		}
		if e := os.WriteFile(filepath.Join(root, "actual-reserved-before-http"), []byte("original API paused after durable spend before HTTP/device"), 0600); e != nil {
			t.Fatal(e)
		}
		for {
			time.Sleep(time.Second)
		}
	}, options)
	t.Fatal("must be SIGKILLed, returned", e)
}
