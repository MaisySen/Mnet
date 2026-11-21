//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Pinned addrconf.c procname allowlist, ZERO secret-valued leaves. Slot36 is
// deliberately absent. This is observation, not a promise that every CONFIG
// dependent leaf exists on a running kernel. Missing operands fail closed.
var ipv6PolicyLeaves = [60]string{
	"forwarding", "hop_limit", "mtu", "accept_ra", "accept_redirects", "autoconf", "dad_transmits", "router_solicitations", "router_solicitation_interval", "router_solicitation_delay", "use_tempaddr", "temp_valid_lft", "temp_prefered_lft", "regen_max_retry", "max_desync_factor", "max_addresses", "force_mld_version", "accept_ra_defrtr", "accept_ra_pinfo", "accept_ra_rtr_pref", "router_probe_interval", "accept_ra_rt_info_max_plen", "proxy_ndp", "optimistic_dad", "accept_source_route", "mc_forwarding", "disable_ipv6", "accept_dad", "force_tllao", "ndisc_notify", "mldv1_unsolicited_report_interval", "mldv2_unsolicited_report_interval", "suppress_frag_ndisc", "accept_ra_from_local", "use_optimistic", "accept_ra_mtu", "", "use_oif_addrs_only", "accept_ra_min_hop_limit", "ignore_routes_with_linkdown", "drop_unicast_in_l2_multicast", "drop_unsolicited_na", "keep_addr_on_down", "router_solicitation_max_interval", "seg6_enabled", "seg6_require_hmac", "enhanced_dad", "addr_gen_mode", "disable_policy", "accept_ra_rt_info_min_plen", "ndisc_tclass", "rpl_seg_enabled", "ra_defrtr_metric", "ioam6_enabled", "ioam6_id", "ioam6_id_wide", "ndisc_evict_nocarrier", "accept_untracked_na", "accept_ra_min_lft", "force_forwarding",
}

// Additional current source consumers not represented in the60-word emitter.
var ipv6ExtraPolicyLeaves = [...]string{"regen_min_advance", "ra_honor_pio_life", "ra_honor_pio_pflag"}

func ipv6PolicyPaths() []string {
	var paths []string
	for _, scope := range []string{"all", "default"} {
		for _, leaf := range ipv6PolicyLeaves {
			if leaf != "" {
				paths = append(paths, "/proc/sys/net/ipv6/conf/"+scope+"/"+leaf)
			}
		}
		for _, leaf := range ipv6ExtraPolicyLeaves {
			paths = append(paths, "/proc/sys/net/ipv6/conf/"+scope+"/"+leaf)
		}
	}
	return paths
}
func permittedIPv6OperandPath(path string) bool {
	// Defense in depth before ANY source Openat. Never probe the secret-valued
	// file even to infer initialized from success/error or obtain a prefix.
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.Contains(part, "secret") {
			return false
		}
	}
	return true
}

type ipv6PolicyOperands struct {
	all, defaults [60]int64
	extras        map[string]int64
}

func parseIPv6PolicyOperands(sys SysctlSnapshot) (*ipv6PolicyOperands, error) {
	out := &ipv6PolicyOperands{extras: map[string]int64{}}
	for _, path := range ipv6PolicyPaths() {
		raw, ok := sys[path]
		if !ok {
			return nil, fmt.Errorf("missing IPv6 operand %s (CONFIG/visibility unresolved): %w", path, afPolicy)
		}
		n, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || strconv.FormatInt(n, 10) != raw || n < -2147483648 || n > 2147483647 && !ipv6UnsignedOperand(path) {
			return nil, afPolicy
		}
		if ipv6UnsignedOperand(path) && (n < 0 || n > 4294967295) {
			return nil, afPolicy
		}
		if !ipv6OperandHandlerRange(path, n) {
			return nil, afPolicy
		}
		out.extras[path] = n
	}
	for i, leaf := range ipv6PolicyLeaves {
		if leaf == "" {
			continue
		}
		out.all[i] = out.extras["/proc/sys/net/ipv6/conf/all/"+leaf]
		out.defaults[i] = out.extras["/proc/sys/net/ipv6/conf/default/"+leaf]
	}
	return out, nil
}

