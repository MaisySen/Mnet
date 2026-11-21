# S54 fresh DOWN original TUN source derivation and conversion proposal

Source derivation and diagnostic producer only. No postcreate/op1 operational
conversion, ops2-5, Ready wiring or policy-resolved bits. No real read/open/close,
ioctl/device/helper/process/preflight/run/binary. No author tests/race.
S53 independent212 inert/race16/final compile is input evidence only.

## Pinned sources inspected

All Linux v6.17, complete local pinned tun/addrconf/devinet/dev files plus newly
retrieved complete mcast/link_watch. Fetch success/source text, not a search excerpt:

https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/devinet.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/dev.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/link_watch.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/mcast.c

No claim the running target uses this exact kernel/configuration. Observed wire
shapes are independently checked; a version string does not identify runtime
policy or establish original ownership.

## 1. Original exclusive TUN creation and administrative DOWN

In tun.c tun_set_iff around2705..2830, existing names reject IFF_TUN_EXCL, while a
new device allocates the TUN private state, applies tun_net_initialize and calls
register_netdevice. tun_net_initialize1306ff gives TUN ARPHRD_NONE, zero hardware
address/header lengths, MTU1500 and flags POINTOPOINT|NOARP|MULTICAST. It does NOT
set IFF_UP. This differs from TAP Ethernet and must not borrow TAP initialization.

register_netdevice's NETDEV_REGISTER notifier runs before tun_set_iff returns.
The file is attached through tun_net_init/tun_attach during registration; it is
not a link found later by name. Fresh allocation/history and exclusive original
file remain essential: matching observed name/flags cannot prove no prior UP,
control ioctl, duplication/export or queue history.

At the end of tun_set_iff, no IFF_NO_CARRIER request means netif_carrier_on. Carrier
is NOT administrative UP. link_watch.c linkwatch_do_dev runs rfc2863_policy but
only invokes activation/netif_state_change when dev flags contain IFF_UP.
dev.c netif_state_change1574ff itself gates NETDEV_CHANGE and link notification on
IFF_UP. A delayed carrier event for a still-DOWN fresh TUN does not by itself take
the ordinary NETDEV_UP/configuration path. Raw operstate/running/carrier bits are
still observed and not assumed equal to just the driver's three initial flags.

## 2. IPv4 creation defaults and effective policy

On NETDEV_REGISTER, devinet.c inetdev_event1580ff initializes in_device through
inetdev_init266ff. It kzallocs, copies namespace ipv4.devconf_dflt, allocates ARP
parameters, registers per-link sysctls and initializes multicast. ip_mc_up only
runs there if IFF_UP. No address is assigned by inetdev_init. This is not an
assertion that all IPv4 policy values are zero or that the interface is safe.

Compiled ipv4_devconf_dflt83ff has redirects/secure redirects/shared_media,
ACCEPT_SOURCE_ROUTE and ARP_EVICT_NOCARRIER enabled, and nonzero IGMP intervals.
It would be wrong to require an all-zero vector or infer ACCEPT_SOURCE_ROUTE=0
from an abbreviated excerpt. devinet_init_net2750ff chooses live defaults through
net_inherit_devconf: copy current namespace for3, init namespace for0/1, compiled
for2. Therefore even in a new isolated netns, defaults can be inherited policy.
Later default writes can propagate to interfaces with unset per-word state via
devinet_copy_dflt_conf2400ff. Creation-time snapshot alone does not freeze policy.

Current source reads complete ALL/default operands for33 named words. Existing
effectiveV4 formulas are diagnostics; no blanket interpretation that copied local
defaults override ALL OR/AND/MAX consumers. Forwarding0 is required but not enough.
No host-global sysctl rewrite is proposed as a shortcut.

## 3. IPv6 object exists before UP, with source-derived overrides

addrconf_notify NETDEV_REGISTER3648ff creates inet6_dev when MTU>=1280. Fresh TUN
MTU1500 meets this. ipv6_add_dev377ff:

- copies current namespace ipv6.devconf_dflt; if inherited stable_secret is
  initialized, switches addr_gen_mode to STABLE_PRIVACY;
- sets cnf.mtu6=dev.mtu (fresh1500) and ra_mtu=0;
- sets accept_dad=-1 for NOARP or loopback, so this TUN does NOT retain default1;
- sets use_tempaddr=-1 for ARPHRD_NONE (among other tunnel types);
- token is in6addr_any, allocated address list initially empty;
- IF_READY is set only if netif_running AND addrconf_link_ready; fresh DOWN does
  not satisfy that creation condition;
