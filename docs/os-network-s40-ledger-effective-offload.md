# S40 ledger contract, same-pass ALL diagnostics and offload source decision

Source-only, UNRUN. S39 independent review reported 166 green inert tests. No
compile/test/vet/race/live reads/ioctl/device/packet effects executed by author.
No real supplier or Kernel adapter is claimed implemented. No execution release.

## Changes in this source slice

`transactionLedgerAdapter` replaces concrete `*inertAdapterProtocol` assertions in
both owner eligibility and creation-custody binding. It extends Kernel with one
private stable-session accessor. A future real Kernel adapter can implement the
same ledger contract without pretending to be the fake protocol. Alternate
in-memory adapter tests exercise exact-session acceptance and wrong-session
refusal. The interface itself grants no permission and implements no effects.
Original transaction owner is now retained by the session at creation-custody
binding, not assigned to whichever transaction first asks for packet policy.
A second transaction cannot replace that owner even while the ledger is phase1.
The creation record remains one-shot, fully frozen, owned by that session.

New `readEffective` profile collects the original raw link dump and required
sysctl witnesses in ONE reader invocation/pass, with immutable numeric operands
and normal dump sequence/sender/shape provenance. This is sequential observation,
NOT an atomic kernel snapshot or proof nothing changed between reads. Existing
legacy `read` passes cannot return effective diagnostics. New/failed collection
invalidates earlier passes, including early cancellation/invalid configuration.

Available 32 IPv4 ALL and DEFAULT sysctl leaves are explicit names from v6.17
`devinet_sysctl.devinet_vars`, not automatic lowercasing of UAPI words. Examples:
ARPFILTER is arp_filter, SRC_VMARK is src_valid_mark, NOXFRM is disable_xfrm,
NOPOLICY is disable_policy. BOOTP_RELAY has no entry in that table; no fake
bootp_relay pathname or absent-means-zero inference is added. Canonical signed
32-bit decimal parsing preserves -1 and refuses overflow/alternate syntax.
Complete 33-word owned-link vector remains raw and its signed values are retained.

The diagnostic computes the exact source macro expressions where operands exist:

- LOCAL: forwarding, tag, medium_id.
- AND: mc_forwarding, accept_source_route, bc_forwarding, arp_evict_nocarrier.
- MAX: rp_filter, arp_announce, arp_ignore, arp_accept, arp_notify.
- OR: proxy_arp, secure_redirects, send_redirects, shared_media, log_martians,
  arp_filter, disable_xfrm, disable_policy, promote_secondaries, accept_local,
  src_valid_mark, proxy_arp_pvlan, route_localnet, ignore_routes_with_linkdown,
  drop_unicast_in_l2_multicast, drop_gratuitous_arp.
- accept_redirects: LOCAL forwarding selects AND, otherwise OR.

C boolean results are normalized to 0/1, MAX stays signed. DEFAULT is recorded
separately and never used as ALL or proof of actual creation inheritance. This
computes 29 word expressions, not an allow/safe decision. BOOTP, IGMP version and
both IGMP timer effects remain explicitly uncomputed. IGMP version depends on
v1/v2 seen timers as well as the forced version knobs; report interval also uses
jiffies conversion/minimum1. No whole effective-IGMP claim from configuration.

IPv6 full60-word semantics are NOT generalized to the IPv4 macros. Existing
per-link disable/full-vector and ALL/DEFAULT forwarding/force_forwarding witnesses
remain prerequisites. No claim that these are all IPv6 consumer operands. Strict
AF policy and complete inventory conversion remain blocked. Effective diagnostics
are not yet bound into packet custody samples, so no sample can gain readiness
from their independent availability. A real supplier must collect and bind them
in its original-resource pass before any eventual active path.

## Source-grounded offload decision

Pinned sources inspected, all upstream Linux v6.17, not a claim the user's kernel
is that version:

1. https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
2. https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/if_tun.h
3. https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/dev.c
4. https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/features.c
5. https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/netdev_features.h
6. https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/inetdevice.h
7. https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/devinet.c
8. https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/route.c
9. https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/arp.c
10. https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/igmp.c
11. https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/ip_input.c

### Fresh original TUN setup

`alloc_netdev_mqs` allocates zeroed private storage via kvzalloc. Fresh tun's
`set_features` therefore starts at zero. `tun_net_init` initially advertises
TUN_USER_FEATURES in hardware/features, but `register_netdevice` invokes
`__netdev_update_features` before publishing the complete link. Its
`ndo_fix_features=tun_net_fix_features` returns the requested features intersected
with `tun->set_features` for the TUN_USER_FEATURES subset. Fresh zero masks this
subset out of active features. It is incorrect to infer active checksum/GSO
support merely from the `tun_net_init` initial advertised assignment.

TUN_USER_FEATURES is HW_CSUM, TSO, TSO6, TSO_ECN, GSO_UDP_L4,
GSO_UDP_TUNNEL, GSO_UDP_TUNNEL_CSUM. It is NOT all netdev features. Scatter/gather,
fragment lists, software GSO/GRO flags can remain; requiring all netdev bits zero
would reject ordinary fresh devices without establishing a useful contract.

