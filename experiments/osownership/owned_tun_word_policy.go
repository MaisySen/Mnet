//go:build mnet_enrollment_development

package osownership

import "encoding/binary"

// Named source-index policy, not baseline equality or audit bytes deemed safe.
// Product invariant: local owned /32 endpoint + exact peer /32, no transit or
// implicit IPv6. Forwarding checks are necessary but NOT sufficient. Every other
// named word remains a release blocker until its effect on this invariant is
// reviewed. Exact names prevent unlabelled trailing data becoming readiness.
type ownedTUNWordPolicy struct {
	Decision                                    tunWordDecision
	Satisfied                                   bool
	Family                                      int
	Index                                       int
	Name                                        string
	Value                                       uint32
	Rule                                        string
	Resolved                                    bool
	PrerequisitesChecked                        bool
	Category, SourceReferences, MissingEvidence string
}
type ownedTUNPolicyAssessment struct {
	Words      []ownedTUNWordPolicy
	Unresolved int
}

var ownedTUNIPv4Words = [...]string{"IPV4_DEVCONF_FORWARDING", "IPV4_DEVCONF_MC_FORWARDING", "IPV4_DEVCONF_PROXY_ARP", "IPV4_DEVCONF_ACCEPT_REDIRECTS", "IPV4_DEVCONF_SECURE_REDIRECTS", "IPV4_DEVCONF_SEND_REDIRECTS", "IPV4_DEVCONF_SHARED_MEDIA", "IPV4_DEVCONF_RP_FILTER", "IPV4_DEVCONF_ACCEPT_SOURCE_ROUTE", "IPV4_DEVCONF_BOOTP_RELAY", "IPV4_DEVCONF_LOG_MARTIANS", "IPV4_DEVCONF_TAG", "IPV4_DEVCONF_ARPFILTER", "IPV4_DEVCONF_MEDIUM_ID", "IPV4_DEVCONF_NOXFRM", "IPV4_DEVCONF_NOPOLICY", "IPV4_DEVCONF_FORCE_IGMP_VERSION", "IPV4_DEVCONF_ARP_ANNOUNCE", "IPV4_DEVCONF_ARP_IGNORE", "IPV4_DEVCONF_PROMOTE_SECONDARIES", "IPV4_DEVCONF_ARP_ACCEPT", "IPV4_DEVCONF_ARP_NOTIFY", "IPV4_DEVCONF_ACCEPT_LOCAL", "IPV4_DEVCONF_SRC_VMARK", "IPV4_DEVCONF_PROXY_ARP_PVLAN", "IPV4_DEVCONF_ROUTE_LOCALNET", "IPV4_DEVCONF_IGMPV2_UNSOLICITED_REPORT_INTERVAL", "IPV4_DEVCONF_IGMPV3_UNSOLICITED_REPORT_INTERVAL", "IPV4_DEVCONF_IGNORE_ROUTES_WITH_LINKDOWN", "IPV4_DEVCONF_DROP_UNICAST_IN_L2_MULTICAST", "IPV4_DEVCONF_DROP_GRATUITOUS_ARP", "IPV4_DEVCONF_BC_FORWARDING", "IPV4_DEVCONF_ARP_EVICT_NOCARRIER"}
var ownedTUNIPv6Words = [...]string{"DEVCONF_FORWARDING", "DEVCONF_HOPLIMIT", "DEVCONF_MTU6", "DEVCONF_ACCEPT_RA", "DEVCONF_ACCEPT_REDIRECTS", "DEVCONF_AUTOCONF", "DEVCONF_DAD_TRANSMITS", "DEVCONF_RTR_SOLICITS", "DEVCONF_RTR_SOLICIT_INTERVAL", "DEVCONF_RTR_SOLICIT_DELAY", "DEVCONF_USE_TEMPADDR", "DEVCONF_TEMP_VALID_LFT", "DEVCONF_TEMP_PREFERED_LFT", "DEVCONF_REGEN_MAX_RETRY", "DEVCONF_MAX_DESYNC_FACTOR", "DEVCONF_MAX_ADDRESSES", "DEVCONF_FORCE_MLD_VERSION", "DEVCONF_ACCEPT_RA_DEFRTR", "DEVCONF_ACCEPT_RA_PINFO", "DEVCONF_ACCEPT_RA_RTR_PREF", "DEVCONF_RTR_PROBE_INTERVAL", "DEVCONF_ACCEPT_RA_RT_INFO_MAX_PLEN", "DEVCONF_PROXY_NDP", "DEVCONF_OPTIMISTIC_DAD", "DEVCONF_ACCEPT_SOURCE_ROUTE", "DEVCONF_MC_FORWARDING", "DEVCONF_DISABLE_IPV6", "DEVCONF_ACCEPT_DAD", "DEVCONF_FORCE_TLLAO", "DEVCONF_NDISC_NOTIFY", "DEVCONF_MLDV1_UNSOLICITED_REPORT_INTERVAL", "DEVCONF_MLDV2_UNSOLICITED_REPORT_INTERVAL", "DEVCONF_SUPPRESS_FRAG_NDISC", "DEVCONF_ACCEPT_RA_FROM_LOCAL", "DEVCONF_USE_OPTIMISTIC", "DEVCONF_ACCEPT_RA_MTU", "DEVCONF_STABLE_SECRET", "DEVCONF_USE_OIF_ADDRS_ONLY", "DEVCONF_ACCEPT_RA_MIN_HOP_LIMIT", "DEVCONF_IGNORE_ROUTES_WITH_LINKDOWN", "DEVCONF_DROP_UNICAST_IN_L2_MULTICAST", "DEVCONF_DROP_UNSOLICITED_NA", "DEVCONF_KEEP_ADDR_ON_DOWN", "DEVCONF_RTR_SOLICIT_MAX_INTERVAL", "DEVCONF_SEG6_ENABLED", "DEVCONF_SEG6_REQUIRE_HMAC", "DEVCONF_ENHANCED_DAD", "DEVCONF_ADDR_GEN_MODE", "DEVCONF_DISABLE_POLICY", "DEVCONF_ACCEPT_RA_RT_INFO_MIN_PLEN", "DEVCONF_NDISC_TCLASS", "DEVCONF_RPL_SEG_ENABLED", "DEVCONF_RA_DEFRTR_METRIC", "DEVCONF_IOAM6_ENABLED", "DEVCONF_IOAM6_ID", "DEVCONF_IOAM6_ID_WIDE", "DEVCONF_NDISC_EVICT_NOCARRIER", "DEVCONF_ACCEPT_UNTRACKED_NA", "DEVCONF_ACCEPT_RA_MIN_LFT", "DEVCONF_FORCE_FORWARDING"}