// NONSECRET uncertainty: no supported read-only initialized boolean found in
// pinned UAPI. Mode2 is compatible with an initialized secret but does not
// reveal its presence/absence unambiguously; NONE/EUI/RANDOM do not prove absence.
const stableInitializationUnresolved = "no nonsecret initialized-state observation in reviewed UAPI; secret-valued file never probed"

type ipv6ConsumerAssessment struct {
	Index                                                         int
	Name, Leaf, ConsumerRule, PhaseRule, Config, ProcUnit, AFUnit string
	All, Default, Local, Effective                                int64
	LocalObserved, Computed                                       bool
	Resolved                                                      bool
}

func ipv6ConsumerRules(o *ipv6PolicyOperands) []ipv6ConsumerAssessment {
	if o == nil {
		return nil
	}
	out := make([]ipv6ConsumerAssessment, 60)
	for i, name := range ownedTUNIPv6Words {
		r := ipv6ConsumerAssessment{Index: i, Name: name, Leaf: ipv6PolicyLeaves[i], All: o.all[i], Default: o.defaults[i], ConsumerRule: "UNRESOLVED consumer path/config and local operand; default is initialization not generic ALL formula", PhaseRule: "DOWN observation cannot authorize UP/packet flow; named phase review still required"}
		switch i {
		case 0:
			r.ConsumerRule = "forward input/output uses ALL.forwarding OR ingress.LOCAL.force_forwarding; LOCAL.forwarding separately controls RA/ND/router behavior; all-write propagates"
		case 6:
			r.ConsumerRule = "LOCAL.dad_transmits initializes per-address probe count; NOARP bypass is a different later DAD branch"
		case 11, 12, 13, 14:
			r.ConsumerRule = "LOCAL temporary lifetime/regeneration parameters consumed only within temp-address state machine; use_tempaddr and device-type gate separate"
		case 15:
			r.ConsumerRule = "LOCAL.max_addresses bounds address addition paths; does not prove absence of already-created addresses"
		case 16:
			r.ConsumerRule = "ALL.force_mld_version when nonzero, else LOCAL; live MLD state/timers still required"
		case 17, 18, 19, 21, 33, 35, 38, 49, 52, 58:
			r.ConsumerRule = "named LOCAL RA subpolicy/limit within ndisc RA consumer; ipv6_accept_ra and packet validation gates are separate; no ALL OR"
		case 28:
			r.ConsumerRule = "LOCAL.force_tllao controls target link-layer address inclusion in ND send path"
		case 29:
			r.ConsumerRule = "ALL.ndisc_notify OR LOCAL.ndisc_notify on DAD completion and NETDEV_UP/CHANGEADDR; no global NOARP exemption asserted"
		case 32:
			r.ConsumerRule = "LOCAL.suppress_frag_ndisc rejects fragmented ND before normal ND receive processing"
		case 37:
			r.ConsumerRule = "LOCAL.use_oif_addrs_only participates in source-address selection; does not prohibit implicit IPv6 effects"
		case 41:
			r.ConsumerRule = "LOCAL.drop_unsolicited_na rejection precedes accept_untracked_na processing"
		case 46:
			r.ConsumerRule = "LOCAL.enhanced_dad OR ALL.enhanced_dad generates DAD nonce; actual DAD bypass/state remains separate"
		case 50:
			r.ConsumerRule = "LOCAL.ndisc_tclass selected by ND send path; absent idev chooses zero"
		case 56:
			r.ConsumerRule = "ALL.ndisc_evict_nocarrier AND LOCAL.ndisc_evict_nocarrier for NETDEV_CHANGE carrier-down neighbor eviction"
		case 57:
			r.ConsumerRule = "LOCAL.accept_untracked_na: 0 reject,1 create,2 only same-subnet via ipv6_chk_prefix; unsolicited rejection takes precedence"
		case 3:
			r.ConsumerRule = "LOCAL.forwarding ? LOCAL.accept_ra==2 : LOCAL.accept_ra!=0 (ipv6_accept_ra); not ALL OR"
		case 4:
			r.ConsumerRule = "redirect requires !LOCAL.forwarding && LOCAL.accept_redirects plus first-hop/ND checks"
		case 5:
			r.ConsumerRule = "PIO autoconf requires PIO.autoconf && LOCAL.autoconf plus lifetime/P-flag and RA gates"
		case 10:
			r.ConsumerRule = "temporary creation requires signed LOCAL.use_tempaddr>0; NONE-device initialization overrides to -1"
		case 24:
			r.ConsumerRule = "MIN(ALL,LOCAL) signed accept_source_route; RH types4/3 dispatch separate SEG6/RPL rules; no generic nonzero acceptance"
		case 26:
			r.ConsumerRule = "LOCAL.disable_ipv6 checked at add/config/receive; ALL writes propagate and update default, not generic OR read"
		case 27:
			r.ConsumerRule = "NOARP/LOOPBACK bypass DAD; otherwise ALL<1&&LOCAL<1 bypass; failure disable trigger ALL>1||LOCAL>1 with further conditions"
		case 36:
			r.ConsumerRule = stableInitializationUnresolved
			r.PhaseRule = "omitted zero emitter slot is NOT secret-state proof; conversion blocked when initialized state required"
		case 44:
			r.ConsumerRule = "MIN(ALL.seg6_enabled,LOCAL.seg6_enabled) nonzero at SRH ingress; HMAC separate CONFIG gate"
		case 45:
			r.ConsumerRule = "LOCAL.seg6_require_hmac controls SRH HMAC validation (-1,0,1 distinct), CONFIG dependent"
		case 47:
			r.ConsumerRule = "LOCAL addr_gen_mode; initialization may override from secret flag, ARPHRD_NONE EUI64->RANDOM on configuration; sysctl write may configure while DOWN"
		case 48:
			r.ConsumerRule = "ALL.disable_policy OR LOCAL.disable_policy bypasses particular IPv6 policy consumers; does not prove no XFRM state"
		case 51:
			r.ConsumerRule = "MIN(ALL.rpl_seg_enabled,LOCAL.rpl_seg_enabled) nonzero at RPL routing-header ingress"
		case 53:
			r.ConsumerRule = "LOCAL.ioam6_enabled gates ingress Hop-by-Hop IOAM processing; disabled ignores, not equivalent to dropping IPv6"
		case 59:
			r.ConsumerRule = "ALL.forwarding OR ingress.LOCAL.force_forwarding at forward gate; default is new-interface template, all-write has propagation"
		}
		switch i {
		case 3, 4, 17, 18, 19, 21, 33, 35, 38, 49, 52, 58:
			r.PhaseRule = "fresh-DOWN no supplied packet is not a future gate: RA/redirect admission, neighbor updates and UP/traffic remain unproved"
		case 5, 10, 11, 12, 13, 14, 15, 47:
			r.PhaseRule = "default-copy creation differs from later address configuration; addr_gen_mode/disable sysctl writes can trigger configuration even while device DOWN"
		case 6, 27, 46:
			r.PhaseRule = "fresh NOARP bypasses DAD begin, not all address/ND behavior; flags and exact transition custody must remain proved"
		case 24, 44, 45, 48, 51, 53:
			r.PhaseRule = "packet-consumer operand only; extension-header/XFRM/endpoint admission and original DOWN-to-UP transition not authorized"
		}
		// Formula facts above are not completed endpoint/phase safety proof.
		r.Config = ipv6OperandConfig(i)
		r.ProcUnit, r.AFUnit = ipv6OperandUnits(i)
		r.Resolved = false
		out[i] = r
	}
	return out
}