`set_offload(0)` sets tun->set_features=0, clears the TUN_USER_FEATURES wanted bits,
and invokes netdev_update_features. For the pinned untouched exclusive fresh
creation it is a redundant explicit transition, not shown necessary to obtain
plain complete datagrams. It would be a new effect-bearing ioctl, so NONE ADDED.
S39's mandatory successful-zero-call history remains a conservative diagnostic
requirement that current real creation cannot satisfy, NOT an instruction to make
the ioctl. A follow-up reviewed model can select fresh-exclusive untouched history
plus active feature observation instead. Do not quietly relax it on this research
alone or call the history an observable readback.

TUNGETFEATURES only reports valid IFF flags. No TUNGETOFFLOAD exists in this UAPI
or ioctl switch. TUNSETNOCSUM is ignored. But ethtool GET FEATURES does expose
hardware, wanted and ACTIVE dev->features bitmaps in `features_prepare_data`.
Thus observable active feature state DOES exist through ethtool, not the imaginary
TUN getter. Wanted/hardware are not active. A future read-only supplier needs exact
netlink family resolution/schema, reply sender/sequence/device index/masks, known
feature index/name mapping, missing/unknown-bit handling and bounded parse before
this can count as an actual observation. No ethtool parser/reader is implemented
here. An older kernel/profile mismatch must block, not reuse v6.17 bit numbers.

### Effect on raw packet bytes

`netif_skb_features` starts with active dev->features and narrows per packet.
`validate_xmit_skb` software-segments when netif_needs_gso and calls checksum help
for CHECKSUM_PARTIAL when the output device cannot supply that checksum. TUN is
the kernel's output netdev for packets read by userspace. With user checksum/GSO
acceptance absent, kernel transmit preparation must complete/segment rather than
hand raw userspace an unmarked partial-checksum/GSO contract. These are packet-
path source facts, not proof every driver/kernel or the product's app path worked.
Per-packet S38 header/length/checksum defenses still apply; kernel/application
fragmentation is not user reassembly or byte rewriting by this tunnel.

In the opposite direction, `tun_get_user` only reads virtio metadata if VNET_HDR
is set. It zero-initializes that metadata otherwise, identifies IPv4 from the raw
version nibble under NO_PI, and injects the supplied IP packet. Our policy checks
complete transport checksums (fragment final check remains receiving kernel).
The offload setting governs output to userspace, not a magic verification of any
bytes userspace injects. Unsupported ICMP/options handling stays unchanged.

### Exact framing mask and identity design

`tun_flags` masks stored flags with TUN_FEATURES | IFF_PERSIST | IFF_TUN | IFF_TAP.
TUN_FEATURES is NO_PI | ONE_QUEUE | VNET_HDR | MULTI_QUEUE | NAPI | NAPI_FRAGS.
`tun_flags_show` emits that value as hex. EXCL is a creation request, not stored
readback. Expected untouched single-queue TUN/noPI mask is 0x1001, not 0x9001.
TUNGETIFF separately ORs NOFILTER when no socket filter, and NOFILTER aliases NO_PI
at 0x1000, so this lone ioctl bit cannot prove framing. `tun_fill_info` separately
emits IFLA_TUN_TYPE, PI, VNET_HDR, PERSIST and MULTI_QUEUE; these are another real
observable framing source, not ethtool offload state.

Real supplier design should prefer original held TUN FD + held namespace identity
and exact same-pass rtnetlink nested framing attributes. If tun_flags sysfs is
used, name/path alone is insufficient: resolve symlink-free under a reviewed
namespace-correct sysfs view and bracket it with original FD/kernel netns and
ifindex/name/alias/creation-handle correlation. Existing root /sys may be mounted
for another namespace; do not assume setns changes that mount's view. Namespace
custody, original FD custody and publication/lifetime checks must remain distinct.
TUNGETDEVNETNS can return the device network namespace FD, whose nsfs/type/device/
inode can be compared with held namespace; reading original FD metadata and raw
number alone cannot prove the linked interface has not moved/replaced. A future
supplier must handle permissions and bounded failure, never fall back to adopting
an existing name or recycled ifindex. No such ioctl is executed or added here.

## Tests and residual gates

UNRUN alternate-ledger interface/foreign ledger/creation-owner tests; same-pass
OR/AND/MAX/redirect-conditional/signed/default-not-ALL vectors; exact sysctl path
spelling/no BOOTP path; legacy/no-profile, stale/failed collection, post-pass sys
substitution, malformed/overflow/signed inputs and raw preservation. Once-only
inert reviewer source gate before execution. Real source adapter, original held
resources, active feature collection, full IPv6/effective policy, authenticated
peer MTU and installed host route/firewall/XFRM/CGNAT overlap/app path remain open.
No default/global sysctl adjustment or host install path is added.
