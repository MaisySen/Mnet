package daemon

import (
	"context"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func expiredAt(n netmap.Node, now time.Time) bool {
	return n.KeyExpiry != nil && !now.Before(*n.KeyExpiry)
}

// unexpiredMap returns a separate map: never change the server response or
// allow path refresh to reinstall expired keys. No-expiry legacy maps keep
// their existing semantics. Unknown offline admin changes cannot be inferred.
func unexpiredMap(m *netmap.Map, now time.Time) *netmap.Map {
	out := *m
	out.Peers = nil
	if expiredAt(m.Self, now) {
		return &out
	}
	for _, p := range m.Peers {
		if !expiredAt(p, now) {
			out.Peers = append(out.Peers, p)
		}
	}
	return &out
}

func nextMapExpiry(m *netmap.Map, now time.Time) time.Time {
	var next time.Time
	consider := func(n netmap.Node) {
		if n.KeyExpiry != nil && (next.IsZero() || n.KeyExpiry.Before(next)) {
			next = *n.KeyExpiry
		}
	}
	if expiredAt(m.Self, now) {
		if len(m.Peers) > 0 {
			return now
		}
		return next
	}
	consider(m.Self)
	for _, p := range m.Peers {
		consider(p)
	}
	return next
}

// expiryLoop enforces deadlines we already know even with the control
// server offline or its long poll blocked. The same apply lock protects
// map renewal, timer expiry and endpoint refresh, so an old timer cannot
// overwrite a newer map. Down cancels and joins this loop before closing
// the device. Recheck wall time at least once per minute when armed.
func (d *Daemon) expiryLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		d.applyMu.Lock()
		var next time.Time
		if m := d.currentMap(); m != nil {
			now := time.Now()
			needsExpiry := expiredAt(m.Self, now)
			for _, p := range m.Peers {
				if expiredAt(p, now) {
					needsExpiry = true
					break
				}
			}
			if needsExpiry {
				d.applyMapLocked(m)
			}
			next = nextMapExpiry(d.currentMap(), now)
		}
		d.applyMu.Unlock()
		var timer *time.Timer
		var tick <-chan time.Time
		if !next.IsZero() {
			wait := time.Until(next)
			// Bound retries if device IPC rejects removal.
			if wait < 10*time.Millisecond {
				wait = 10 * time.Millisecond
			}
			if wait > time.Minute {
				wait = time.Minute
			}
			timer = time.NewTimer(wait)
			tick = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-d.mapChanged:
		case <-tick:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}