- initializes multicast state/sysctls and joins interface-local and link-local
  all-nodes memberships; all-router membership additionally depends on forwarding.

Membership is IPv6 state even without an IPv6 unicast/link-local address. mcast.c
ipv6_mc_init_dev2810ff initializes delayed works/queues; __ipv6_dev_mc_inc allocates
membership, calls igmp6_group_added and emits multicast notifications. The latter
checks IFF_UP (and reporting flags) before sending reports. It is false to state
DOWN means no IPv6 state, no notifier side effects or no multicast objects.
Those memberships are not themselves the same as an IPv6 address-list record.

addrconf_init_net7390ff copies current/init/compiled defaults by inheritance mode,
then applies ipv6_defaults.autoconf/disable_ipv6 and clears default/all stable
secret initialized flags. Configuration can change later. The source default
objects have many nonzero values. Runtime module defaults, inheritance mode,
CONFIG feature branches and concurrent sysctl writers cannot be guessed from
compiled source. The current canonical sysctl reader does NOT yet observe every
IPv6 default word, nor the initialized secret state, so full inheritance-equality
proof for all60 local words is genuinely missing.

Wire emitter ipv6_store_devconf omits STABLE_SECRET slot36, leaving it zero from
output initialization. That zero is NOT proof no secret or stable addressing
policy exists. inet6_fill_ifla6_attrs emits optional RA_MTU only when ra_mtu!=0;
fresh ra_mtu0 normally means attr absent, not attr9 value0. Conf MTU1500 is separate.
The NOARP accept_dad and use_tempaddr -1 values are two's-complement u32 in the raw
vector, not invalid simply because numerically large. Still unresolved as a
phase-specific policy, not changed in code here.

## 4. Can link-local appear before UP or disable?

Reviewed ordinary original-new-DOWN path: NETDEV_REGISTER only allocates inet6_dev;
it does NOT call addrconf_init_auto_addrs. Carrier-on while IFF_UP stays clear is
UP-gated in linkwatch and netif_state_change. Thus source supports the conditional
claim that this fresh, never-UP, unmodified path does not generate link-local
through those normal events before a later UP. It does NOT support a universal
claim about every DOWN interface or any observed name/index lacking history.

addrconf_notify NETDEV_UP/CHANGE3680ff checks disable_ipv6, link readiness/qdisc,
sets IF_READY then calls addrconf_init_auto_addrs. addrconf_dev_config3454ff supports
ARPHRD_NONE and changes EUI64 mode to RANDOM because NONE has no EUI support, then
calls addrconf_addr_gen. Therefore later UP with IPv6 enabled can create link-local
and DAD/route state, even though creation itself did not. A later disable operation
must precede UP and must have complete postcondition, not rely on default EUI64,
NOARP accept_dad=-1 or administrative DOWN as permanent IPv6 safety.

External explicit address addition, sysctl handlers that trigger address work,
prior UP/control history, driver/kernel changes or unmodeled asynchronous path
can break the conditional profile. The exact postcreate contract must OBSERVE
absence of all unexpected addresses/routes and original history, not merely cite
source. Absence of UP is a stage fact, not packet authority. Full multicast/route
side effects and external-writer exclusion remain review gaps. No assertion that
all IPv6 route tables stay empty based solely on the cited REGISTER branch.

## 5. Named unresolved devconf inventory

Every name below still has Resolved=false in assessOwnedTUNPolicy. This is deliberate.
Observed shape/current value, source initialization origin, effective expression
and phase safety are different facts. Source-derived copying/overrides above do
not discharge every consumer's safety semantics. Lists use zero-based wire indices.

### IPv4 (33 unresolved words)

