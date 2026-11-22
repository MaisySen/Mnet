//go:build mnet_enrollment_development

package osownership

// Companion to the numerical ALL/LOCAL formulas in effective_v4_observation.go.
// These describe particular consumers, not an approval for an arbitrary host
// default, link phase, route table, XFRM state or eventual endpoint packet path.
type ipv4ConsumerAssessment struct {
	Word                    effectiveV4Word
	ConsumerRule, PhaseRule string
	Dependencies            string
	Resolved                bool
}

func assessIPv4ConsumerOperands(l LinkRecord, o *effectiveV4Observation) ([]ipv4ConsumerAssessment, error) {
	words, e := effectiveV4Link(l, o)
	if e != nil {
		return nil, e
	}
	out := make([]ipv4ConsumerAssessment, len(words))
	for i, w := range words {
		r := ipv4ConsumerAssessment{Dependencies: endpointConsumerDependency(i), Word: w, ConsumerRule: "formula only; full named consumer/phase impact unresolved", PhaseRule: "creation-DOWN observation never authorizes UP, routes, address changes or packet runtime"}
		switch i {
		case 0:
			r.ConsumerRule = "LOCAL.forwarding route-input transit gate; namespace ip_forward/all write propagation separate; NOARP does not disable IP forwarding"
		case 3:
			r.ConsumerRule = "RX redirects: LOCAL.forwarding selects ALL&&LOCAL versus ALL||LOCAL; route.c first-hop/security checks also apply"
		case 4, 6:
			r.ConsumerRule = "secure_redirects/shared_media participate in IPv4 redirect validation; not substitute for RX redirect gate"
		case 5:
			r.ConsumerRule = "ALL||LOCAL send_redirects participates in route-input redirect eligibility; not ARP and not suppressed by NOARP alone"
		case 7:
			r.ConsumerRule = "MAX(ALL,LOCAL) rp_filter at fib_validate_source; existing secpath sets r=0; FIB/custom-rule/src_valid_mark inputs still matter"
		case 8:
			r.ConsumerRule = "ALL&&LOCAL accept_source_route in IPv4 options receiver; endpoint packet-option admission independently required"
		case 14:
			r.ConsumerRule = "ALL||LOCAL disable_xfrm sets particular allocated route DST_NOXFRM; not evidence no XFRM policy/state"
		case 15:
			r.ConsumerRule = "ALL||LOCAL disable_policy sets IPv4 IPCB NOPOLICY in input/route consumers; xfrm check observes flag"
		case 25:
			r.ConsumerRule = "ALL||LOCAL route_localnet permits loopback-source/destination routing branches; does not establish endpoint safety"
		case 2, 12, 17, 18, 20, 21, 24, 30:
			r.ConsumerRule = "ARP-family behavior has specific ALL/LOCAL formula; arp_send_dst and arp_rcv skip NOARP device. No general exemption for all ARP-adjacent/network policy words"
		case 32:
			r.ConsumerRule = "ARP_EVICT_NOCARRIER: arp_netdev_event carrier-down eviction is NOT bypassed by arp_send_dst/arp_rcv NOARP gates; requires individual carrier/lifetime proof"
		case 16, 26, 27:
			r.ConsumerRule = "IGMP version/timers retain unresolved live state and time-unit dependencies; multicast membership exists while device DOWN"
		}
		if i == 7 && (w.Value < 0 || w.Value > 2) {
			return nil, afPolicy
		}
		out[i] = r
	}
	return out, nil
}
