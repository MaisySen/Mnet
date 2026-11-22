//go:build mnet_enrollment_development

package osownership

// These dependency rows are deliberately not booleans supplied by a caller.
// The actual collector preserves each row as unresolved until the named source
// checks AND installed byte gate exist. No inherited numeric word is rewritten.
func endpointConsumerDependency(index int) string {
	switch index {
	case 0, 1, 31:
		return "exact own-local route/default rules plus installed exact-unicast endpoint byte gate; no transit"
	case 2, 12, 13, 17, 18, 20, 21, 24, 30, 32:
		return "original NOARP flags, exact nongateway peer route, and per-I/O flag/route stability"
	case 3, 4, 5, 6:
		return "installed rejection of redirect ICMP and nonlocal destinations; own-local route environment"
	case 7:
		return "rp_filter enum0..2, exact peer reverse FIB/default rules, original mark/secpath environment"
	case 8:
		return "installed IHL5 gate in both directions, including bounded quoted ICMP headers"
	case 9:
		return "exact nonzero unicast endpoints, no transit, and held-source BOOTP consumer review"
	case 10:
		return "diagnostic-only logging behavior; supplier never logs packet payloads"
	case 11:
		return "route tag/classification consumers require exact no-classifier scheduler/rule environment"
	case 14, 15:
		return "explicit current XFRM policy/state environment; disable_xfrm/disable_policy are not absence proofs"
	case 16, 26, 27:
		return "installed multicast/IGMP rejection with bounded draining; do not infer no multicast bookkeeping"
	case 19:
		return "exact one permanent own /32 and no secondary-address mutation during supplier lifetime"
	case 22:
		return "peer source absent from complete local-address inventory; exact unicast reverse FIB"
	case 23:
		return "actual source-injection mark0 plus supported no-mangle/rule environment"
	case 25:
		return "installed exact nonloopback source/destination gate; no host-global route_localnet write"
	case 28:
		return "actual owned link UP/carrier-present and exact route flags0 checked per I/O"
	case 29:
		return "actual raw-TUN allocation packet-type path, not merely absence of Ethernet bytes"
	default:
		return "unsupported named consumer index"
	}
}
