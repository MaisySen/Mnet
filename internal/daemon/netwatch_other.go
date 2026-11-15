//go:build !linux

package daemon

import "context"

// watchNetworkChanges has no platform feed here; endpoint discovery
// keeps its interval.
func watchNetworkChanges(ctx context.Context) <-chan struct{} { return nil }