func ipv6UnsignedOperand(path string) bool {
	for _, leaf := range []string{"addr_gen_mode", "force_forwarding", "ra_defrtr_metric", "ioam6_id", "ioam6_id_wide", "ioam6_enabled", "ndisc_evict_nocarrier", "ra_honor_pio_life", "ra_honor_pio_pflag"} {
		if strings.HasSuffix(path, "/"+leaf) {
			return true
		}
	}
	return false
}
func ipv6OperandConfig(index int) string {
	switch index {
	case 19, 20:
		return "CONFIG_IPV6_ROUTER_PREF"
	case 21, 49:
		return "CONFIG_IPV6_ROUTER_PREF && CONFIG_IPV6_ROUTE_INFO"
	case 23, 34:
		return "CONFIG_IPV6_OPTIMISTIC_DAD"
	case 25:
		return "CONFIG_IPV6_MROUTE"
	case 45:
		return "CONFIG_IPV6_SEG6_HMAC"
	case 36:
		return "NO PROC READ PERMITTED; emitter omits slot"
	}
	return "IPv6 sysctl table present; runtime source support not inferred"
}
func ipv6OperandUnits(index int) (string, string) {
	switch index {
	case 8, 9, 20, 43:
		return "seconds via proc_dointvec_jiffies", "milliseconds via jiffies_to_msecs; no raw equality"
	case 30, 31:
		return "milliseconds via proc_dointvec_ms_jiffies", "milliseconds via jiffies_to_msecs"
	case 36:
		return "NEVER READ", "omitted zero, not initialized boolean"
	}
	return "source integer (signedness per leaf)", "native-endian s32 slot (u32 fields retain bits)"
}