func assessOwnedTUNPolicy(l LinkRecord) (ownedTUNPolicyAssessment, error) {
	var out ownedTUNPolicyAssessment
	f, e := decodeAttributes(l.Raw[26])
	if e != nil {
		return out, afShape
	}
	for _, family := range []int{2, 10} {
		body, ok := f[uint16(family)]
		if !ok {
			if family == 10 {
				continue
			}
			return out, afShape
		}
		attrs, e := decodeAttributes(body)
		if e != nil {
			return out, afShape
		}
		names := ownedTUNIPv4Words[:]
		conf := attrs[1]
		if family == 10 {
			names = ownedTUNIPv6Words[:]
			conf = attrs[2]
		}
		if len(conf) != len(names)*4 {
			return out, afShape
		}
		for index, name := range names {
			word := ownedTUNWordPolicy{Family: family, Index: index, Name: name, Value: binary.NativeEndian.Uint32(conf[index*4:]), Rule: "UNRESOLVED-original-custody-effective-policy-endpoint-admission"}
			decision := ownedV4Decision(index)
			if family == 10 {
				decision = ownedV6Decision(index)
			}
			word.Decision = decision
			if family == 2 {
				word.Rule = endpointConsumerDependency(index)
			} else {
				word.Rule = "original idev/disable1 plus no owned IPv6 state and installed supplier IPv6 byte rejection; unresolved until source-bound"
			}
			// Static assertions can be checked now; dormant/unchanged decisions need
			// original creation flags/values and namespace effective policy evidence.
			word.Satisfied = decision.Decision == "assert" && word.Value == decision.Expected
			word.Resolved = false // no effective-all/group/packet-gate policy proof yet
			if !word.Resolved {
				out.Unresolved++
			}
			out.Words = append(out.Words, word)
		}
	}
	return out, nil
}
