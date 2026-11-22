//go:build mnet_enrollment_development

package osownership

// These name the still-missing source evidence, not a numeric waiver or a
// runtime feature toggle. The current collector does not produce that evidence.
// Common original-custody/disable/address/byte-gate checks remain separately
// visible as prerequisites. Neither those checks nor this table accept a
// running-kernel semantic profile or establish the live environments below.
func endpointPolicyMissingEvidence(spec endpointPolicyConsumerSpec) string {
	base := "running-kernel supported semantic profile not source-bound; "
	if spec.Index >= 0 && spec.Index < len(ownedTUNIPv4Words) && spec.Name == ownedTUNIPv4Words[spec.Index] {
		switch spec.Index {
		case 0, 1, 31:
			return base + "complete unicast/multicast/broadcast forwarding branches and current packet hooks; zero LOCAL.forwarding alone is not no transit"
		case 3, 4, 5, 6:
			return base + "complete redirect/forwarding consumer and no post-injection ICMP/header rewrite proof"
		case 7:
			return base + "current rp_filter enum, reverse FIB/default rules and actual mark/secpath/hook environment"
		case 8:
			return base + "IHL5 and quoted-header gate plus no post-injection option/header rewrite proof"
		case 9:
			return base + "BOOTP helper/sysctl-only token search is a lead, not complete generated/generic consumer or kernel-build proof"
		case 11:
			return base + "route-tag consumers and actual tc/TCX/cgroup classifier state; pfifo_fast alone is not absence proof"
		case 13:
			return base + "full arp_fwd_proxy call graph including deferred/proxy paths, actual NOARP ancestry and medium comparison relevance"
		case 14, 15:
			return base + "source-bound non-key XFRM counts/defaults and their complete consumer/lifetime relevance must be verified; decoder/collector source is not current execution or supported semantics"
		case 16, 26, 27:
			return base + "current IGMP/multicast state, timers and time units or rigorous construction proof; byte rejection does not suppress kernel generation"
		case 22:
			return base + "complete local-address inventory and exact peer reverse-FIB/source acceptance context"
		case 23:
			return base + "actual TUN allocation mark0 and no netfilter/tc/TCX/cgroup post-allocation mangle environment"
		case 25:
			return base + "exact nonloopback byte gate plus no post-injection source/destination rewrite proof"
		case 29:
			return base + "actual raw-TUN skb allocation packet-type path and current ingress/tc/TCX/cgroup rewriting context"
		}
	}
	switch spec.Group {
	case "diagnostic-unverified":
		return base + "kernel diagnostics/ALL-local logging exposure is not suppressed or accepted by supplier existence"
	case "noarp-candidate":
		return base + "complete named ARP consumer/lifecycle proof, not proximity to a NOARP receive/send gate"
	case "carrier-candidate":
		return base + "named route/neighbor event and carrier lifecycle, including revoke-before-cleanup and current route flags"
	case "single-address-candidate":
		return base + "complete address-promotion/lifetime consumer proof"
	case "neighbor-notify":
		return base + "ALL OR LOCAL notify, UP/CHANGEADDR/DAD and no-address lifetime; read-side bytegate cannot suppress NA generation"
	case "neighbor-carrier-eviction":
		return base + "ALL AND LOCAL eviction and NETDEV_CHANGE/carrier-down neighbor lifecycle"
	case "multicast-lifecycle":
		return base + "current multicast/MLD state and timers or rigorous fresh-namespace/device construction proof; read-side rejection is not emission suppression"
	case "emitter-omission":
		return base + "pinned omission is shape only, not secret initialization evidence; no secret read is authorized or performed"
	case "forwarding", "route-selection", "retained-mtu", "ra-router", "address-dad", "local-disable", "neighbor-input-output", "extension-policy":
		return base + "complete named IPv6 consumer/UP/lifecycle and kernel-emission relevance proof beyond common disable/graph/byte-gate prerequisites"
	default:
		return base + "current XFRM default/policy/state, netfilter/hook/mark, tc/TCX/cgroup packet-type/classifier, multicast or exact FIB operands as named by the consumer"
	}
}
