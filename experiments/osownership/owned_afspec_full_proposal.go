//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"strconv"
)

// UNUSED proposal, no Compose or reader caller. Shape evidence is NOT kernel
// identity, request provenance, authenticated ownership or mutation authority.
type ownedAFRefusal string

func (e ownedAFRefusal) Error() string { return string(e) }

const (
	afRequest   ownedAFRefusal = "afspec-request-profile-unverified"
	afShape     ownedAFRefusal = "afspec-unknown-full-shape"
	afPhase     ownedAFRefusal = "afspec-phase-witness-mismatch"
	afDirectory ownedAFRefusal = "afspec-directory-witness-mismatch"
	afPolicy    ownedAFRefusal = "afspec-policy-witness-mismatch"
)

type ownedAFPhase uint8

const (
	afCreated ownedAFPhase = iota + 1
	afTagged
	afDisablePending
	afDisabled
	afCleanup
)

type ownedAFRequestClaim struct {
	Header         [16]byte
	Flags          uint16
	ExtMaskPresent bool
	// Identifier describes shape, NEVER claims exact running kernel version.
	Shape string
}
type ownedAFPhaseClaim struct {
	Phase                    ownedAFPhase
	IPv6ApplicableAtCreation bool
}
type ownedAFParsedProposal struct {
	applicable bool
	disabled   bool
	flags      uint32
	raw        map[uint16][]byte
}

func parseOwnedAFFullProposal(l LinkRecord, sys SysctlSnapshot, request ownedAFRequestClaim, phase ownedAFPhaseClaim) (ownedAFParsedProposal, error) {
	var out ownedAFParsedProposal
	if request.Header != [16]byte{} || request.Flags != 0x301 || request.ExtMaskPresent || request.Shape != "full-v4c33-v6c60-ip38-icmp7" {
		return out, afRequest
	}
	if phase.Phase < afCreated || phase.Phase > afCleanup {
		return out, afPhase
	}
	body, yes := l.Raw[26]
	if !yes {
		return out, afShape
	}
	families, e := decodeAttributes(body)
	if e != nil || len(families) < 1 || len(families) > 2 {
		return out, afShape
	}
	v4, yes := families[2]
	if !yes {
		return out, afShape
	}
	for k := range families {
		if k != 2 && k != 10 {
			return out, afShape
		}
	}
	a4, e := decodeAttributes(v4)
	if e != nil || len(a4) != 1 || len(a4[1]) != 132 {
		return out, afShape
	}
	if binary.NativeEndian.Uint32(a4[1][:4]) != 0 || sys["/proc/sys/net/ipv4/conf/"+l.Name+"/forwarding"] != "0" {
		return out, afPolicy
	}
	v6, applicable := families[10]
	out.applicable = applicable
	if applicable != phase.IPv6ApplicableAtCreation {
		return out, afPhase
	}
	if e = validateIPv6Directory(l.Name, applicable, sys); e != nil {
		return out, afDirectory
	}
	if !applicable {
		// Current Plan has no reviewed not-applicable disable transition.
		if phase.Phase == afDisabled {
			return out, afPhase
		}
		out.raw = cloneRawAttrs(l.Raw)
		return out, nil
	}
	a6, e := decodeAttributes(v6)
	if e != nil || len(a6) < 7 || len(a6) > 8 {
		return out, afShape
	}
	lengths := map[uint16]int{1: 4, 2: 240, 3: 304, 5: 16, 6: 56, 7: 16, 8: 1}
	for k, n := range lengths {
		if len(a6[k]) != n {
			return out, afShape
		}
	}
	for k, b := range a6 {
		if _, known := lengths[k]; !known && (k != 9 || len(b) != 4 || binary.NativeEndian.Uint32(b) == 0) {
			return out, afShape
		}
	}
	// Full emitter stores vector-count witness in first u64 stats word.
	if binary.NativeEndian.Uint64(a6[3][:8]) != 38 || binary.NativeEndian.Uint64(a6[6][:8]) != 7 {
		return out, afShape
	}
	flags := binary.NativeEndian.Uint32(a6[1])
	if flags & ^uint32(0x800000f0) != 0 {
		return out, afShape
	}
	out.flags = flags
	word := func(index int) uint32 { return binary.NativeEndian.Uint32(a6[2][index*4 : index*4+4]) }
	p := "/proc/sys/net/ipv6/conf/" + l.Name + "/"
	// Count60 last word is FORCE_FORWARDING, not ignorable trailing bytes.
	if word(0) != 0 || word(59) != 0 || word(26) > 1 || word(47) > 3 || sys[p+"forwarding"] != "0" || sys[p+"force_forwarding"] != "0" || sys[p+"disable_ipv6"] != strconv.FormatUint(uint64(word(26)), 10) || sys[p+"addr_gen_mode"] != strconv.FormatUint(uint64(word(47)), 10) || uint32(a6[8][0]) != word(47) {
		return out, afPolicy
	}
	if word(36) != 0 {
		return out, afShape
	} // source zero-fill, slot omitted
	out.disabled = word(26) == 1
	if phase.Phase == afDisabled && (!out.disabled || flags != 0) {
		return out, afPhase
	}
	// Flags-zero after disable is deliberately conservative proposed policy,
	// not a universal property proved for every kernel/configuration.
	out.raw = cloneRawAttrs(l.Raw)
	return out, nil
}
