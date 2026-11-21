# S41 source/fake-wire supplier slice and BOOTP correction

Source-only and UNRUN. S40 independent 171 green is inert model evidence only.
No compile/test/vet/race, FD/netns/ioctl/ethtool/sysfs reads, helper writes or live
packet effects executed. No actual supplier implements the fake-only wire marker.
No real Kernel adapter, active policy conversion or ready state is supplied here.

## Corrected BOOTP claim

S40's claim that v6.17 omitted bootp_relay was WRONG. An abbreviated displayed
excerpt skipped the line, and the author incorrectly treated that as full-table
absence. The complete 2,908-line pinned devinet.c includes the BOOTP_RELAY RW
entry at line2625 in the full table2602..2656. SHA256:
668cbb8c5d6efedfacb2c9d4a7a924c5e560c9c1ca88af5e8cc47c59aa596270.

Collector now requires all33 ALL and DEFAULT IPv4 leaves, including bootp_relay;
computes30 source expressions, including BOOTP_RELAY's ALL&&LOCAL. Missing BOOTP
operand fails, never becomes zero. Revised tests require the leaf and independently
exercise all1/local1 plus the four direct-OR consumers. S40 green did not validate
that erroneous source claim. Its historical note is superseded on this point.

Source URL:
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/devinet.c
Macro URL:
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/inetdevice.h

## Original-held identity and generation source seam

`originalHeldPacketResources` retains the actual private *ownedCreation,
*fileCustody via creation, and *namespaceCustody references plus exact owner/session
from phase1. It cannot adopt a raw fd number, name/path or recycled ifindex. Its
constructor mints its own private original-object correlation token, not an input
claim of correspondence. It DOES NOT stat/ioctl/read any FD, prove the namespace,
check the actual FD remains open, or authenticate a peer. These observations and
closed-state integration remain absent; retaining pointers is not kernel proof.
This constructor is unused. It is not a ready supplier and does not satisfy the
fake-only wire marker.

`heldPacketLedger` correlates original object, nonzero generation, target and
TUN-device namespace, adapter identity and exact canonical handle. Target/device
namespace equality is mandatory, not assumed from current thread alone. A pure
before-effect transition invalidates on ioctl/dup/export/close intent, namespace
change or custody loss. Failed/ambiguous effects cannot restore lineage; close
intent is terminal. Generation overflow also leaves it uncertain. There is no
unpoison API or name/index reacquisition. Methods do not invoke effects. Existing
close/ioctl helpers are NOT wired to this new invalidation seam, so it is not yet
a guarantee that real transitions have been intercepted. This integration is an
explicit remaining source task, not hidden behind the generation field.

## Same-pass framing and feature decoder

A new test-only wire protocol returns held identity, original-correlated LinkRecord
and a single compact ethtool reply. Reader brackets the link/ALL/feature collection
with exact original held identities. Same-value new object pointer fails. The raw
link must equal the current extended AF pass's link, not substitute same index.
The pass supplies all33/default operands and complete local vector; its current
sequence/profile is rechecked after features. Result preserves raw link and finite
computed words; a new/failed read, changed generation or stale AF pass invalidates
it. Current result also checks AF operands/raw link were not replaced. Collection
is sequential, not atomic, and no packet effects are authorized by the result.

Scoped nested parser expects IFLA_LINKINFO kind1=tun and DATA2 with NLA_F_NESTED.
TUN data fields from v6.17 if_link.h: TYPE3=1, PI4=0, VNET_HDR5=0, PERSIST6=0,
MULTI_QUEUE7=0. Optional OWNER1/GROUP2 must be u32. Counts8/9 or new fields fail as
unsupported single-queue schema. Missing/duplicate/unknown attributes, incorrect
nested/net-byteorder flags, wrong byte widths, truncated/alignment/padding fail.
Generic legacy Compose/linkKind acceptance is NOT widened. Real TUN nested info
still cannot be promoted through that legacy generic path. No name-only sysfs.

Etthool read request encoder emits FEATURES_GET11/version1 with REQUEST flag,
indexed device and COMPACT_BITSETS header flag1. Dynamic family ID is an input,
not guessed. No socket, resolver or request execution exists. Reply decoder binds
kernel sockaddr sender0, no truncation, bounded<=4096 single frame, exact declared
length/family/sequence/local recipient, flags0 (no multipart/notifications), command
FEATURES_GET_REPLY11/version1/reserved0 and exact ifindex/name reply header. Missing,
duplicate, unknown or wrongly nested fields fail. No unsolicited broadcast counts
as a response to the original request.

All HW/WANTED/ACTIVE/NOCHANGE bitsets are mandatory in this pinned reply shape.
Size exactly64. HW has the complete all-ones mask; remaining bitmaps are NOMASK
flag+SIZE+VALUE, not masked or verbose forms. Values are native u32 arrays combined
in low32/high32 order, NOT a native u64 interpretation on big-endian systems.
Reserved positions2/12/13 must be clear. All attributes are copied with length bounds.
ACTIVE, not WANTED or HW, drives only the diagnostic compatibility classifier.
It refuses active checksum1/3/4 and protocol GSO16..35. This is conservative versus
only TUN_USER_FEATURES and includes SCTP/encapsulation/ACCECN. Generic software
GSO11/GRO14/SG0 can be active without failing. This classifier is neither a claim
all remaining hardware offload is impossible nor a real host no-offload proof.

