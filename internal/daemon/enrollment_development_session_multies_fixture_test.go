//go:build mnet_enrollment_development && multies_native_fixture

package daemon

import (
	"context"
	"fintex.au/mnet/internal/controlclient"
	"golang.zx2c4.com/wireguard/tun"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func init() {
	developmentTransportStart = func(t *testing.T, d *Daemon, device tun.Device) (func(), func() error) {
		t.Helper()
		limits := controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 16}
		m, err := d.nativeCurrentMap(context.Background(), limits)
		if err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		local, remote := net.Pipe()
		t.Cleanup(func() { remote.Close(); listener.Close(); local.Close() })
		acceptEnded := make(chan error, 1)
		go func() { _, err := listener.Accept(); acceptEnded <- err }()
		readEnded := make(chan error, 1)
		go func() { var b [1]byte; _, err := local.Read(b[:]); readEnded <- err }()
		ready := make(chan *DevelopmentNetworkSession, 1)
		finish := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			result <- d.runDevelopmentNetwork(context.Background(), m, &DevelopmentNetworkOptions{Limits: limits, Device: func(string) (tun.Device, error) { return device, nil }, Run: func(ctx context.Context, s *DevelopmentNetworkSession) error {
				if err := s.Own(listener); err != nil {
					return err
				}
				if err := s.Own(local); err != nil {
					return err
				}
				ready <- s
				select {
				case <-finish:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}})
		}()
		var session *DevelopmentNetworkSession
		select {
		case session = <-ready:
		case err := <-result:
			t.Fatal("explicit disposable session failed", err)
		case <-time.After(3 * time.Second):
			t.Fatal("explicit disposable session startup timeout")
		}
		return func() {
			for _, ended := range []<-chan error{acceptEnded, readEnded} {
				select {
				case err := <-ended:
					t.Fatalf("resource closed before network stop: %v", err)
				default:
				}
			}
			mode := os.Getenv("MNET_DEVELOPMENT_NETWORK_SESSION_CASE")
			switch mode {
			case "policy-change":
				path := filepath.Join(d.stateOwner.lease.Parent(), "enrollment-policy-v2.json")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(raw, ' '), 0600); err != nil {
					t.Fatal(err)
				}
				select {
				case <-session.Done():
				case <-time.After(2 * time.Second):
					t.Fatal("changed policy did not stop the session")
				}
			case "expiry":
				select {
				case <-session.Done():
				case <-time.After(16 * time.Second):
					t.Fatal("original receipt deadline did not stop session")
				}
			}
			close(finish)
			select {
			case err := <-result:
				if mode == "" && err != nil {
					t.Error("explicit disposable session ended unexpectedly", err)
				}
				if mode != "" && err == nil {
					t.Error("retired session did not report cancellation")
				}

			case <-time.After(3 * time.Second):
				t.Fatal("explicit disposable session stop did not join")
			}
			select {
			case <-session.Done():
			default:
				t.Error("stopped session still active")
			}
			for _, ended := range []<-chan error{acceptEnded, readEnded} {
				select {
				case err := <-ended:
					if err == nil {
						t.Error("retired session resource did not fail")
					}
				case <-time.After(time.Second):
					t.Error("retired session left a blocked listener/connection")
				}
			}
			if len(session.Peers()) != 0 {
				t.Error("stopped session still advertises peers")
			}
			d.deviceMu.RLock()
			running := d.dev != nil
			d.deviceMu.RUnlock()
			if running {
				t.Error("stopped session retained device")
			}
		}, session.Refresh
	}
}
