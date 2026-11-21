# S44 repair retained red, shared flags and first-step source proposal

S43 independent RED189/1 is retained. Its failure was a new test's direct
namespaceCustody construction, violating the existing structural effect boundary.
No guard weakened. This revision removes that concrete type from the test and
uses the approved pure newHeldPacketLedger plus the ACTUAL fileCustody.attachLineage
method; it verifies attachment/invalidation on the same original file custody
placeholder. No os.File method is invoked by that new test. The test no longer
claims to exercise a real namespace resource constructor. S43 red is not green
until independent once-only retest after source review.

Source-only, author UNRUN compile/test/vet/race/live helpers or actual FD/netns/
ioctl/netlink/sysfs reads/writes. Source proposals below remain under existing
live tags and kernelExecutionReleased=false. No device retry, install, launch,
global policy or app packet/transport byte change. No readiness release.

## Existing-link update flags corrected from source

S43 token request incorrectly used0x605 (REQUEST|ACK|EXCL|CREATE), while existing
mutate used ACK-only0x5 for link updates. That fake request/echo consistency was
NOT proof of correct kernel semantics. Mutation flag selection is now one shared
mutationRequestFlags function used by both existing NetlinkClient.mutate and token
request encoding. NEWLINK16 update=REQUEST|ACK, no CREATE/EXCL. Address/route adds
retain exclusive-create semantics, deletes remain ACK-only. New test compares the
token's actual encoded flags to the existing shared source and independently5.

Pinned Linux v6.17 rtnetlink.c3711..3715 rtnl_changelink rejects EXCL when a link
already exists, and rejects REPLACE. rtnl_newlink dispatch3892..3930 first resolves
positive ifindex and calls changelink for an existing device. If no device exists
and CREATE is absent it returns ENODEV rather than creating or adopting by name.
Thus the first alias/MTU step must not send exclusive-create flags.

Source:
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/rtnetlink.c

## Explicit volatile raw policy before any live mint

S43 whole-byte stats equality could prevent a legitimate readback because packet
counters/time metadata can change independently of setup. The new comparator is
narrow, pinned and applies ONLY to first-setup expected-poststate matching. It
keeps actual raw bytes in diagnostics/post evidence. It masks named fields ONLY
in separate comparison copies, preserving raw attributes/order/headers/padding.
Every key's presence and length must still agree. Stable safety attributes and
unknown attributes are not dropped.

| Attribute ID | Allowed volatility | Stable requirements |
| --- | --- | --- |
| link IFLA_STATS7 | all24 u32 counters | exactly96 bytes, equal presence |
| link IFLA_STATS64 23 | all25 u64 counters | exactly200 bytes, equal presence |
| AF_SPEC26/family10/INET6_STATS3 | counter words1..37 | exactly304 bytes and leading count38 |
| AF_SPEC26/family10/INET6_ICMP6STATS6 | counter words1..6 | exactly56 bytes and leading count7 |
| AF_SPEC26/family10/INET6_CACHEINFO5 | updated timestamp bytes4..7 only | exactly16 bytes; max_reasm_len, reachable_time, retrans_time unchanged |

Everything else stays exact, including core header hardware/flags/index, addresses,
routes, rules, link name/alias/MTU except the declared first-step changes, ALL and
DEFAULT/per-link sys operands, full devconf words, AF flags/token/address-generation
mode, TUN framing, queue/offload evidence and unknown fields. Carrier state33,
carrier changes35, up/down counts47/48, operstate16, qdisc6, master10, link/netns
identity and XDP are NOT silently classified harmless. They remain equality gates.
A changing reachable_time can still refuse, explicitly conservative until its
consumer/state policy is reviewed. This is not a universal liveness claim for all
kernels or changing baseline links.

rtnetlink.c1463..1486 rtnl_fill_stats fills both stats structures from dev_get_stats.
if_link.h9..40 defines24 u32 fields;218..251 defines25 u64 fields. addrconf.c5745..5808
writes pinned leading counts and counter arrays;5815..5830 fills cacheinfo with
max_reasm_len, metadata updated timestamp, reachable_time and retrans_time.
if_link.h1103..1109 fixes cache field layout. No entire cache ignored.

Sources:
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/if_link.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/rtnetlink.c

One fresh fetch of addrconf.c returned429. The already downloaded complete pinned
v6.17 source was inspected for these anchors, not inferred from a failed fetch or
truncated excerpt. No disappearance/unsupported claim from that fetch failure.

## First-step typed source sender proposal

Default routeSocket.Exchange remains exact read-only dump allowlist. Internal
exchangeSource takes a PRIVATE firstLinkSendPermit only through a new first-step
source sender; no generic kind-allowance or caller bool. Permit matches one exact
token/request, original operation set-alias-mtu-exact, shared ACK-only NEWLINK flags,
socket port ID, current token/generation/live owner deadline and unclosed lineage.
It is consumed before send even when send fails, and performs token-bound final
invalidation capturing the actual latest generation. Default caller with nil
permit cannot send NEWLINK/DELLINK/SETLINK or other mutations. Fake callback tests
prove exact permitted first request, no repeat and wrong mutation kinds refused.