0. `IPV4_DEVCONF_FORWARDING`: unresolved phase/effective safety.
1. `IPV4_DEVCONF_MC_FORWARDING`: unresolved phase/effective safety.
2. `IPV4_DEVCONF_PROXY_ARP`: unresolved phase/effective safety.
3. `IPV4_DEVCONF_ACCEPT_REDIRECTS`: unresolved phase/effective safety.
4. `IPV4_DEVCONF_SECURE_REDIRECTS`: unresolved phase/effective safety.
5. `IPV4_DEVCONF_SEND_REDIRECTS`: unresolved phase/effective safety.
6. `IPV4_DEVCONF_SHARED_MEDIA`: unresolved phase/effective safety.
7. `IPV4_DEVCONF_RP_FILTER`: unresolved phase/effective safety.
8. `IPV4_DEVCONF_ACCEPT_SOURCE_ROUTE`: unresolved phase/effective safety.
9. `IPV4_DEVCONF_BOOTP_RELAY`: unresolved phase/effective safety.
10. `IPV4_DEVCONF_LOG_MARTIANS`: unresolved phase/effective safety.
11. `IPV4_DEVCONF_TAG`: unresolved phase/effective safety.
12. `IPV4_DEVCONF_ARPFILTER`: unresolved phase/effective safety.
13. `IPV4_DEVCONF_MEDIUM_ID`: unresolved phase/effective safety.
14. `IPV4_DEVCONF_NOXFRM`: unresolved phase/effective safety.
15. `IPV4_DEVCONF_NOPOLICY`: unresolved phase/effective safety.
16. `IPV4_DEVCONF_FORCE_IGMP_VERSION`: unresolved phase/effective safety.
17. `IPV4_DEVCONF_ARP_ANNOUNCE`: unresolved phase/effective safety.
18. `IPV4_DEVCONF_ARP_IGNORE`: unresolved phase/effective safety.
19. `IPV4_DEVCONF_PROMOTE_SECONDARIES`: unresolved phase/effective safety.
20. `IPV4_DEVCONF_ARP_ACCEPT`: unresolved phase/effective safety.
21. `IPV4_DEVCONF_ARP_NOTIFY`: unresolved phase/effective safety.
22. `IPV4_DEVCONF_ACCEPT_LOCAL`: unresolved phase/effective safety.
23. `IPV4_DEVCONF_SRC_VMARK`: unresolved phase/effective safety.
24. `IPV4_DEVCONF_PROXY_ARP_PVLAN`: unresolved phase/effective safety.
25. `IPV4_DEVCONF_ROUTE_LOCALNET`: unresolved phase/effective safety.
26. `IPV4_DEVCONF_IGMPV2_UNSOLICITED_REPORT_INTERVAL`: unresolved phase/effective safety.
27. `IPV4_DEVCONF_IGMPV3_UNSOLICITED_REPORT_INTERVAL`: unresolved phase/effective safety.
28. `IPV4_DEVCONF_IGNORE_ROUTES_WITH_LINKDOWN`: unresolved phase/effective safety.
29. `IPV4_DEVCONF_DROP_UNICAST_IN_L2_MULTICAST`: unresolved phase/effective safety.
30. `IPV4_DEVCONF_DROP_GRATUITOUS_ARP`: unresolved phase/effective safety.
31. `IPV4_DEVCONF_BC_FORWARDING`: unresolved phase/effective safety.
32. `IPV4_DEVCONF_ARP_EVICT_NOCARRIER`: unresolved phase/effective safety.

### IPv6 (60 unresolved words)

