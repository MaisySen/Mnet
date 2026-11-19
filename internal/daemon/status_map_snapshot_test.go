package daemon

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// Hold the actual map-application barrier while Status starts. Wait for its
// goroutine to block on that barrier, not a sleep-based scheduling guess, then
// publish the corresponding authorization status before allowing it to return.
func TestStatusWaitsForMapApplicationBeforeSnapshot(t *testing.T) {
	d := testDaemon(t)
	d.mu.Lock()
	d.status = "up"
	d.mu.Unlock()
	d.applyMu.Lock()
	locked := true
	defer func() {
		if locked {
			d.applyMu.Unlock()
		}
	}()
	result := make(chan *Status, 1)
	go func() { result <- d.Status() }()
	deadline := time.Now().Add(time.Second)
	stack := make([]byte, 1<<20)
	for {
		n := runtime.Stack(stack, true)
		blocked := false
		for _, goroutine := range strings.Split(string(stack[:n]), "\n\n") {
			if strings.Contains(goroutine, "(*Daemon).Status(") && strings.Contains(goroutine, "sync.(*Mutex).Lock") {
				blocked = true
				break
			}
		}
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Status did not reach held map application barrier")
		}
		runtime.Gosched()
	}
	d.mu.Lock()
	d.status = "key-expired"
	d.mu.Unlock()
	d.applyMu.Unlock()
	locked = false
	select {
	case status := <-result:
		if status.Status != "key-expired" {
			t.Fatalf("Status waited for map completion but returned stale authorization %q", status.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("Status did not finish after map completion")
	}
}
