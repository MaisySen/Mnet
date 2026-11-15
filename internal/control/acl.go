package control

import (
	"fmt"
	"net"
	"strings"

	"fintex.au/mnet/internal/netmap"
)

// matchNode reports whether an ACL pattern names this node: "*" or the
// node name (case insensitive) or its mesh address.
func matchNode(pattern string, n *netmap.Node) bool {
	if pattern == "*" {
		return true
	}
	return strings.EqualFold(pattern, n.Name) || pattern == n.IPv4
}

// matchRoute reports whether an ACL dst pattern covers an advertised
// subnet: "*", the exact CIDR, or a wider CIDR that contains it.
func matchRoute(pattern, route string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == route {
		return true
	}
	_, pnet, perr := net.ParseCIDR(pattern)
	_, rnet, rerr := net.ParseCIDR(route)
	if perr != nil || rerr != nil {
		return false
	}
	pOnes, pBits := pnet.Mask.Size()
	rOnes, rBits := rnet.Mask.Size()
	return pBits == rBits && pOnes <= rOnes && pnet.Contains(rnet.IP)
}

// allowsPair reports whether the policy permits any traffic between a and
// b, in either direction. WireGuard peers need mutual config, so a rule
// for one direction is enough for the pair to appear in each other's map.
func allowsPair(pol *netmap.ACLPolicy, a, b *netmap.Node) bool {
	if pol == nil || len(pol.Rules) == 0 {
		return true
	}
	for _, r := range pol.Rules {
		if matchNode(r.Src, a) && (matchNode(r.Dst, b) || servesMatching(b, r.Dst)) {
			return true
		}
		if matchNode(r.Src, b) && (matchNode(r.Dst, a) || servesMatching(a, r.Dst)) {
			return true
		}
	}
	return false
}

// servesMatching reports whether n advertises a route the dst pattern
// covers; such a rule must also open the mesh path to the router itself.
func servesMatching(n *netmap.Node, dst string) bool {
	for _, rt := range n.Routes {
		if matchRoute(dst, rt) {
			return true
		}
	}
	return false
}

// filterRoutes keeps the advertised routes of peer that viewer may use.
func filterRoutes(pol *netmap.ACLPolicy, viewer, peer *netmap.Node) []string {
	if pol == nil || len(pol.Rules) == 0 {
		return peer.Routes
	}
	var out []string
	for _, rt := range peer.Routes {
		for _, r := range pol.Rules {
			if matchNode(r.Src, viewer) && matchRoute(r.Dst, rt) {
				out = append(out, rt)
				break
			}
		}
	}
	return out
}

// filterPeerInPlace filters an already detached peer snapshot, avoiding
// an allocation for each peer. Only snapshot-owned memory may be passed.
func filterPeerInPlace(pol *netmap.ACLPolicy, viewer, peer *netmap.Node) bool {
	if !allowsPair(pol, viewer, peer) {
		return false
	}
	peer.Routes = filterRoutes(pol, viewer, peer)
	if peer.ExitNode && pol != nil && len(pol.Rules) > 0 {
		allowed := false
		for _, rule := range pol.Rules {
			if matchNode(rule.Src, viewer) && (rule.Dst == "*" || rule.Dst == "0.0.0.0/0" || matchNode(rule.Dst, peer)) {
				allowed = true
				break
			}
		}
		if !allowed {
			peer.ExitNode = false
		}
	}
	return true
}

// validateACL sanity checks a policy before storing: src must be "*" or a
// node name in the network; dst may also be a CIDR.
func (s *Store) validateACL(networkID string, pol *netmap.ACLPolicy) error {
	if pol == nil {
		return nil
	}
	names := map[string]bool{}
	for _, n := range s.data.Nodes {
		if n.NetworkID == networkID {
			names[strings.ToLower(n.Name)] = true
		}
	}
	for i, r := range pol.Rules {
		if r.Src == "" || r.Dst == "" {
			return fmt.Errorf("rule %d: src and dst are required", i)
		}
		if r.Src != "*" && !names[strings.ToLower(r.Src)] {
			return fmt.Errorf("rule %d: unknown src node %q", i, r.Src)
		}
		if r.Dst != "*" && !names[strings.ToLower(r.Dst)] {
			if _, _, err := net.ParseCIDR(r.Dst); err != nil {
				return fmt.Errorf("rule %d: dst %q is not a node name or CIDR", i, r.Dst)
			}
		}
	}
	return nil
}
