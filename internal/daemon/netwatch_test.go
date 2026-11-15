//go:build linux

package daemon

import (
	"context"
	"testing"
	"time"
)

func TestWatchNetworkChangesLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := watchNetworkChanges(ctx)
	if ch == nil {
		t.Skip("rtnetlink unavailable in this environment")
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			// a stray event is fine; the channel must close soon after
			select {
			case <-ch:
			case <-time.After(2 * time.Second):
				t.Fatal("channel did not close after cancel")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after cancel")
	}
}
