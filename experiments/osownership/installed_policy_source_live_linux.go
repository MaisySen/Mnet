//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/binary"
	"reflect"
)

// Private common-prerequisite pass, not semantic acceptance or caller flags.
// It is minted only from the actual collector/original ancestry and installed
// framing. The supported running-kernel profile/live environment is still
// missing and every named row records that gap. No test constructs this pass.
type installedEndpointPolicyPass struct {
	evidence *sourceRuntimeEvidence
	link     LinkRecord
	v4       [33]uint32
	v6       [60]uint32
}

func (e *sourceRuntimeEvidence) installedPolicyPassSource(ctx context.Context) (*installedEndpointPolicyPass, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if e == nil || e.resources == nil || e.routed == nil || e.supplier == nil || e.admission == nil {
		return nil, afPhase
	}
	o := e.resources
	if o.owner == nil || o.session == nil || o.session.canonical == nil || o.namespace == nil || o.creation == nil || o.creation.fd == nil || o.creation.attempt == nil || o.ledger == nil || e.offload == nil {
		return nil, afPhase
	}
	r := e.supplier
	c := r.owner
	if c == nil || c.tx != o.owner || c.adapter == nil || !c.adapter.authorized(ctx) || c.lane == nil || !c.lane.ownerThreadCurrent() || r.evidence == nil || r.evidence.resources != o || r.evidence.routed != e.routed || r.frames == nil || e.before == nil || e.after == nil || e.before.resources != o || e.after.resources != o || e.before.identity != e.after.identity || e.after.identity != e.routed.identity || !e.routed.current() {
		return nil, afPhase
	}
	lane, ok := c.lane.(*operation1ResidentSource)
	if !ok || !lane.active || lane.owner != c || lane.ns != o.namespace || lane.channel != e.admission || lane.reactor != r {
		return nil, afPhase
	}
	up := e.routed.ancestry
	if up == nil || up.ancestry == nil || up.ancestry.disabledAncestry == nil {
		return nil, afPhase
	}
	d := up.ancestry.disabledAncestry
	if !d.consumed || d.owner != c.tx || d.session != o.session || d.ledger != o.ledger || d.lifetime != lane.disableLife || !d.lifetime.current() || !sameDisableOriginalIdentity(d.identity, e.routed.identity) {
		return nil, afPhase
	}
	if err := e.admission.checkResidentSource(ctx, o.namespace, c.tx.plan.Binding, e.routed.identity.generation); err != nil {
		return nil, err
	}
	own, err := endpointIPv4(c.tx.plan.Binding.OwnIP)
	if err != nil {
		return nil, err
	}
	peer, err := endpointIPv4(c.tx.plan.Binding.PeerIP)
	if err != nil {
		return nil, err
	}
	r.frames.mu.Lock()
	installed := r.frames.admission && r.frames.flowStarted && !r.frames.stopped && r.frames.generation == e.routed.identity.generation && r.frames.policy != nil && r.frames.policy.mtu == MTU && r.frames.policy.own == own && r.frames.policy.peer == peer
	r.frames.mu.Unlock()
	if !installed || !exactSetupPostMatches(e.diagnostic.raw, e.routed.raw) || !reflect.DeepEqual(e.diagnostic.sys, e.routed.sys) || e.framing != (tunNestedFraming{typ: 1}) || e.observedMTU != MTU || !e.offload.matches(o.creation.attempt, o.creation.fd, o.namespace.identity, c.tx.plan.Binding.Name) {
		return nil, afPhase
	}
	if err = ValidateDefaultRules(e.diagnostic.raw.Rules); err != nil {
		return nil, err
	}
	p := &installedEndpointPolicyPass{evidence: e}
	found := false
	for _, l := range e.diagnostic.raw.Links {
		if l.Index == o.session.canonical.Link.Index {
			if found || l.Flags != 0x110d1 || l.Hardware != 65534 || l.Name != c.tx.plan.Binding.Name || l.Alias != c.tx.plan.Binding.Alias || l.MTU != MTU {
				return nil, afPhase
			}
			p.link = l
			found = true
		}
	}
	if !found || e.diagnostic.sys["/proc/sys/net/ipv6/conf/"+p.link.Name+"/disable_ipv6"] != "1" {
		return nil, afPhase
	}
	families, err := decodeAttributes(p.link.Raw[26])
	if err != nil {
		return nil, err
	}
	a4, err := decodeAttributes(families[2])
	if err != nil || len(a4[1]) != 33*4 {
		return nil, afShape
	}
	a6, err := decodeAttributes(families[10])
	if err != nil || len(a6[2]) != 60*4 {
		return nil, afShape
	}
	for i := range p.v4 {
		p.v4[i] = binary.NativeEndian.Uint32(a4[1][4*i:])
	}
	for i := range p.v6 {
		p.v6[i] = binary.NativeEndian.Uint32(a6[2][4*i:])
	}
	if p.v6[26] != 1 || p.v6[2] != MTU || p.v6[36] != 0 {
		return nil, afPolicy
	}
	// Exact source-generated post-route graph already admits only the permanent
	// own IPv4 /32, its local route and the direct peer /32. Explicit AF_INET6
	// address/route absence is checked separately; idev/MC/timers are not absent.
	if !endpointIPv6AddressRouteAbsent(e.diagnostic.raw) {
		return nil, afPolicy
	}
	if len(e.diagnostic.raw.Addresses) != 1 {
		return nil, afPhase
	}
	for _, a := range e.diagnostic.raw.Addresses {
		if err = validateOwnedAddress(o.session.canonical, a); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// All other IPv4 prerequisites remain unsupported, including no-mangle,
// mark/pkt_type, reverse FIB, route tag/classification and current XFRM state.
// No candidate below is semantic acceptance, including NOARP/IPv6 categories.
func (p *installedEndpointPolicyPass) v4Rule(index int) (string, bool) {
	if p == nil || p.evidence == nil || p.evidence.resources == nil || p.evidence.after == nil || index < 0 || index >= len(p.v4) {
		return "invalid-source-pass", false
	}
	switch index {
	case 2, 12, 17, 18, 20, 21, 24, 30:
		return "arp.c: arp_send_dst/arp_rcv NOARP gates; same current original NOARP link and direct-route graph; no claim of absent neighbor bookkeeping", p.link.Flags&0x80 != 0
	case 14, 15:
		return "original-bound non-key XFRM metadata: two matching zero SAD/six SPD counts and IN/FWD/OUT ACCEPT observations; consumer/profile/lifetime relevance is still not a semantic witness", p.evidence.xfrm.matches(p.evidence.resources, p.evidence.after.identity)
	case 32:
		return "arp.c arp_evict_nocarrier is carrier-down neighbor eviction, NOT bypassed by an ARP receive exemption; original carrier present per I/O, cleanup occurs only after lifetime revoke", p.link.Flags == 0x110d1
	case 10:
		return "diagnostic-unverified: kernel log_martians is not suppressed by supplier existence; effective ALL/local diagnostics and their exposure remain unsupported", false
	case 19:
		return "devinet.c promote_secondaries: exactly one permanent own /32, no secondary address, no post-completion controller address mutation; per-I/O graph recheck", len(p.evidence.diagnostic.raw.Addresses) == 1
	case 28:
		return "fib_semantics.c ignore_routes_with_linkdown: exact original UP/RUNNING/LOWER_UP and unchanged direct peer/local routes checked per I/O", p.link.Flags == 0x110d1
	case 13:
		return "medium_id proxy-medium comparison in arp.c remains individually unsupported here; not admitted by a general ARP-adjacent label", false
	default:
		return endpointConsumerDependency(index) + "; required current packet-hook/mark/FIB/XFRM operand not yet source-supported", false
	}
}
func (p *installedEndpointPolicyPass) v6Rule(index int) (string, bool) {
	if p == nil || p.evidence == nil || p.evidence.admission == nil || !endpointIPv6AddressRouteAbsent(p.evidence.diagnostic.raw) || index < 0 || index >= len(p.v6) {
		return "invalid-source-pass", false
	}
	// Common prerequisites only: retained idev/MTU, consumed disable ancestry,
	// no AF_INET6 addresses/routes and the installed byte gate. These do not
	// establish complete named lifecycle or emission semantics. In particular a
	// read-side byte gate rejects already-generated output, not its generation.
	// Running-kernel/profile and live environment gaps remain on every row.
	disabled := p.v6[26] == 1 && p.v6[2] == MTU && p.evidence.admission.started
	switch index {
	case 2:
		return "addrconf.c retained idev mtu6=1280, never below-minimum unregister or claimed IPv6 absence", disabled && p.v6[2] == MTU
	case 26:
		return "addrconf.c/ip6_input.c/ip6_output.c actual original LOCAL.disable_ipv6=1", disabled && p.v6[26] == 1
	case 36:
		return "addrconf.c omitted zero emitter slot is shape only, NOT initialized-secret evidence; secret state not required by disabled IPv6 endpoint profile and never read", disabled && p.v6[36] == 0
	case 0, 59:
		return "IPv6 forwarding consumers unreachable for admitted traffic: disable input/output plus no IPv6 route and IPv4-only supplier; no ALL/default numeric rewrite", disabled
	case 1, 37, 39:
		return "IPv6 source/route/hop selection has no admitted IPv6 packet/address/route; disable output checked before postrouting", disabled
	case 3, 4, 5, 7, 8, 9, 17, 18, 19, 20, 21, 33, 35, 38, 43, 49, 52, 58:
		return "RA/router/redirect configuration: disable receive and addrconf NETDEV_UP guard, no IPv6 addresses/routes (retained idev/bookkeeping is not absence) and no supplier IPv6 ingress; no generic ALL formula", disabled
	case 6, 10, 11, 12, 13, 14, 15, 23, 27, 34, 42, 46, 47:
		return "address/DAD/temp/generation: original disable-before-address-and-UP ancestry plus current no IPv6 addresses/routes; no claim NOARP alone suppresses every effect", disabled
	case 16, 25, 30, 31:
		return "IPv6 multicast/MLD kernel emissions/lifecycle remain unproved: a read-side gate drops observed output but does not suppress generation or show no bookkeeping/timers", false
	case 29:
		return "NDISC_NOTIFY uses ALL OR LOCAL on UP/CHANGEADDR and DAD completion; no-address lifetime is a prerequisite but read-side byte rejection does not suppress kernel NA emission; current ALL/lifecycle semantic evidence missing", false
	case 56:
		return "NDISC_EVICT_NOCARRIER uses ALL AND LOCAL on NETDEV_CHANGE and carrier-down neighbor eviction, not the receive disable gate; current carrier alone is not complete neighbor-lifecycle proof", false
	case 22, 28, 32, 40, 41, 50, 57:
		return "ND/L2-neighbor consumers: disabled receive/address/neighbor-add paths and IPv4-only original supplier; no general NOARP exemption", disabled
	case 24, 44, 45, 48, 51, 53, 54, 55:
		return "IPv6 extension/IOAM/policy consumers have no admitted IPv6 input/output; this does NOT prove current XFRM absence for IPv4 rows", disabled
	default:
		return "unsupported IPv6 source consumer", false
	}
}
func (e *sourceRuntimeEvidence) assessInstalledRowsSource(ctx context.Context) error {
	p, err := e.installedPolicyPassSource(ctx)
	if err != nil {
		return err
	}
	if len(e.policy.Words) != 93 || len(e.consumer) != 33 {
		return afShape
	}
	result := ownedTUNPolicyAssessment{Words: append([]ownedTUNWordPolicy(nil), e.policy.Words...)}
	seen4 := [33]bool{}
	seen6 := [60]bool{}
	for n := range result.Words {
		w := &result.Words[n]
		var rule string
		var supported bool
		var spec endpointPolicyConsumerSpec
		switch w.Family {
		case 2:
			if w.Index < 0 || w.Index >= 33 || seen4[w.Index] || w.Name != ownedTUNIPv4Words[w.Index] || w.Value != p.v4[w.Index] {
				return afShape
			}
			seen4[w.Index] = true
			spec = endpointIPv4ConsumerCatalog[w.Index]
			rule, supported = p.v4Rule(w.Index)
		case 10:
			if w.Index < 0 || w.Index >= 60 || seen6[w.Index] || w.Name != ownedTUNIPv6Words[w.Index] || w.Value != p.v6[w.Index] {
				return afShape
			}
			seen6[w.Index] = true
			spec = endpointIPv6ConsumerCatalog[w.Index]
			rule, supported = p.v6Rule(w.Index)
		default:
			return afShape
		}
		if spec.Index != w.Index || spec.Name != w.Name {
			return afShape
		}
		w.Rule = "candidate prerequisites: " + rule
		w.Category, w.SourceReferences = spec.Group, spec.References
		w.PrerequisitesChecked = supported
		w.MissingEvidence = endpointPolicyMissingEvidence(spec)
		// This producer currently emits common prerequisites, not a semantic
		// witness. Human-readable reason text must never grant readiness.
		w.Resolved = false
		w.Satisfied = false
		if !w.Resolved {
			result.Unresolved++
		}
	}
	consumers := append([]ipv4ConsumerAssessment(nil), e.consumer...)
	for i := range consumers {
		w := &consumers[i]
		if w.Word.Index != i || w.Word.Name != ownedTUNIPv4Words[i] || uint32(w.Word.Local) != p.v4[i] || !w.Word.AllObserved {
			return afShape
		}
		spec := endpointIPv4ConsumerCatalog[i]
		if spec.Index != i || spec.Name != w.Word.Name {
			return afShape
		}
		w.ConsumerRule, w.PrerequisitesChecked = p.v4Rule(i)
		w.Category, w.SourceReferences = spec.Group, spec.References
		w.MissingEvidence = endpointPolicyMissingEvidence(spec)
		// No source semantic witness is produced by this candidate collector.
		w.Resolved = false
		w.PhaseRule = "candidate common original post-route/two-phase gate checks only; no source-bound supported semantic profile or complete live packet environment; no Ready"
	}
	e.policy = result
	e.consumer = consumers
	return nil
}
