# S36 clone-isolation correction + substantive data-path policy grounding

S35 once-only reviewer suite152pass/1fail retained at reviewer source. Its final
raw tamper oracle aliased pass.links, so mutation changed both sides and reached
phase refusal instead of expected request mismatch. No local rerun/repair-run.
S36 is source+UNRUN tests. No kernel/device/netlink/sysctl effect or live supplier.

## Clone boundary correction

New pass.linkSnapshot validates current provenance, then structurally deep-copies
link slice/maps/byte arrays. Test builders now use it instead of raw.Links=pass.links.
Tamper test independently saves the pass MTU, changes the returned raw copy and
requires pass unchanged plus exact afRequest. Another test mutates nested bytes
and MTU in one of two snapshots, verifies both original pass and second snapshot
unchanged. Received-baseline mutation test clones source baseline links separately.
No policy change merely to make the mistaken S35 expectation pass.

## Exact source grounding, v6.17 not runner-version claim

Public files downloaded/read for this slice:
https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/arp.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/route.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/fib_frontend.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/ip_options.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/igmp.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/inetdevice.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/ip6_input.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/ip6_output.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/route.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c

TUN initializer explicitly sets ARPHRD_NONE, POINTOPOINT|NOARP|MULTICAST. Existing
open code asks for IFF_TUN, not TAP. However current raw path does NOT yet attest
these exact flags/hardware against original creation custody. Therefore NOARP-
conditioned decisions cannot be marked resolved merely because kind string=tun.

arp_rcv drops before arp_process under NOARP; arp_xmit returns under NOARP;
arp_constructor makes NOARP/point-to-point neighbor entries NUD_NOARP. This gates
ARP request/reply controls, NOT every arp.c side effect. arp_netdev_event still
consults ARP_EVICT_NOCARRIER on NETDEV_CHANGE independent of NOARP, so its selected
policy is ASSERT1, not 'not consulted'. This counterexample is explicitly tested.

IPv6 ingress ipv6_rcv_core drops on disable_ipv6 before normal IPv6 processing.
ip6_output drops before postrouting/finish output; fib6_nh_init refuses disabled
nexthop; ipv6_find_idev refuses disabled creation path before multicast route add.
Together with per-phase whole namespace no IPv6 address/route checks, these justify
not-consulted-IN-DISABLED-DATA-PATH decisions for other named IPv6 knobs, not a
claim that sysctl management/control paths stop consulting them. Preserve exact
creation values and fail changed settings until management effects reviewed.

## Per-word product policy decisions

New owned_tun_policy_decisions.go supplies a decision/source/reason for EVERY
source-named33 IPv4 and60 IPv6 words. These are deliberate conservative product
choices, not Linux default assumptions. No knobs are written by this work.

IPv4 selected ASSERT0: forwarding, multicast/broadcast transit, proxy-independent
redirect receive/send/security/shared trust, source routing, BOOTP relay, route
tag/medium grouping, XFRM/policy bypass, secondary promotion, accept_local,
mark-dependent reverse routing and route_localnet. ASSERT1: strict rp_filter,
ignore_routes_with_linkdown, drop_unicast_in_l2_multicast, arp_evict_nocarrier.
Strict rp_filter must be supported by exact peer reverse route, not guessed safe
for arbitrary multi-route hosts. Conservative refusal is preferable to tuning.

NOARP-conditional not-consulted: proxy_ARP/ARPfilter/announce/ignore/accept/notify/
PVLAN/drop_gratuitous_ARP. These still retain original settings and require exact
creation hardware/flags proof before resolution. LOG_MARTIANS is unchanged logging,
not authority. FORCE_IGMP_VERSION and unsolicited interval controls are unchanged
ONLY IF no multicast input/membership: tun has MULTICAST, so NOARP alone does NOT
gate IGMP. Current membership and packet-admission proof is missing, retained as
an actual prerequisite, not hidden audit acceptance.

IPv6 ASSERT0 forwarding/force_forwarding, ASSERT1 disable, ASSERT0 source-omitted
stable-secret slot. Addrgen remains range/proc-match AND unchanged original mode.
EVERY other named word is conditioned on disabled ingress/output/route gates AND
unchanged from original creation. This includes RA/redirect/autoconf/DAD/source
route/mcast/proxy, lifetime/retry/timing, MTU/hoplimit, segment/RPL/IOAM, policy,
metric/route preferences and neighbor retention. They are dormant for the selected
disabled data path, not universally harmless values or ignored trailing words.

## Why policy still deliberately blocks conversion

inetdevice.h shows effective IPv4 decisions combine per-interface and ALL using
OR,AND,MAX. Per-link assert0 alone isn't sufficient for redirect/XFRM/etc where
ALL may override. Current pass collects only ALL forwarding and a subset of other
leaves, not every effective knob. Original hardware/flag proof, exact unchanged
creation values, effective namespace ALL config, multicast membership/admission,
and management-path review are not implemented. A source decision is not that
proof. Assessment records each decision and whether an ASSERT matches the current
word, but Resolved stays false until those prerequisites exist. All93 decisions
remain conversion blockers. No audit-only shortcut: each has a concrete policy
and missing evidence path. activeInventory still unconditionally refuses.

Next connected slice should implement C (original canonical flags/hardware and
creation-value binding) + A (effective ALL knobs/namespace policy witnesses) under
inert provenance, then B (multicast/packet admission/management policy). No extra
status layers or speculative real supplier activation. A real supplier's source
can be prepared only within the same explicit shape/path/identity contracts;
symlink/ENOENT/sender/truncation/stability and ownership need separate review.

## UNRUN oracles

Deep pass snapshot separation, exact differing-copy afRequest, per-word decision
coverage, unsafe-learning/transit/bypass ASSERT0 choices, restrictive ingress/
carrier ASSERT1, NOARP notifier counterexample and no premature policy promotion.
Existing diagnostic baseline/phases/no-ready tests unchanged semantically. S35
red not rewritten, S36 no result claimed. Review before once-only inert execution.
