package session

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestViewerControlCallbackCanEndItsOwnGate(t *testing.T) {
	var c *ViewerControl
	calls := make(chan ControlStatus, 4)
	c = NewViewerControl(func(status ControlStatus) {
		calls <- status
		c.End()
	})
	completed := make(chan struct{})
	go func() { c.End(); close(completed) }()
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("control status callback deadlocked ending its own gate")
	}
	if c.Snapshot().State != "unavailable" {
		t.Fatal("End lost terminal state")
	}
	c.End()
	select {
	case <-calls:
	default:
		t.Fatal("terminal callback absent")
	}
	select {
	case <-calls:
		t.Fatal("repeated End recursively notified terminal callback")
	default:
	}
}

func TestViewerControlCallbackCanCancelItsPendingRequest(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	var vc *ViewerControl
	var mu sync.Mutex
	var states []string
	result := make(chan error, 1)
	vc = NewViewerControl(func(status ControlStatus) {
		mu.Lock()
		states = append(states, status.State)
		mu.Unlock()
		if status.State == "pending" {
			result <- vc.Cancel()
		}
	})
	hc := NewHostControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	waitControl(t, vc, "available")
	complete := make(chan error, 1)
	go func() { complete <- vc.Request() }()
	select {
	case err := <-complete:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending callback deadlocked canceling its own request")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("callback cancel did not finish")
	}
	waitControl(t, vc, "declined")
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		mu.Lock()
		last := ""
		if len(states) > 0 {
			last = states[len(states)-1]
		}
		mu.Unlock()
		if last == "declined" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("serialized notification did not publish latest declined state")
}

func TestViewerControlCallbackCoalescesDuringBlockedAppCode(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var running atomic.Int32
	var overlap atomic.Bool
	events := make(chan ControlStatus, 8)
	vc := NewViewerControl(func(status ControlStatus) {
		if running.Add(1) != 1 {
			overlap.Store(true)
		}
		defer running.Add(-1)
		if status.State == "available" {
			close(entered)
			<-release
		}
		events <- status
	})
	hc := NewHostControl(nil)
	ConnectViewerWithOptions(client, ViewerOptions{Control: vc})
	h := ServeHostWithOptions(host, &authorizationCapturer{}, &authorizationInjector{}, 8, HostOptions{Control: hc, NegotiateControl: true})
	defer h.Stop()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("status callback did not block")
	}
	completed := make(chan error, 1)
	go func() {
		if err := vc.Request(); err != nil {
			completed <- err
			return
		}
		if err := vc.Cancel(); err != nil {
			completed <- err
			return
		}
		for i := 0; i < 1000; i++ {
			vc.End()
		}
		completed <- nil
	}()
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("app callback held control operations hostage")
	}
	if vc.BindInput(&InputEvent{}) == nil {
		t.Fatal("blocked callback preserved input after End")
	}
	releaseOnce.Do(func() { close(release) })
	for _, state := range []string{"available", "unavailable"} {
		select {
		case got := <-events:
			if got.State != state {
				t.Fatalf("queued stale status%s, want%s", got.State, state)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("latest notification not drained")
		}
	}
	if overlap.Load() {
		t.Fatal("control callbacks overlapped")
	}
	select {
	case got := <-events:
		t.Fatal("unbounded duplicate terminal callback", got.State)
	default:
	}
}
