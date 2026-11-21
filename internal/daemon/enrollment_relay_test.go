package daemon

import (
	"context"
	"fintex.au/mnet/internal/controlclient"
	"testing"
	"time"
)

func TestNativeRelayRefusesAbsentOwnedDeviceAndContexts(t *testing.T) {
	d := &Daemon{}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, canceled, context.Background()} {
		if r, err := d.nativeCurrentRelay(ctx, controlclient.RegistrarLimits{Timeout: time.Second, MaxConcurrent: 1, MaxRequests: 1}, 4); err == nil || r != nil {
			t.Fatal("unowned/device-less relay constructed")
		}
	}
}
func TestNativeRelayGenerationCheckDoesNotWaitOnDeviceWriter(t *testing.T) {
	d := &Daemon{}
	r := &nativeRegistrarRelay{daemon: d, ctx: context.Background()}
	d.deviceMu.Lock()
	done := make(chan bool, 1)
	go func() { done <- r.current() }()
	select {
	case allowed := <-done:
		d.deviceMu.Unlock()
		if allowed {
			t.Fatal("busy device generation admitted")
		}
	case <-time.After(time.Second):
		d.deviceMu.Unlock()
		t.Fatal("relay use waited behind device teardown")
	}
}

func TestNativeRelayGenerationCheckDuringMapReader(t *testing.T) {
	d := &Daemon{}
	r := &nativeRegistrarRelay{daemon: d, ctx: context.Background()}
	d.deviceMu.RLock()
	done := make(chan bool, 1)
	go func() { done <- r.current() }()
	select {
	case allowed := <-done:
		d.deviceMu.RUnlock()
		if allowed {
			t.Fatal("absent device admitted")
		}
	case <-time.After(time.Second):
		d.deviceMu.RUnlock()
		t.Fatal("synchronous relay callback deadlocked on map reader")
	}
}
