//go:build mnet_enrollment_development

package osownership

// UNUSED decisions for source review. No writes and no active conversion.
// These values are product choices justified by exact endpoint/no-transit policy,
// NOT assertions that Linux defaults have them or that matching is sufficient.
type tunWordDecision struct {
	Decision string
	Expected uint32
	Source   string
	Reason   string
}

func ownedV4Decision(index int) tunWordDecision {
	zero := func(source, reason string) tunWordDecision { return tunWordDecision{"assert", 0, source, reason} }
	switch index {
	case 0:
		return zero("inetdevice.h:IN_DEV_FORWARD;route.c:ip_route_input_slow", "no transit")
	case 1:
		return zero("inetdevice.h:IN_DEV_MFORWARD;route.c:ip_route_input_mc", "no multicast transit")
	case 2, 12, 17, 18, 20, 21, 24, 30:
		return tunWordDecision{"not-consulted-if-original-NOARP-custody", 0, "arp.c:arp_rcv/arp_constructor/arp_solicit", "require original ARPHRD_NONE+POINTOPOINT+NOARP for ARP paths"}
	case 3:
		return zero("inetdevice.h:IN_DEV_RX_REDIRECTS;route.c:__ip_do_redirect", "never learn peer routes via redirects; effective all OR must also be zero")
	case 4:
		return zero("inetdevice.h:IN_DEV_SEC_REDIRECTS;route.c:__ip_do_redirect", "redirect acceptance already zero; pin restrictive no learned path")
	case 5:
		return zero("inetdevice.h:IN_DEV_TX_REDIRECTS;route.c:ip_rt_send_redirect", "never emit redirects")
	case 6:
		return zero("inetdevice.h:IN_DEV_SHARED_MEDIA;route.c:__ip_do_redirect", "do not widen redirect trust")
	case 7:
		return tunWordDecision{"assert", 1, "fib_frontend.c:fib_validate_source/__fib_validate_source", "strict reverse route to exact peer; effective max/all witness required"}
	case 8:
		return zero("ip_options.c:ip_options_rcv_srr;inetdevice.h:IN_DEV_SOURCE_ROUTE", "no source routed packets")
	case 9:
		return zero("inetdevice.h:IN_DEV_BOOTP_RELAY", "no relay")
	case 10:
		return tunWordDecision{"unchanged-from-original-creation", 0, "route.c:martian_source", "logging only, preserve diagnostic behavior without using it as authorization"}
	case 11:
		return zero("fib_frontend.c:fib_validate_source;inetdevice.h:IN_DEV_IDTAG", "no route class tag side effects")
	case 13:
		return zero("inetdevice.h:IN_DEV_MEDIUM_ID", "no shared medium grouping needed for point-to-point")
	case 14, 15:
		return zero("route.c:IN_DEV_ORCONF(NOXFRM/NOPOLICY)", "do not bypass kernel security policy; effective all OR witness required")
	case 16, 26, 27:
		return tunWordDecision{"unchanged-with-no-multicast-admission", 0, "igmp.c:igmp_ifc_event/igmp_heard_query", "timers/version only dormant if no groups or multicast input; membership and packet gate not implemented"}
	case 19:
		return zero("devinet.c:inet_del_ifa;inetdevice.h:IN_DEV_PROMOTE_SECONDARIES", "single exact /32; no secondary promotion")
	case 22:
		return zero("fib_frontend.c:__fib_validate_source", "never accept peer packet claiming local source")
	case 23:
		return zero("fib_frontend.c:__fib_validate_source;IN_DEV_SRC_VMARK", "no mark-dependent reverse path")
	case 25:
		return zero("route.c:ip_route_input_slow;IN_DEV_NET_ROUTE_LOCALNET", "never route loopback across tunnel")
	case 28:
		return tunWordDecision{"assert", 1, "inetdevice.h:IN_DEV_IGNORE_ROUTES_WITH_LINKDOWN", "do not retain usable path when link down"}
	case 29:
		return tunWordDecision{"assert", 1, "route.c:ip_route_input_slow", "drop unicast carried as multicast rather than expand ingress"}
	case 32:
		return tunWordDecision{"assert", 1, "arp.c:arp_netdev_event NETDEV_CHANGE", "evict stale neighbor state on carrier loss; NOARP does NOT bypass notifier"}
	case 31:
		return zero("route.c:ip_route_input_slow;IN_DEV_BFORWARD", "no broadcast transit")
	}
	return tunWordDecision{Decision: "unresolved", Reason: "unknown IPv4 word"}
}
func ownedV6Decision(index int) tunWordDecision {
	switch index {
	case 0, 59:
		return tunWordDecision{"assert", 0, "addrconf.c:ipv6_store_devconf;ip6_input.c:ipv6_rcv", "no forwarding or force_forwarding"}
	case 26:
		return tunWordDecision{"assert", 1, "ip6_input.c:ipv6_rcv_core;ip6_output.c:ip6_output;route.c:fib6_nh_init", "disabled required on ingress/output/route paths"}
	case 36:
		return tunWordDecision{"assert", 0, "addrconf.c:ipv6_store_devconf", "source omits zero-filled stable secret slot; not a secret retrieval"}
	case 47:
		return tunWordDecision{"phase-witness-range-and-unchanged", 0, "addrconf.c:addrconf_addr_gen;ipv6_store_devconf", "no silent addrgen change; matching proc and0..3 already parsed"}
	default:
		return tunWordDecision{"not-consulted-in-disabled-data-path-and-unchanged", 0, "ip6_input.c:ipv6_rcv_core;ip6_output.c:ip6_output;route.c:fib6_nh_init;addrconf.c:ipv6_add_addr/ipv6_find_idev", "disabled gate before IPv6 forwarding/options/autoconf paths; no IPv6 routes/addresses; preserve every source-named word from creation"}
	}
}