0. `DEVCONF_FORWARDING`: unresolved phase/effective safety.
1. `DEVCONF_HOPLIMIT`: unresolved phase/effective safety.
2. `DEVCONF_MTU6`: unresolved phase/effective safety.
3. `DEVCONF_ACCEPT_RA`: unresolved phase/effective safety.
4. `DEVCONF_ACCEPT_REDIRECTS`: unresolved phase/effective safety.
5. `DEVCONF_AUTOCONF`: unresolved phase/effective safety.
6. `DEVCONF_DAD_TRANSMITS`: unresolved phase/effective safety.
7. `DEVCONF_RTR_SOLICITS`: unresolved phase/effective safety.
8. `DEVCONF_RTR_SOLICIT_INTERVAL`: unresolved phase/effective safety.
9. `DEVCONF_RTR_SOLICIT_DELAY`: unresolved phase/effective safety.
10. `DEVCONF_USE_TEMPADDR`: unresolved phase/effective safety.
11. `DEVCONF_TEMP_VALID_LFT`: unresolved phase/effective safety.
12. `DEVCONF_TEMP_PREFERED_LFT`: unresolved phase/effective safety.
13. `DEVCONF_REGEN_MAX_RETRY`: unresolved phase/effective safety.
14. `DEVCONF_MAX_DESYNC_FACTOR`: unresolved phase/effective safety.
15. `DEVCONF_MAX_ADDRESSES`: unresolved phase/effective safety.
16. `DEVCONF_FORCE_MLD_VERSION`: unresolved phase/effective safety.
17. `DEVCONF_ACCEPT_RA_DEFRTR`: unresolved phase/effective safety.
18. `DEVCONF_ACCEPT_RA_PINFO`: unresolved phase/effective safety.
19. `DEVCONF_ACCEPT_RA_RTR_PREF`: unresolved phase/effective safety.
20. `DEVCONF_RTR_PROBE_INTERVAL`: unresolved phase/effective safety.
21. `DEVCONF_ACCEPT_RA_RT_INFO_MAX_PLEN`: unresolved phase/effective safety.
22. `DEVCONF_PROXY_NDP`: unresolved phase/effective safety.
23. `DEVCONF_OPTIMISTIC_DAD`: unresolved phase/effective safety.
24. `DEVCONF_ACCEPT_SOURCE_ROUTE`: unresolved phase/effective safety.
25. `DEVCONF_MC_FORWARDING`: unresolved phase/effective safety.
26. `DEVCONF_DISABLE_IPV6`: unresolved phase/effective safety.
27. `DEVCONF_ACCEPT_DAD`: unresolved phase/effective safety.
28. `DEVCONF_FORCE_TLLAO`: unresolved phase/effective safety.
29. `DEVCONF_NDISC_NOTIFY`: unresolved phase/effective safety.
30. `DEVCONF_MLDV1_UNSOLICITED_REPORT_INTERVAL`: unresolved phase/effective safety.
31. `DEVCONF_MLDV2_UNSOLICITED_REPORT_INTERVAL`: unresolved phase/effective safety.
32. `DEVCONF_SUPPRESS_FRAG_NDISC`: unresolved phase/effective safety.
33. `DEVCONF_ACCEPT_RA_FROM_LOCAL`: unresolved phase/effective safety.
34. `DEVCONF_USE_OPTIMISTIC`: unresolved phase/effective safety.
35. `DEVCONF_ACCEPT_RA_MTU`: unresolved phase/effective safety.
36. `DEVCONF_STABLE_SECRET`: unresolved phase/effective safety.
37. `DEVCONF_USE_OIF_ADDRS_ONLY`: unresolved phase/effective safety.
38. `DEVCONF_ACCEPT_RA_MIN_HOP_LIMIT`: unresolved phase/effective safety.
39. `DEVCONF_IGNORE_ROUTES_WITH_LINKDOWN`: unresolved phase/effective safety.
40. `DEVCONF_DROP_UNICAST_IN_L2_MULTICAST`: unresolved phase/effective safety.
41. `DEVCONF_DROP_UNSOLICITED_NA`: unresolved phase/effective safety.
42. `DEVCONF_KEEP_ADDR_ON_DOWN`: unresolved phase/effective safety.
43. `DEVCONF_RTR_SOLICIT_MAX_INTERVAL`: unresolved phase/effective safety.
44. `DEVCONF_SEG6_ENABLED`: unresolved phase/effective safety.
45. `DEVCONF_SEG6_REQUIRE_HMAC`: unresolved phase/effective safety.
46. `DEVCONF_ENHANCED_DAD`: unresolved phase/effective safety.
47. `DEVCONF_ADDR_GEN_MODE`: unresolved phase/effective safety.
48. `DEVCONF_DISABLE_POLICY`: unresolved phase/effective safety.
49. `DEVCONF_ACCEPT_RA_RT_INFO_MIN_PLEN`: unresolved phase/effective safety.
50. `DEVCONF_NDISC_TCLASS`: unresolved phase/effective safety.
51. `DEVCONF_RPL_SEG_ENABLED`: unresolved phase/effective safety.
52. `DEVCONF_RA_DEFRTR_METRIC`: unresolved phase/effective safety.
53. `DEVCONF_IOAM6_ENABLED`: unresolved phase/effective safety.
54. `DEVCONF_IOAM6_ID`: unresolved phase/effective safety.
55. `DEVCONF_IOAM6_ID_WIDE`: unresolved phase/effective safety.
56. `DEVCONF_NDISC_EVICT_NOCARRIER`: unresolved phase/effective safety.
57. `DEVCONF_ACCEPT_UNTRACKED_NA`: unresolved phase/effective safety.
58. `DEVCONF_ACCEPT_RA_MIN_LFT`: unresolved phase/effective safety.
59. `DEVCONF_FORCE_FORWARDING`: unresolved phase/effective safety.

