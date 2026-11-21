//go:build mnet_enrollment_development

package osownership

import "strconv"

// These predicates resolve ONLY the named creation-DOWN exclusion, not safety
// after UP, packet readiness or global host policy. Re-evaluate after any phase
// change. Source caller must hold the controller ownership lane and original
// file custody lock. No defaults-equal-local shortcut and no host sysctl writes.
func assessCreationDownPolicy(d ownedSourceDiagnostic) (ownedTUNPolicyAssessment, error) {
	var zero ownedTUNPolicyAssessment
	h := d.held
	if h == nil || h.ledger == nil || h.session == nil || h.owner == nil || !d.lineageUsable || !h.ledger.matches(h.identity) || h.identity.generation != 1 || h.identity.closed || h.identity.uncertain || h.session.forwardStep != 1 || h.session.pending != nil || h.session.canonical == nil || h.session.canonical.TagConfirmed || h.session.closeAttempted || h.session.cleanupMode || h.session.poisoned {
		return zero, afPhase
	}
	if h.session.owner != h.owner || h.owner.stopStarted || h.owner.fdCloseAttempted {
		return zero, afPhase
	}
	h.ledger.mu.Lock()
	pending := h.ledger.setup != nil
	h.ledger.mu.Unlock()
	if pending {
		return zero, afPhase
	}
	handle := *h.session.canonical
	var link *LinkRecord
	for i := range d.raw.Links {
		l := &d.raw.Links[i]
		if l.Index == handle.Link.Index || l.Name == handle.Link.Name {
			if link != nil || l.Index != handle.Link.Index || l.Name != handle.Link.Name || l.Alias != handle.CreationLink.Alias || l.MTU != handle.CreationLink.MTU || l.Hardware != 65534 || l.Flags & ^uint32(0x40|0x10000) != 0x1090 {
				return zero, afPhase
			}
			link = l
		}
	}
	if link == nil {
		return zero, afPhase
	}
	// Creation means no address or attached route of either family. This is an
	// actual full raw-inventory operand, not inferred from DOWN or disable_ipv6.
	for _, a := range d.raw.Addresses {
		if a.Index == link.Index {
			return zero, afPhase
		}
	}
	for _, r := range d.raw.Routes {
		if len(r.Raw[9]) != 0 || len(r.Raw[30]) != 0 {
			return zero, afPhase
		}
		if r.OIF == uint32(link.Index) || r.IIF == uint32(link.Index) {
			return zero, afPhase
		}
	}
	v4, e := parseEffectiveV4(d.sys)
	if e != nil {
		return zero, e
	}
	w4, e := effectiveV4Link(*link, v4)
	if e != nil {
		return zero, e
	}
	v6, e := parseIPv6PolicyOperands(d.sys)
	if e != nil {
		return zero, e
	}
	w6, e := assessIPv6ConsumerOperands(*link, v6)
	if e != nil {
		return zero, e
	}
	// Independently observed proc/local AF agreement for the five existing local
	// witnesses, including unsigned handlers. Missing/uncanonical never means0.
	for _, item := range []struct {
		leaf          string
		family, index int
	}{
		{"forwarding", 4, 0}, {"forwarding", 6, 0}, {"disable_ipv6", 6, 26}, {"addr_gen_mode", 6, 47}, {"force_forwarding", 6, 59},
	} {
		p := "/proc/sys/net/ipv" + strconv.Itoa(item.family) + "/conf/" + link.Name + "/" + item.leaf
		raw, ok := d.sys[p]
		n, e := strconv.ParseInt(raw, 10, 64)
		if !ok || e != nil || strconv.FormatInt(n, 10) != raw {
			return zero, afPolicy
		}
		if item.family == 6 && !ipv6OperandHandlerRange(p, n) {
			return zero, afPolicy
		}
		local := int64(w4[0].Local)
		if item.family == 6 {
			local = w6[item.index].Local
		}
		if n != local {
			return zero, afPolicy
		}
	}
	out, e := assessOwnedTUNPolicy(*link)
	if e != nil {
		return zero, e
	}
	out.Unresolved = 0
	for i := range out.Words {
		w := &out.Words[i]
		ok := false
		rule := "no reviewed creation-DOWN exclusion predicate"
		if w.Family == 2 {
			r := w4[w.Index]
			switch w.Index {
			case 0:
				ok = r.Computed && r.Value == 0
				rule = "LOCAL.forwarding==0 excludes reviewed IPv4 unicast transit gate; multicast/broadcast separate words"
			case 1, 31:
				ok = r.Computed && r.Value == 0
				rule = "ALL&&LOCAL multicast/broadcast forwarding gate false; no default template assumption"
			case 3:
				ok = r.Computed && r.Value == 0
				rule = "LOCAL forwarding-selective ALL/LOCAL RX redirect gate false; no first-hop/FIB assumptions needed for exclusion"
			case 5:
				ok = r.Computed && r.Value == 0
				rule = "ALL||LOCAL send_redirects false, no redirect generation enabled by this word"
			case 8:
				ok = r.Computed && r.Value == 0
				rule = "ALL&&LOCAL source-route permission false before IPv4 source-option processing"
			case 14, 15:
				ok = r.Computed && r.Value == 0
				rule = "ALL||LOCAL bypass flag false; does NOT assert absence or suitability of existing XFRM state/policy"
			case 25:
				ok = r.Computed && r.Value == 0
				rule = "ALL||LOCAL route_localnet false; no loopback-source relaxation from this control"
			case 7:
				rule = "MAX(ALL,LOCAL) observed, but rp_filter has secpath/FIB/custom-rule/src_valid_mark dependencies; no endpoint behavior resolution"
			}
		} else {
			r := w6[w.Index]
			switch w.Index {
			case 0:
				ok = r.Computed && r.Effective == 0 && w6[0].Local == 0
				rule = "ALL.forwarding==0 && LOCAL.force_forwarding==0 excludes forward gate; LOCAL.forwarding==0 separately excludes router-mode behavior"
			case 59:
				ok = r.Computed && r.Effective == 0
				rule = "ALL.forwarding OR LOCAL.force_forwarding false at reviewed forwarding gate"
			case 3:
				ok = r.Computed && r.Effective == 0
				rule = "ipv6_accept_ra false by actual LOCAL forwarding/accept_ra; ND neighbor processing NOT claimed absent"
			case 4:
				ok = r.Computed && r.Effective == 0
				rule = "LOCAL.forwarding OR !LOCAL.accept_redirects excludes rt6_do_redirect acceptance"
			case 5:
				ok = r.Computed && r.Local == 0
				rule = "LOCAL.autoconf==0 excludes PIO-derived address creation; other address generation remains separately unresolved"
			case 10:
				ok = r.Local <= 0
				rule = "signed LOCAL.use_tempaddr<=0 excludes temporary-address creation entry"
			case 24:
				ok = r.Computed && r.Effective < 0
				rule = "MIN signed accept_source_route<0 excludes MIP6 RH2 processing; SEG6/RPL are separate unresolved words"
			case 26:
				ok = r.Local != 0
				rule = "LOCAL.disable_ipv6 true rejects IPv6 input and addrconf_add_dev; not a secret-state or global no-IPv6 proof"
			case 48:
				ok = r.Computed && r.Effective == 0
				rule = "ALL||LOCAL disable_policy false excludes reviewed forward/host-route bypass; existing XFRM policy remains unassessed"
			case 36:
				rule = stableInitializationUnresolved + "; no proved no-supplier lifetime waiver"
			}
		}
		w.Rule = "CREATION-DOWN-ONLY: " + rule
		w.Resolved = ok
		w.Satisfied = ok
		if ok {
			w.Decision.Decision = "creation-down-excluded-effect"
			w.Decision.Reason = w.Rule
		} else {
			out.Unresolved++
		}
	}
	// Conditional result only. Source caller still holds controller/file
	// custody; this value itself is never a durable applicability token.
	if !h.ledger.matches(h.identity) {
		return zero, afPhase
	}
	return out, nil
}
