package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"fintex.au/mnet/internal/daemon"
)

func waitTimeout(args []string) (time.Duration, error) {
	timeout := 60 * time.Second
	if len(args) > 0 {
		if len(args) != 2 || args[0] != "--timeout" {
			return 0, fmt.Errorf("usage: mnet wait [--timeout 60s]")
		}
		var err error
		timeout, err = time.ParseDuration(args[1])
		if err != nil {
			return 0, fmt.Errorf("invalid wait timeout; use a duration such as 60s")
		}
	}
	if timeout <= 0 || timeout > 30*time.Minute {
		return 0, fmt.Errorf("wait timeout must be greater than zero and at most 30m")
	}
	return timeout, nil
}

// Wait observes current daemon readiness, not ownership/completion of any
// particular asynchronous Up request. A timeout never changes daemon state.
func waitUntilUp(ctx context.Context, hc *http.Client, base string, interval time.Duration) (*daemon.Status, error) {
	var last *daemon.Status
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/localapi/status", nil)
		if err != nil {
			return nil, fmt.Errorf("invalid local API address")
		}
		resp, err := hc.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return last, fmt.Errorf("wait ended: %w; daemon state was not changed", ctx.Err())
			}
			return nil, fmt.Errorf("cannot read local daemon status")
		}
		var st daemon.Status
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("daemon status failed: HTTP %d", resp.StatusCode)
		}
		err = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&st)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("invalid daemon status response")
		}
		last = &st
		switch st.Status {
		case "up":
			if st.IPv4 == "" {
				return &st, fmt.Errorf("daemon reports up without a mesh address")
			}
			return &st, nil
		case "key-expired":
			return &st, fmt.Errorf("device authorization expired; inspect local status and obtain approval")
		case "down":
			if st.LastError != "" {
				return &st, fmt.Errorf("daemon startup failed; inspect mnet status for its diagnostic")
			}
			// Up is asynchronous: its acknowledgement can precede joining. Do not
			// treat an initial Down sample as failure or claim it belongs to our call.
		case "preparing", "joining", "waiting-approval":
		default:
			return &st, fmt.Errorf("unknown daemon status")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return &st, fmt.Errorf("wait ended: %w; daemon state was not changed", ctx.Err())
		case <-timer.C:
		}
	}
}