sendFirstLinkUpdateSource remains source-only with no caller. It requires original
resource+namespace+ledger/token, holds original fileCustody mutex through verifier,
send and ACK collection, verifies original device before and after, invokes only
the private permit sender and parses the raw narrow exact zero ACK. Fresh socket's
real local port must already be used to build the token; no guessed port binding.
Source does not create/adopt an existing link, set UP, or send later Plan steps.
No real caller constructs the complete flow, and no execution release exists.

No finish/mint occurs in that source sender. Actual feature STRSET+ACTIVE transport
and complete postread pipeline remain missing, so successful ACK alone is not
reported as networking or transition completion. Both finish and send boundary
now recheck owner's original deadline/stop/close eligibility, not only begin.

## Original held FD/device namespace verifier proposal

New verifyOriginalDeviceLockedSource is actual READ-ONLY source code behind live
tags/unreleased guard, not a fake pointer-only identity check. It requires the
original retained fileCustody lock held over observation/use; owned Close cannot
reuse its descriptor during this interval. It checks held namespace/current-thread
custody, Fstat original descriptor is expected TUN character device10,200, CLOEXEC,
TUNGETIFF observed name matches original handle, then TUNGETDEVNETNS on THAT exact
original FD. The returned namespace FD is checked with Fstat/Fstatfs nsfs and
NS_GET_NSTYPE=CLONE_NEWNET, and device/inode must equal pinned target namespace.
Temporary namespace FD is closed. The original TUN FD is not closed or duplicated.
Observed name is correlated with exact later held-handle rtnetlink index/name/alias,
not used alone to adopt a replacement. TUNGETIFF's aliased NO_PI/NOFILTER bit is NOT
framing proof; separate nested TUN parser remains required.

TUNGETDEVNETNS is grounded in tun.c3312..3317: it requires CAP_NET_ADMIN in the
device network's user namespace, then open_related_ns on the TUN device's net.
Missing support/permission errors block. There is no name/sysfs fallback. nsfs
identity relies on held namespace and FD lifetime, not equal pointer serials.
Character-device fstat alone is explicitly NOT unique TUN identity. Retained
original descriptor history+kernel-reported device namespace/name+exact netlink
correlation are all needed. Untracked raw closes/FD transfers or hostile root
still require isolation/containment; source conventions do not defeat them.

Sources:
https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/if_tun.h

This verifier has NOT been compiled or run here and is NOT validated kernel proof.
S43 original verifier was unimplemented; S44 now proposes source implementation,
not actual observation. No unit test names or instantiates effectful namespace
objects to pretend it ran.

## Same-pass real readback proposal, deliberate remaining blocker

readFirstLinkPostSource holds original file lock, obtains kernel-derived original
FD/netns evidence before and after existing collectSourceDiagnostic, compares
namespace/name/held generation, and preserves that evidence plus exact raw/ALL/TUN
parsed diagnostics. It returns originalReadbackProposal, NOT setupPostEvidence.
This prevents real source readback without actual ethtool verified names/ACTIVE
from calling finish. Sequential collection is not atomic; current source strict
stability/equality plus exclusive lineage assumptions remain visible. No
readiness, canonical session advancement, Transaction.Ready or packet path.

Needed before a real mint: fresh dynamically resolved ethtool family/same socket
STRSET+ACTIVE source with verified sender/seq/port/device, full before/after original
identity, captured token generation and exact expected stable fields with the
named volatile policy. Then a separate reviewed conversion may mint post evidence.
There is no current live adapter implementing the fake setup marker. Later Plan
steps' generated raw side effects still require separate source grounding.

## Cancellation and close limits

Close waits on original file mutex while a setup effect/readback holds it. The
route socket uses an approximate250ms context and10ms poll loop, but context
cancellation cannot interrupt every syscall or arbitrary callback. Poll timeout
is NOT a hard wall-clock completion/close guarantee; scheduling, ioctl, filesystem
read and kernel stalls can delay release. The original mutex prevents descriptor
reuse races, not a bounded-time shutdown proof. No asynchronous close of a reused
fd number or unlocked retry is introduced. Strong cancellation would need its own
reviewed interruptible transport/lifetime design, not a claim these bounds solve it.

## Unrun vectors and release boundary

Corrected structural test through shared actual attachment seam, exact shared flags,
first typed send fake permit/once-only/wrong command, explicit volatile counters
positive while stats-size/count/presence/cache-policy/carrier/flags/conf/unknown
changes fail. Existing first-step ACK/setup vectors retained, no tiny rerun loop.
No actual effects/read/helper execution or live compile authorized/taken. Review
source first; independent reviewer owns contained once-only runs. Feature transport,
complete real transition/readback, later setup, strict Compose/full IPv6, peer-MTU
authentication and real installed host route/firewall/XFRM/CGNAT/app path still open.