Cross-cutting unresolved categories: ALL/default inheritance and concurrent writes;
IPv4 source-route/redirect/rp_filter/localnet/XFRM and policy consumers; ARP/IGMP
behavior under NOARP/POINTOPOINT; IPv6 RA/autoconf/DAD/address generation and module
configuration; ND/MLD, forwarding/force_forwarding, routing headers/SEG6/RPL/IOAM,
policy bypass and kept-address effects. Current decision records name prerequisites
but explicitly do not resolve them. No row became an assertion or a magic-zero
requirement in this slice.

## 6. Actual AF producer now held-original bound, still diagnostic only

Scoped operation1 source postcreate Snapshot calls observeCreatedOriginalAF,
which is connected to the existing real route-socket producer, not a fake pass
constructor. It requires the same controller/attempt/custody/session/resident
namespace, phase1 untagged canonical link and no pending op. Original file mutex
spans before kernel original verifier, held source identity, actual route AF dump,
complete raw/sys/effective collection, after verifier and diagnostic assessment.
The pass's link set must match raw under existing pinned comparison policy. Same
original identity/netns/name before/after required. Source creates full named word
assessment but leaves every unresolved bit false and returns typed diagnostic.

The diagnostic retains this real source-owner afReadPass, not just raw parsed AF
bytes. Route sender/seq/recipient/request/lifetime claims remain those of S53.
Resident scope current namespace/TID/active checks and held ledger generation bind
it to original custody, not merely target name. Failure invalidates current pass.
Snapshot STILL returns ownedAFPolicyIncomplete and empty Inventory after creation.
No bindCreationApplicability, phase advancement, postcreate conversion, packet
capability or Ready. The raw-only alternate original diagnostic method is removed
from the live source object, avoiding a misleading parallel provenance path.

No code execution proves these source checks run successfully. Sequential sampling
is not atomic; held file lock protects original Close, not every external kernel
writer. Pinned address/route lifetime drift remains fail-closed with NO new masks.

## 7. Proposed minimal postcreate conversion contract, for review NOT code

1. Require successful exclusive original creation/retention plus no-UP/no-dup/no-
   export/control history, same live owner/session/Plan and current generation.
2. Consume one current held-original real AF pass above; validate exact name/index,
   alias-empty creation identity, original MTU1500/profile, administrative DOWN,
   TUN framing/queue/persistence and actual verified feature names/ACTIVE profile.
3. Require baseline except exactly the new owned original link, no unexpected
   address or route state in ANY table, original kernel netns/name evidence and
   unchanged canonical host policy. Multicast/inet6 object metadata must have a
   reviewed schema, not be called no IPv6 state.
4. Validate source-derived local word inheritance/overrides against COMPLETE
   measured creation defaults where available. Identify exactly which missing
   IPv6 default/secret/config operands need collecting before any success.
   Do not replace unresolved consumer semantics with equality or a phase label.
5. Review a phase-specific named policy: any temporary deferral while DOWN requires
   proven no packet supplier/UP path and bounded owner lifetime through disable,
   never a general resolution of that word for active phases. No such deferral is
   approved or coded here. Effective policy and endpoint gate remain independent.
6. Only after those obligations are resolved, propose one-use phase1 Inventory
   for creation readback, bound exact owner/attempt/generation/sequence. It cannot
   certify alias/op1 or disabled/up/route phases and cannot become Ready directly.

Genuine remaining unknowns prevent conversion NOW: complete live IPv6 default
operand proof; feature/config-dependent raw emitted profile; rigorous residual
state/async event constraints; unresolved named word consumers/phase treatment;
original source runtime feasibility/permissions. No postcreate operational code
should be released merely because the source initialization story is plausible.

## Compile-only and scope

Same explicit package offline preinstalledGo1.26.11 build, CGOoff, development+
process_launch only; no tests/run/binary/preflight_launch. Log delivered. No tests
were added or executed for kernel claims. This derivation cites pinned source and
records its limits; compiler verifies only source types/APIs. No ops2-5, readiness,
ordinary-host reliability, lifetime normalization or shared sessionproof/Mux/
mobile/session/view changes. Existing S19 EACCES not retried/worked around.
