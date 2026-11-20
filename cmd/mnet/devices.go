package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fintex.au/mnet/internal/daemon"
)

const devicePageSize = 8

// An explicit read-only snapshot. It creates no session or network authority.
func readDeviceSnapshot(ctx context.Context, hc *http.Client, base string) (*daemon.Status, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("local status owner unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/localapi/status", nil)
	if err != nil {
		return nil, errors.New("local status address unavailable")
	}
	response, err := hc.Do(req)
	if err != nil {
		return nil, errors.New("local daemon unavailable or status timed out")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, errors.New("local status refused; check this daemon's local capability")
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("local daemon status unavailable; refresh to try again")
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("local status unreadable or too large")
	}
	var st daemon.Status
	if err = json.Unmarshal(b, &st); err != nil || st.Status == "" || len(st.Peers) > 4096 {
		return nil, errors.New("local status response invalid")
	}
	if ctx.Err() != nil {
		return nil, errors.New("local status request canceled")
	}
	return &st, nil
}

const deviceSnapshotMaxAge = 30 * time.Second

type timedDeviceSnapshot struct {
	status  *daemon.Status
	started time.Time
}

func (s timedDeviceSnapshot) current(now time.Time) bool {
	return s.status != nil && !s.started.IsZero() && !now.Before(s.started) && now.Sub(s.started) < deviceSnapshotMaxAge
}
func readTimedDeviceSnapshot(ctx context.Context, hc *http.Client, base string) (timedDeviceSnapshot, error) {
	started := time.Now()
	status, err := readDeviceSnapshot(ctx, hc, base)
	if err != nil {
		return timedDeviceSnapshot{}, err
	}
	snapshot := timedDeviceSnapshot{status, started}
	if !snapshot.current(time.Now()) {
		return timedDeviceSnapshot{}, errors.New("local status snapshot expired while reading")
	}
	return snapshot, nil
}

// Core X11's fixed font has no general Unicode text support. Untrusted names
// cannot add lines, control characters, markup or unbounded drawing requests.
func deviceText(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == limit {
			break
		}
		if r < 32 || r > 126 {
			r = '?'
		}
		b.WriteRune(r)
		n++
	}
	if n == 0 {
		return "(not reported)"
	}
	return b.String()
}

// Page movement is local snapshot inspection. It cannot select a peer or read
// the daemon, and an unavailable/non-Up snapshot has no peer pages.
func devicePageForKey(key uint32, state string, page, peers int) (int, bool) {
	end := 0
	if state == "up" && peers > 0 {
		end = (peers - 1) / devicePageSize
	}
	switch key {
	case 0xff51, 0xff55:
		page-- // Left/PageUp
	case 0xff53, 0xff56:
		page++ // Right/PageDown
	case 0xff50:
		page = 0 // Home
	case 0xff57:
		page = end // End
	default:
		return page, false
	}
	return min(max(page, 0), end), true
}

func devicePeerState(p daemon.PeerStatus) string {
	switch p.HandshakeState {
	case "unavailable", "clock-skew":
		return "unknown"
	case "never", "stale":
		return "offline"
	case "recent":
		if p.LastHandshake < 0 {
			return "unknown"
		}
		if p.Online {
			return "online"
		}
		return "offline"
	case "": // Compatibility with an older daemon, never infer missing telemetry.
		if p.LastHandshake < 0 {
			return "unknown"
		}
		if p.Online {
			return "online"
		}
		return "offline"
	default:
		return "unknown"
	}
}
func deviceSnapshotLines(st *daemon.Status, page int, sampled time.Time) ([]string, int) {
	lines := []string{"MNET / LOCAL DEVICE SNAPSHOT", "Read only. This window cannot enable networking, enroll or start remote input."}
	if st == nil {
		return append(lines, "No current snapshot. Refresh reads only this local daemon."), 0
	}
	state := "unknown"
	switch st.Status {
	case "up", "down", "preparing", "joining", "waiting-approval", "key-expired":
		state = st.Status
	}
	lines = append(lines, "Local daemon: "+state+"  |  Read started: "+sampled.Format("15:04:05 MST"), "This device: "+deviceText(st.NodeName, 32)+"  |  IPv4: "+deviceText(st.IPv4, 20), "Network label: "+deviceText(st.Network, 48)+"  (not verified Multies identity)")
	if st.Status != "up" {
		hint := "Peer rows hidden while local daemon is not Up. No saved rows imply access."
		if st.Status == "waiting-approval" {
			hint = "Approval required. Use private mnet status output; no approval URL is shown here."
		}
		return append(lines, hint), 0
	}
	lines = append(lines, "Up is local readiness, not traffic proof. Peer online/path are reported telemetry.")
	total := len(st.Peers)
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / devicePageSize
	}
	if page < 0 {
		page = 0
	}
	if page > maxPage {
		page = maxPage
	}
	start := page * devicePageSize
	end := min(start+devicePageSize, total)
	if total == 0 {
		return append(lines, "No peers reported in this snapshot."), 0
	}
	lines = append(lines, fmt.Sprintf("Peers %d-%d of %d. Names are labels, not verified identity or approval.", start+1, end, total), fmt.Sprintf("%-32s %-20s %-9s %-8s %s", "NAME", "IPv4", "REPORTED", "PATH", "HANDSHAKE"))
	for _, p := range st.Peers[start:end] {
		path := "unknown"
		if p.Endpoint != "" {
			path = "direct"
			if p.Relay {
				path = "relay"
			}
		}
		handshake := "unavailable"
		switch p.HandshakeState {
		case "never":
			handshake = "never"
		case "clock-skew":
			handshake = "clock skew"
		case "recent", "stale":
			if p.LastHandshake >= 0 {
				handshake = fmt.Sprintf("%ds ago", p.LastHandshake)
			}
		case "":
			if p.LastHandshake >= 0 {
				handshake = fmt.Sprintf("%ds ago", p.LastHandshake)
			}
		}
		lines = append(lines, fmt.Sprintf("%-32s %-20s %-9s %-8s %s", deviceText(p.Name, 32), deviceText(p.IPv4, 20), devicePeerState(p), path, handshake))
	}
	return lines, page
}
