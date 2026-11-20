package main

import (
	"errors"
	"net/netip"
	"strings"

	"fintex.au/mnet/internal/daemon"
)

// The status map supplies a diagnostic address, not a remote identity grant.
// Never let duplicate names/aliases or stale node state choose a machine silently.
func selectPingTarget(selector string, st *daemon.Status) (string, error) {
	if st == nil || st.Status != "up" {
		return "", errors.New("ping requires the local daemon to be up")
	}
	if selector == "" || selector != strings.TrimSpace(selector) {
		return "", errors.New("ping requires one exact peer name, FQDN or IPv4 address")
	}
	var matched []daemon.PeerStatus
	for _, p := range st.Peers {
		if p.Name == selector || p.IPv4 == selector || p.FQDN == selector {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		return "", errors.New("no current peer matches the ping target")
	}
	if len(matched) != 1 {
		return "", errors.New("ping target is ambiguous; use a unique current peer address")
	}
	target := matched[0].IPv4
	ip, err := netip.ParseAddr(target)
	if err != nil || !ip.Is4() || ip.String() != target || ip.IsUnspecified() || ip.IsMulticast() || target == "255.255.255.255" {
		return "", errors.New("peer has no valid unicast IPv4 ping address")
	}
	owners := 0
	for _, p := range st.Peers {
		if p.IPv4 == target {
			owners++
		}
	}
	if owners != 1 {
		return "", errors.New("peer address is ambiguous in the current map")
	}
	return target, nil
}