The profile name is explicitly linux-v6.17-netdev-features64. A caller-provided
profile string and family version1 DO NOT identify the running kernel's private
netdev feature numbering. Real discovery/version/source verification or verified
feature name/index strings is STILL REQUIRED. Matching64 alone can collide with
another kernel schema. No live result may rely on this schema until that check is
implemented. Unknown sizes/profile fail, not fall through to remembered bits.

## Requested exact source anchors and semantics

The following are paraphrased source anchors, not assertions about runtime state.

### Four direct-OR fields

- route.c1753/1863/2408 use IN_DEV_ORCONF for NOPOLICY;1879/2650 use it for NOXFRM.
- arp.c798 uses IN_DEV_ORCONF for DROP_GRATUITOUS_ARP when sender equals target.
- ip_input.c422 uses IN_DEV_ORCONF for DROP_UNICAST_IN_L2_MULTICAST.
- inetdevice.h89..98 defines OR against namespace ALL and local conf; this is not
  per-link override, max, or DEFAULT inheritance.

URLs:
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/route.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/arp.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/ip_input.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/inetdevice.h

### TUN zero initialization and fill_info

- dev.c11812 alloc_netdev_mqs uses kvzalloc for device+private storage.
- tun.c187 declares set_features; fresh zero storage has no user-selected bits.
- tun.c926..931 initially advertises hardware/current feature capabilities.
- dev.c11216 registration runs __netdev_update_features;10781..10787 invokes
  ndo_fix_features then core dependency normalization before applying active bits.
- tun.c1097..1102 tun_net_fix_features masks TUN_USER_FEATURES with set_features.
- tun.c2845..2896 set_offload updates selected/wanted bits; zero is explicit reset,
  not shown necessary for the fresh untouched v6.17 path. No zero-offload ioctl added.
- tun.c2304..2340 tun_fill_info emits type and boolean PI/VNET/PERSIST/MULTI_QUEUE;
  queue counts are only present for multiqueue. This is an actual source emitter,
  not inferred from the user's requested TUNSETIFF flags.
- tun.c2645..2656 tun_flags/tun_flags_show uses stored TUN_FEATURES|PERSIST|TUN|TAP;
  EXCL is not in that stored mask. TUNGETIFF NOFILTER/NO_PI alias remains ambiguous.

URLs:
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/dev.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/if_link.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/if_tun.h

### Etthool active schema and actual bit numbering

- features.c40..53 features_prepare_data maps HW=hw_features, WANTED=wanted_features,
  ACTIVE=dev->features, NOCHANGE=never-change; all mask covers NETDEV_FEATURE_COUNT.
- features.c86..113 features_fill_reply supplies HW with all mask; WANTED/ACTIVE/
  NOCHANGE use NULL mask, triggering NOMASK compact encoding.
- bitset.c232..268 emits nested bitset, NOMASK if null mask, SIZE, native-u32 VALUE
  and optional MASK. No network-byteorder flag, no raw native-u64 representation.
- generated UAPI header10 version1;81..88 device header index1/name2/flags3;
  110..118 NOMASK1/SIZE2/BITS3/VALUE4/MASK5;296..304 FEATURES header1/HW2/WANTED3/
  ACTIVE4/NOCHANGE5. User command11 and kernel reply11 follow separate enums.
- netlink.c256..276 emits reply device index/name; family netnsok at1543..1548.
- netdev_features.h14..100 enum has64 feature positions. Checksum IP1/HW3/IPv6 4;
  GSO_SHIFT/TSO16, ECN18, TSO6 20, UDP_TUNNEL26, UDP_TUNNEL_CSUM27, UFO32,
  UDP_L4 33, FRAGLIST34, ACCECN35; GSO_LAST35. Reserved2/12/13. Software GSO11,
  GRO14, SG0. End HSR_DUP63, NETDEV_FEATURE_COUNT64. Python enumeration of the
  actual pinned enum confirmed these, not a remembered or invented bit table.
- common.c16..80 maps named feature indexes, including matching checksum/GSO names.
  A real supplier should cross verified names/indexes rather than assume version1
  fixes this kernel-internal enum forever.

URLs:
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/features.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/bitset.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/ethtool_netlink_generated.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/netlink.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/netdev_features.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/common.c

## Remaining boundaries

UNRUN tests exercise exact nested framing, bitmap/schema/identity/refusal cases,
active not wanted/hardware, generic feature acceptance, protocol GSO rejection,
read-only request shape, before/after object replacement, target/device netns,
all terminal generation events, immutable raw record, failed-pass freshness and
corrected BOOTP/direct-OR operands. No real reads or syscalls are in these tests.

Held-resource reference retention is not a kernel observation. Actual FD/netns
collection, original generation hooks on every effect helper, dynamic ethtool
family/schema/name verification, bounded recvmsg provenance, namespace isolation,
full raw strict Compose, host route/firewall/XFRM/CGNAT app path and authenticated
peer MTU remain incomplete. S39 zero-offload fake requirement is not relaxed here.
Packet policy inputs still cannot turn this source/fake model into authorization,
a live Kernel adapter or readiness. No user application bytes change.