// Computes only reviewed consumer operands, NEVER safety or applicability.
// Creation/default equality is intentionally absent. ALL/default proc integers
// with jiffies handlers are never compared to AF millisecond words here.
func assessIPv6ConsumerOperands(l LinkRecord, o *ipv6PolicyOperands) ([]ipv6ConsumerAssessment, error) {
	out := ipv6ConsumerRules(o)
	if len(out) != 60 {
		return nil, afPolicy
	}
	families, e := decodeAttributes(l.Raw[26])
	if e != nil {
		return nil, afShape
	}
	attrs, e := decodeAttributes(families[10])
	if e != nil {
		return nil, afShape
	}
	conf := attrs[2]
	if len(conf) != 240 {
		return nil, afShape
	}
	local := [60]int64{}
	for i := range local {
		if i == 36 {
			continue
		}
		n := binary.NativeEndian.Uint32(conf[4*i:])
		local[i] = int64(int32(n))
		if i == 52 || i == 54 || i == 55 {
			local[i] = int64(n)
		}
	}
	boolean := func(v bool) int64 {
		if v {
			return 1
		}
		return 0
	}
	for i := range out {
		if i == 36 {
			continue
		}
		r := &out[i]
		r.Local = local[i]
		r.LocalObserved = true
		switch i {
		case 0, 59:
			r.Effective = boolean(o.all[0] != 0 || local[59] != 0)
			r.Computed = true
		case 3:
			r.Effective = boolean((local[0] != 0 && local[3] == 2) || (local[0] == 0 && local[3] != 0))
			r.Computed = true
		case 4:
			r.Effective = boolean(local[0] == 0 && local[4] != 0)
			r.Computed = true
		case 5, 6, 10, 11, 12, 13, 14, 15, 26, 28, 32, 37, 41, 45, 47, 50, 53, 57:
			r.Effective = local[i]
			r.Computed = true
		case 24, 44, 51:
			r.Effective = min(o.all[i], local[i])
			r.Computed = true
		case 16:
			r.Effective = local[i]
			if o.all[i] != 0 {
				r.Effective = o.all[i]
			}
			r.Computed = true
		case 29, 46:
			r.Effective = boolean(o.all[i] != 0 || local[i] != 0)
			r.Computed = true
		case 56:
			r.Effective = boolean(o.all[i] != 0 && local[i] != 0)
			r.Computed = true
		case 48:
			r.Effective = boolean(o.all[i] != 0 || local[i] != 0)
			r.Computed = true
		}
	}
	return out, nil
}

// Narrow bounds that the reviewed unsigned/minmax handlers actually enforce.
// Signed proc_dointvec controls remain signed, not blanket boolean-normalized.
func ipv6OperandHandlerRange(path string, n int64) bool {
	leaf := path[strings.LastIndex(path, "/")+1:]
	switch leaf {
	case "addr_gen_mode":
		return n >= 0 && n <= 3
	case "force_forwarding", "ioam6_enabled", "ndisc_evict_nocarrier", "ra_honor_pio_life", "ra_honor_pio_pflag":
		return n >= 0 && n <= 1
	case "ioam6_id":
		return n >= 0 && n <= 65535
	case "ra_defrtr_metric":
		return n >= 1 && n <= 4294967295
	case "accept_untracked_na":
		return n >= 0 && n <= 2
	case "ndisc_tclass":
		return n >= 0 && n <= 255
	}
	return true
}
