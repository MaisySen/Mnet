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

// S36 assert vector is suspended. These records name actual prerequisites,
// never inherited defaults to harden or a global-sysctl adjustment path.
func ownedV4Decision(index int) tunWordDecision {
	if index < 0 || index >= len(ownedTUNIPv4Words) {
		return tunWordDecision{Decision: "unresolved", Reason: "unknown word"}
	}
	return tunWordDecision{"endpoint-admission-and-effective-host-policy-unchanged", 0, "inetdevice.h;route.c;fib_frontend.c;arp.c;igmp.c", "original NOARP/point-to-point custody, exact peer route, effective ALL observation and bounded endpoint admission prerequisites"}
}
func ownedV6Decision(index int) tunWordDecision {
	if index < 0 || index >= len(ownedTUNIPv6Words) {
		return tunWordDecision{Decision: "unresolved", Reason: "unknown word"}
	}
	return tunWordDecision{"disabled-original-link-and-unchanged-host-policy", 0, "ip6_input.c;ip6_output.c;addrconf.c;route.c:fib6_nh_init", "original owned-link disable witness, no IPv6 state/packets, original vectors and authority prerequisites"}
}
