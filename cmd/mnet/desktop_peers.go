package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"fintex.au/mnet/internal/daemon"
)

const desktopPeerMaxAge = 30 * time.Second

type desktopPeerChoice struct {
	Peer    daemon.SessionPeer
	Pointer bool
}
type desktopPeerSnapshot struct {
	Peers   []daemon.SessionPeer
	Sampled time.Time
}

// A label is never a selector by itself. Every displayed row has an exact,
// unambiguous status and current session-authorization match.
func desktopEligiblePeers(st *daemon.Status, peers []daemon.SessionPeer) ([]daemon.SessionPeer, error) {
	if st == nil || st.Status != "up" || len(st.Peers) > 4096 || len(peers) > 4096 {
		return nil, errors.New("current network/session evidence unavailable")
	}
	out := []daemon.SessionPeer{}
	for _, peer := range peers {
		selected, err := selectCLISessionPeer(peer.IPv4, st, peers)
		if err == nil && selected == peer {
			out = append(out, peer)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].IPv4 < out[j].IPv4
	})
	return out, nil
}
func readDesktopPeerSnapshot(ctx context.Context, hc *http.Client, base string) (desktopPeerSnapshot, error) {
	if ctx == nil || ctx.Err() != nil {
		return desktopPeerSnapshot{}, errors.New("local peer snapshot owner unavailable")
	}
	// The oldest read bounds freshness, not completion of the last response.
	started := time.Now()
	st, err := readDeviceSnapshot(ctx, hc, base)
	if err != nil {
		return desktopPeerSnapshot{}, err
	}
	if ctx.Err() != nil {
		return desktopPeerSnapshot{}, errors.New("local peer snapshot canceled")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/localapi/session-peers", nil)
	if err != nil {
		return desktopPeerSnapshot{}, errors.New("local session address unavailable")
	}
	response, err := hc.Do(request)
	if err != nil {
		return desktopPeerSnapshot{}, errors.New("local session evidence unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return desktopPeerSnapshot{}, errors.New("local session evidence refused")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return desktopPeerSnapshot{}, errors.New("local session evidence too large")
	}
	var peers []daemon.SessionPeer
	if json.Unmarshal(data, &peers) != nil {
		return desktopPeerSnapshot{}, errors.New("local session evidence invalid")
	}
	eligible, err := desktopEligiblePeers(st, peers)
	if err != nil {
		return desktopPeerSnapshot{}, err
	}
	if ctx.Err() != nil {
		return desktopPeerSnapshot{}, errors.New("local peer snapshot canceled")
	}
	if time.Since(started) >= desktopPeerMaxAge {
		return desktopPeerSnapshot{}, errors.New("local peer snapshot expired while reading")
	}
	return desktopPeerSnapshot{eligible, started}, nil
}

type desktopPeerSelection struct {
	snapshot desktopPeerSnapshot
	selected int
}

func newDesktopPeerSelection() desktopPeerSelection { return desktopPeerSelection{selected: -1} }
func (s *desktopPeerSelection) clear()              { s.snapshot = desktopPeerSnapshot{}; s.selected = -1 }
func (s *desktopPeerSelection) current(now time.Time) bool {
	return !s.snapshot.Sampled.IsZero() && !now.Before(s.snapshot.Sampled) && now.Sub(s.snapshot.Sampled) < desktopPeerMaxAge
}
func (s *desktopPeerSelection) replace(snapshot desktopPeerSnapshot) {
	s.clear()
	s.snapshot = snapshot
}
func (s *desktopPeerSelection) choose(index int, now time.Time) bool {
	if !s.current(now) || index < 0 || index >= len(s.snapshot.Peers) {
		return false
	}
	s.selected = index
	return true
}

// Endpoints are explicit local selection only, never a mode activation.
func (s *desktopPeerSelection) endpoint(last bool, now time.Time) bool {
	index := 0
	if last {
		index = len(s.snapshot.Peers) - 1
	}
	return s.choose(index, now)
}
func (s *desktopPeerSelection) choice(pointer bool, now time.Time) (desktopPeerChoice, error) {
	if !s.current(now) || s.selected < 0 || s.selected >= len(s.snapshot.Peers) {
		return desktopPeerChoice{}, errors.New("select a peer from a current snapshot")
	}
	return desktopPeerChoice{s.snapshot.Peers[s.selected], pointer}, nil
}

// Selection never carries authority across a refreshed/replaced peer. The
// existing dial checks this exact pin again and watches ongoing authorization.
func prepareDesktopChoice(ctx context.Context, choice desktopPeerChoice) (*cliSessionAuthorization, string, error) {
	access, name, err := prepareDesktopView(ctx, choice.Peer.IPv4)
	if err != nil {
		return nil, "", err
	}
	if access.peer != choice.Peer {
		return nil, "", errors.New("selected peer changed; reopen and review current identity")
	}
	return access, name, nil
}

// Selected labels remain visible even when local pagination hides their row.
// The full canonical key, not this bounded printable label, pins execution.
func (s *desktopPeerSelection) detailLines(now time.Time) []string {
	label := "No peer selected. View only or Pointer view needs an explicit row and mode."
	key := "No selected key. Labels are not verified people or Multies identity."
	if s.current(now) && s.selected >= 0 && s.selected < len(s.snapshot.Peers) {
		p := s.snapshot.Peers[s.selected]
		label = fmt.Sprintf("Selected %d/%d label: %s  IP: %s", s.selected+1, len(s.snapshot.Peers), deviceText(p.Name, 64), deviceText(p.IPv4, 45))
		key = "Exact device key: " + p.PublicKey
	}
	return []string{label, key,
		"Pointer view still needs separate host view approval and a later Ask Pointer/host answer.",
		"Tab: focus. Arrows/Home/End: select. PgUp/PgDn: page. Enter/Space: release. R: refresh. Esc: close.",
		"Snapshot expires after 30s; manual local reads only. Files/recording/keyboard are not offered."}
}

// Page keys inspect other current rows without selecting a peer or retaining
// mode focus. The selected identity remains explicit in the detail panel.
func (s *desktopPeerSelection) pageForKey(key uint32, page int, now time.Time) (int, bool) {
	if key != 0xff55 && key != 0xff56 {
		return page, false
	}
	end := 0
	if s.current(now) && len(s.snapshot.Peers) > 0 {
		end = (len(s.snapshot.Peers) - 1) / 6
	}
	if key == 0xff55 {
		page--
	} else {
		page++
	}
	return min(max(page, 0), end), true
}
