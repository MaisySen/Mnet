//go:build mnet_enrollment_development

package daemon

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"golang.zx2c4.com/wireguard/tun"
)

type developmentCountCloser struct{ calls atomic.Int32 }

func (c *developmentCountCloser) Close() error { c.calls.Add(1); return nil }

func TestDevelopmentNetworkRequiresExplicitBoundedInjection(t *testing.T) {
	valid := DevelopmentNetworkOptions{Limits: controlclient.RegistrarLimits{Timeout: time.Second, MaxConcurrent: 1, MaxRequests: 8}, Device: func(string) (tun.Device, error) { t.Fatal("refused options created a device"); return nil, nil }, Run: func(context.Context, *DevelopmentNetworkSession) error {
		t.Fatal("refused options ran session")
		return nil
	}}
	for _, name := range []string{"no-device", "no-run", "long-timeout", "no-timeout", "parallel-budget", "large-budget", "small-budget"} {
		t.Run(name, func(t *testing.T) {
			options := valid
			switch name {
			case "no-device":
				options.Device = nil
			case "no-run":
				options.Run = nil
			case "long-timeout":
				options.Limits.Timeout = 4 * time.Second
			case "no-timeout":
				options.Limits.Timeout = 0
			case "parallel-budget":
				options.Limits.MaxConcurrent = 2
			case "large-budget":
				options.Limits.MaxRequests = 17
			case "small-budget":
				options.Limits.MaxRequests = 3
			}
			if validDevelopmentNetworkOptions(&options) {
				t.Fatal("unsafe disposable session options admitted")
			}
			if err := RunDisposableEnrollmentNetwork(context.Background(), t.TempDir(), func(DevelopmentEnrollmentEvent) error { t.Fatal("refused options emitted event"); return nil }, options); err == nil {
				t.Fatal("invalid session admitted")
			}
		})
	}
}
func TestDevelopmentNetworkRejectsNonlocalEndpoints(t *testing.T) {
	for _, raw := range []string{"192.0.2.1:123", "[::1]:123", "localhost:123", "127.0.0.1:0", "https://127.0.0.1:123", "127.0.0.1"} {
		if developmentLoopbackEndpoint(raw) {
			t.Fatal("non-disposable endpoint admitted", raw)
		}
	}
	if !developmentLoopbackEndpoint("127.0.0.1:1234") {
		t.Fatal("owned loopback endpoint refused")
	}
}
func TestDevelopmentNetworkOwnClosesConsumersAndRejectsLateResources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &DevelopmentNetworkSession{ctx: ctx}
	c := &developmentCountCloser{}
	if err := s.Own(c); err != nil {
		t.Fatal(err)
	}
	cancel()
	s.closeResources()
	s.closeResources()
	if c.calls.Load() != 1 {
		t.Fatal("consumer was not closed exactly once")
	}
	late := &developmentCountCloser{}
	if err := s.Own(late); err == nil || late.calls.Load() != 1 {
		t.Fatal("retired session retained late resource")
	}
	if err := s.Own(io.Closer(nil)); err == nil {
		t.Fatal("nil consumer accepted")
	}
}
