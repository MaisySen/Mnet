//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"reflect"
	"strconv"
)

// Exact v6.17 devinet.c table spelling, including BOOTP_RELAY at index9.
// The S40 missing-leaf claim was wrong; the complete pinned table includes it.
var effectiveV4Leaves = [...]string{"forwarding", "mc_forwarding", "proxy_arp", "accept_redirects", "secure_redirects", "send_redirects", "shared_media", "rp_filter", "accept_source_route", "bootp_relay", "log_martians", "tag", "arp_filter", "medium_id", "disable_xfrm", "disable_policy", "force_igmp_version", "arp_announce", "arp_ignore", "promote_secondaries", "arp_accept", "arp_notify", "accept_local", "src_valid_mark", "proxy_arp_pvlan", "route_localnet", "igmpv2_unsolicited_report_interval", "igmpv3_unsolicited_report_interval", "ignore_routes_with_linkdown", "drop_unicast_in_l2_multicast", "drop_gratuitous_arp", "bc_forwarding", "arp_evict_nocarrier"}

type effectiveV4Observation struct {
	all, defaults [33]int32
	observed      [33]bool
}
type effectiveV4Word struct {
	Index                      int
	Name, Formula              string
	Local, All, Default, Value int32
	AllObserved, Computed      bool
}

func effectiveV4Paths() []string {
	var paths []string
	for _, scope := range []string{"all", "default"} {
		for _, leaf := range effectiveV4Leaves {
			if leaf != "" {
				paths = append(paths, "/proc/sys/net/ipv4/conf/"+scope+"/"+leaf)
			}
		}
	}
	return paths
}
func parseEffectiveV4(sys SysctlSnapshot) (*effectiveV4Observation, error) {
	out := &effectiveV4Observation{}
	for index, leaf := range effectiveV4Leaves {
		if leaf == "" {
			continue
		}
		for _, scope := range []string{"all", "default"} {
			raw, ok := sys["/proc/sys/net/ipv4/conf/"+scope+"/"+leaf]
			n, e := strconv.ParseInt(raw, 10, 32)
			if !ok || e != nil || strconv.FormatInt(n, 10) != raw {
				return nil, afPolicy
			}
			if scope == "all" {
				out.all[index] = int32(n)
			} else {
				out.defaults[index] = int32(n)
			}
		}
		out.observed[index] = true
	}
	return out, nil
}
func cBool(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

// Diagnostic computation only. A same-pass collection is not atomic and cannot
// become permission/readiness by computing values. Unknown consumer semantics
// remain visible and uncomputed, not silently treated as ALL-independent.
func (p *afReadPass) effectiveV4(index int) ([]effectiveV4Word, error) {
	if _, e := p.requestClaim(); e != nil {
		return nil, e
	}
	if p.effective == nil {
		return nil, afPolicy
	}
	observed, e := parseEffectiveV4(p.sys)
	if e != nil || !reflect.DeepEqual(observed, p.effective) {
		return nil, afPolicy
	}
	l, e := p.link(index)
	if e != nil {
		return nil, e
	}
	return effectiveV4Link(l, p.effective)
}

// Shared pure computation for a reviewed source collector and inert pass.
func effectiveV4Link(l LinkRecord, observed *effectiveV4Observation) ([]effectiveV4Word, error) {
	if observed == nil {
		return nil, afPolicy
	}
	family, e := decodeAttributes(l.Raw[26])
	if e != nil {
		return nil, e
	}
	attrs, e := decodeAttributes(family[2])
	if e != nil || len(attrs[1]) != 132 {
		return nil, afShape
	}
	local := [33]int32{}
	for n := range local {
		local[n] = int32(binary.NativeEndian.Uint32(attrs[1][n*4:]))
	}
	out := make([]effectiveV4Word, 33)
	for n, name := range ownedTUNIPv4Words {
		w := effectiveV4Word{Index: n, Name: name, Local: local[n], All: observed.all[n], Default: observed.defaults[n], AllObserved: observed.observed[n], Formula: "unresolved-consumer-semantics"}
		// inetdevice.h macros and route.c/arp.c explicit ORCONF consumers.
		switch n {
		case 0, 11, 13:
			w.Formula = "LOCAL"
		case 1, 8, 9, 31, 32:
			w.Formula = "ALL&&LOCAL"
		case 2, 4, 5, 6, 10, 12, 14, 15, 19, 22, 23, 24, 25, 28, 29, 30:
			w.Formula = "ALL||LOCAL"
		case 7, 17, 18, 20, 21:
			w.Formula = "MAX(ALL,LOCAL)"
		case 3:
			w.Formula = "LOCAL_FORWARD?ALL&&LOCAL:ALL||LOCAL"
		case 16:
			w.Formula = "unresolved-IGMP-version-plus-timer-state"
		case 26, 27:
			w.Formula = "LOCAL-ms-but-jiffies-and-IGMP-state-unresolved"
		}
		if w.Formula == "LOCAL" {
			w.Value = w.Local
			w.Computed = true
		} else if w.AllObserved {
			switch w.Formula {
			case "ALL&&LOCAL":
				w.Value = cBool(w.All != 0 && w.Local != 0)
				w.Computed = true
			case "ALL||LOCAL":
				w.Value = cBool(w.All != 0 || w.Local != 0)
				w.Computed = true
			case "MAX(ALL,LOCAL)":
				w.Value = max(w.All, w.Local)
				w.Computed = true
			case "LOCAL_FORWARD?ALL&&LOCAL:ALL||LOCAL":
				if local[0] != 0 {
					w.Value = cBool(w.All != 0 && w.Local != 0)
				} else {
					w.Value = cBool(w.All != 0 || w.Local != 0)
				}
				w.Computed = true
			}
		}
		out[n] = w
	}
	return out, nil
}
