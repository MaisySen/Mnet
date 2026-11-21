# S38 unused pure packet admission and support matrix, UNRUN

No compile/vet/test/race/mutant, FD/traffic/device or OS effects. No caller wires
this helper to TUN or transport. S36 independent155 green remains prior evidence.
S36 blanket ASSERT hardening decisions removed from deployable source. Named word
assessment remains unresolved, effective-policy/custody prerequisites unchanged.
No readiness conversion is enabled by this packet parser.

## Framing and shared MTU contract

Existing Plan MTU=1280 is the selected common inner MTU, not a new guessed value.
Constructor requires BOTH peer and local readbacks1280, IFF_TUN raw IP, NO_PI,
NO_VNET_HDR and no checksum/GSO offload. Inputs remain claims until bound to exact
original FD/config source, not permission/proof. Reject absent framing/unequal
MTU. No TAP Ethernet, PI header, virtio/GSO metadata or partial-checksum packets.
Actual future FD reader needs read buffer large enough to detect >MTU, never
silently truncate a packet to1280. Transport must carry one inner packet/fragment
without changing it, plus its own framing/encryption overhead outside this MTU.

## Source grounding before encoded rules

https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
TUN get_user checks NO_PI and VNET_HDR separately, chooses IP protocol from version
when NO_PI and resets MAC header. Source creation asks TUN|NO_PI|EXCL, not VNET.
Offloads are explicitly excluded by support contract; not inferred from bare bytes.

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/ip_fragment.c
ip_frag_queue extracts8-byte offset, validates fragment end/empty/overlap state;
receiving kernel handles queues/timeouts/reassembly and errors. Pure admission
passes each TCP/UDP fragment unchanged after own IPv4 checksum/endpoint/protocol/
length/offset bounds. No userspace reassembly, flow table or inferred L4 checksum.
Nonlast payload must multiple8, nonempty, reconstructed payload end<=65515. Reject
reserved flag and contradictory DF+fragment. Out-of-order accepted, kernel resolves
unknown overlaps/corruption. A large UDP datagram fragmented by sender is not
silently discarded for lacking a full first-fragment transport checksum.

https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/icmp.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/icmp.c
UAPI defines type/code values. icmp_unreach handles destination unreachable incl
FRAG_NEEDED and quoted protocol, time exceeded and parameter problem. ICMP receive
validates checksum; kernel/lower transport owns final relevance checks. This helper
also validates unfragmented ICMP checksum before admitting bounded quoted control.

## Exact v1 support matrix

| Packet | Admission |
| --- | --- |
| IPv4 TCP/UDP, exact own→peer or peer→own | supported, length/checksum/header/MTU checked |
| TCP/UDP IPv4 fragments | supported as-is, original exact endpoints/protocol and fragment bounds; receiving kernel reassembles/checks L4 |
| UDP IPv4 checksum0 | permitted by IPv4 semantics, nonzero must validate on complete datagram |
| Complete TCP/nonzero UDP checksum | pseudoheader+payload checksum verified; no offload exemption |
| ICMP echo request/reply | unfragmented, code0, checksum valid |
| ICMP type3 codes0/1/2/3/4/9/10/13 | unfragmented bounded quote, preserves routing/protocol/port/admin errors+PMTU |
| ICMP type11 codes0/1 | TTL/reassembly timeout control with bounded quote |
| ICMP type12 codes0/1/2 | parameter control with in-bounds pointer |
| ICMP redirect/router discovery/source quench/IGMP | refuse, never route-learning or membership across mesh |
| fragmented ICMP | v1 unsupported: cannot statelessly filter types in later fragments; ordinary TCP/UDP fragmentation NOT restricted |
| IPv4 options | explicit v1 unsupported, IHL must5; no hidden source-routing/option rewriting |
| IPv6 or nonpeer/multicast/broadcast/loopback | refuse; same before TUN injection and peer send |

ICMP error quote min28 bytes, bounded by containing<=1280 packet; quoted IPv4
header checksum/version/IHL/declared total and first-fragment offset are checked.
Quoted endpoints must reverse the outer selected own/peer direction; quote protocol
TCP/UDP or echo only, never recursive ICMP errors/IGMP/redirect. Truncated original
payload is normal ICMP, so quote declared length may exceed present quote bytes;
may not be smaller. No port/transaction authentication claimed from bounded quote;
receiving kernel matches sockets/protocol-specific relevance. Outer source must
still be selected peer, not an arbitrary third router. Underlay router errors are
handled by host/transport path, not forged into authenticated inner peer identity.
PMTU FRAG_NEEDED preserves zero legacy MTU; nonzero68..1280, never increases mesh
MTU. ICMP checksum validates entire control before reading quote. This allows
needed error delivery rather than dropping all ICMP and claiming TCP works.

## Checksum and authority ownership

Kernel sender creates valid full transport checksums under the required no-offload
FD contract; helper verifies complete packets defensively. For fragments final
transport checksum belongs to receiving kernel after reassembly; fragment-local
IPv4 checksum always verified. Admission doesn't prove peer honesty or bypass the
kernel checksum. No protocol mutation/reassembly. Caller separately enforces actual
monotonic authority, pair binding, FD custody/namespace and source authenticity
before every real action. Policy construction is not an authorization grant.

## Bounded fake drain

Pure drainBatch accepts at most64 input packet slices, emits only immutable admitted
copies, counts seen/accepted/dropped/bytes and finite typed refusal counters. No
payload logs, goroutines, queues, callbacks or FD reads. Oversize packets rejected
before copying; accepted copies bounded64*1280. Real transport backpressure,
rate/CPU budget, counters persistence and continuously draining local IGMP without
starving traffic are future caller obligations, not claimed solved here.

## UNRUN vectors and residual product gates

Vectors: fragmented4008-byte UDP out-of-order spans preserved; options/checksum/
length/peer/IPv6/IGMP/redirect/bad alignment/DF/reserved/overflow/TTL/UDP length
refusals; explicit ICMP PMTU0/1200, allowed errors, wrong/truncated/options/recursive
quotes and forbidden controls; complete TCP/nonzero UDP checksum tests; bounded
drain/drop counts; unproven framing refusal. These are source fixtures, not runtime
results or arbitrary-app kernel proof. Independent reviewer source gate required.

Creation ARPHRD_NONE/POINTOPOINT/NOARP/full vectors, effective ALL/routing context,
real supplier path/symlink/ENOENT/sender/truncation, common MTU readbacks, offload
proof and installed host route/overlap/firewall/XFRM/app-path remain unresolved.
Namespace model alone isn't plug-and-play. No globals/write path/live release.
