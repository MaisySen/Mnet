//go:build mnet_enrollment_development

package osownership

import "reflect"

const ownedAFPolicyIncomplete ownedAFRefusal = "owned-tun-devconf-policy-incomplete"
const ownedAFIPv6Present ownedAFRefusal = "owned-tun-post-disable-ipv6-present"

// Private scoped proof, never caller-supplied bool. Candidate doesn't grant a
// general26 exception: each use revalidates current pass/ledger/exact raw link.
type ownedAFComposeProof struct {
	session *adapterSession
	pass    *afReadPass
	link    LinkRecord
}

func (p *ownedAFComposeProof) matches(b *CapturedBaseline, l LinkRecord, sys SysctlSnapshot, h *Handle) bool {
	if p == nil || p.session == nil || p.pass == nil || p.session.baseline != b || h == nil || p.session.canonical == nil || !sameSessionHandle(*h, *p.session.canonical) || !reflect.DeepEqual(l, p.link) || !reflect.DeepEqual(map[string]string(sys), map[string]string(p.pass.sys)) {
		return false
	}
	_, e := p.session.diagnoseOwnedAF(p.pass)
	return e == nil
}

// Distinct diagnostic container. No exported/raw Inventory conversion, no Ready
// bit and no public fields. Unresolved named policy cannot be ignored to promote.
type ownedTUNDiagnosticInventory struct {
	inventory Inventory
	raw       RawInventory
	policy    ownedTUNPolicyAssessment
	proof     *ownedAFComposeProof
}

func (s *adapterSession) composeOwnedTUNDiagnostic(raw RawInventory, p *afReadPass) (ownedTUNDiagnosticInventory, error) {
	var zero ownedTUNDiagnosticInventory
	if s == nil || p == nil || s.canonical == nil || !reflect.DeepEqual(raw.Links, p.links) {
		return zero, afRequest
	}
	if _, e := p.requestClaim(); e != nil {
		return zero, e
	}
	d, e := s.diagnoseOwnedAF(p)
	if e != nil {
		return zero, e
	}
	// Every post-disable forward phase, including pending address/up/route, must
	// have NO IPv6 addresses/routes. Whole namespace, not only one name/index.
	if s.forwardStep >= 3 {
		for _, a := range raw.Addresses {
			if a.Family == 10 || a.Local.Is6() || a.Address.Is6() {
				return zero, ownedAFIPv6Present
			}
		}
		for _, r := range raw.Routes {
			if r.Family == 10 || r.Dst.Is6() || r.Source.Is6() || r.Gateway.Is6() || r.PreferredSource.Is6() {
				return zero, ownedAFIPv6Present
			}
		}
		if !d.parsed.disabled {
			return zero, afPhase
		}
	}
	l, e := p.link(s.canonical.Link.Index)
	if e != nil {
		return zero, e
	}
	proof := &ownedAFComposeProof{session: s, pass: p, link: l}
	i, e := s.baseline.composeWithOwnedAF(raw, p.sys, s.canonical, s.addressConfirmed, proof)
	if e != nil {
		return zero, e
	}
	policy, e := assessOwnedTUNPolicy(l)
	if e != nil {
		return zero, e
	}
	return ownedTUNDiagnosticInventory{inventory: i, raw: cloneRawExact(raw), policy: policy, proof: proof}, nil
}

// Not readiness: policy is unresolved by design. No branch on an ignorable
// unchecked flag. This is the ONLY active conversion and it always fails until
// each named policy is independently reviewed and implemented.
func (d ownedTUNDiagnosticInventory) activeInventory() (Inventory, error) {
	return Inventory{}, ownedAFPolicyIncomplete
}
