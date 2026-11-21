# S39 pure creation/custody correlation, source only

S38 independent reviewer reported 160 passing inert tests. That does not prove
real kernel compatibility. S39 is UNRUN, including compile, vet, tests and race.
No device, ioctl, namespace, packet read/write, install or global changes made.
The only supplier implementation is in `_test.go`. No real supplier is introduced.

## Model boundary

Creation is captured once while the canonical session is at phase 1, before any
alias/MTU/address/up transition. The candidate binds exact Transaction pointer,
adapter session and target/session/attempt identity, namespace identity, canonical
Plan and creation Handle, original object pointer and nonzero custody generation.
A raw descriptor integer cannot identify the object. It keeps an immutable full
creation record, including all AF devconf bytes, and matches the already recorded
original creation AF record exactly. No padding/trailing-word normalization.

Initial hardware is ARPHRD_NONE, flags exactly POINTOPOINT|NOARP|MULTICAST, no UP.
Current flags preserve those core flags and allow only UP/RUNNING/LOWER_UP changes.
Multicast remains on the owned link, not silently cleared to hide IGMP behavior.
Raw name/alias/MTU attributes must match decoded fields and the canonical handle.
A fake original successful TUNSETIFF(TUN|NO_PI|EXCL) record and successful
TUNSETOFFLOAD(0) history are required, plus current per-device tun_flags=0x1001.
These are model records, not an attestation, OS proof, or a permission grant.

Every diagnostic batch needs a fresh, single-use supplier pass. A new collection,
including failure, invalidates the previous pass. Current sample must match the
original reader, owner/session/identity/namespace/plan/object/generation/creation
handle. Existing owner clock/deadline, canonical complete ledger, pending/poison/
stop/close checks still run. Distinct fake original objects with the same serial
are not interchangeable. Replaying an old pass or borrowing another reader fails.

Local observed MTU and peer MTU contract must both equal existing 1280, with the
contract bound to both exact Plan public keys. Real authenticated peer negotiation
is still missing: matching fake key strings is correlation, not authentication.
Peer contract is not inferred from a remote packet or an address alone.

The policy output is still a diagnostic packet container. No active conversion,
readiness bit, FD/transport callback, or Kernel implementation is added. Effective
ALL policy and strict whole raw inventory composition remain separately blocked.

## What can actually be observed

Sources inspected:

- https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
- https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/if_tun.h

`tun_net_init` supplies ARPHRD_NONE and the point-to-point/noarp/multicast flags.
`tun_get_iff` reports stored flags with extra per-file bits in the ioctl handler.
`tun_flags_show` reports masked stored flags; `tun_fill_info` separately emits
PI, VNET_HDR, PERSIST and MULTI_QUEUE state. The model chooses per-device tun_flags,
not an imaginary TUNGETOFFLOAD, for framing state. A real supplier still needs
reviewed no-symlink namespace-bound lookup and exact parse/correlation mechanics.

Important: IFF_NOFILTER and IFF_NO_PI both have numeric bit 0x1000. TUNGETIFF adds
NOFILTER when the per-file socket filter is absent. That ioctl bit by itself
cannot prove NO_PI. The independent per-device flags observation plus original
exclusive setup history avoids making that inference in this model.

TUNGETFEATURES reports valid IFF features, not current checksum/GSO offload state.
TUNSETNOCSUM is ignored. No TUNGETOFFLOAD is defined in this UAPI or handled by
this ioctl switch. `set_offload(0)` clears tun's user-selected checksum/GSO features
and updates wanted features. Required no-user-offload state is setup history
under continuous exclusive configuration custody, not a readback API claim.
The existing real creation helper has NOT performed TUNSETOFFLOAD(0), and cannot
satisfy this record. Nothing here changes that helper or performs the call.
Any config-altering ioctl, duplication/pass/close, or custody loss must invalidate
the generation in a future supplier. A hostile root process can bypass a private
ledger: isolation/containment and absence of untracked writers remain prerequisites.
A fake generation number proves neither. Real netdev feature interaction remains
source-review and later expressly authorized compatibility work.

## Datagrams, not stream chunks

`packetChunk` represents one complete TUN/transport datagram and a known-truncation
indicator. No concatenation of split buffers and no tail retry. Future TUN reader
must reserve at least MTU+1 bytes, reject any result longer than MTU and any known
truncation or saturated detection buffer. A valid-looking inner length does not
clear a truncation indication. Transport frame extraction must prove one complete
frame and preserve application bytes; no stream framing implementation is added.

At most 64 inputs per pass. Oversized batches reject before packet work and consume
the pass; no hidden queue or unlimited retry loop. A truncated or >MTU record is
counted/dropped without parsing/copying. Inspected-byte accounting caps each record
at MTU+1, so diagnostic counter work and accepted copies are bounded. Up to 64
immutable admitted copies of 1280 bytes, no packet payload logs. Empty batch is
finite and consumes a pass. Unknown direction rejects. Real scheduling, drain
fairness, backpressure, rate limits and single-owner serialization remain unwired.

## Support limits unchanged

TCP/UDP fragments are passed unchanged after endpoint/header/protocol/bounds checks;
there is no port/flow ACL enforcement on fragments and no port-policy claim for
this endpoint-only tunnel. Receiving kernel owns reassembly/overlap/checksum/socket
matching. No userspace reassembly, NAT, checksum rewrite or application-byte change.
Options, fragmented ICMP and uncommon ICMP outside S38 allowlist remain unsupported.
CGNAT 100.64/10 overlap must be checked against actual host/VPN routes before any
host deployment. Existing namespace-only Prepare rejects mesh overlap, but is not
a real host overlap audit or ordinary-app path proof.

## Unrun source vectors

Both directions pass immutable correct endpoint UDP and reject reversed endpoints.
Cross-owner/session/namespace/identity/plan/original/generation/handle, stale/single-
use/failed collection/cross-reader, framing/offload-history/MTU/key, hardware/flags,
creation vectors/tail/late capture and stop/deadline/poison/pending refusals. Chunk
vectors cover known truncation, MTU+1 oversize, split input datagrams, 65-record
batch refusal and unknown direction. A valid baseline creation is built before
single-prerequisite creation-negative cases. No new tests executed by author.

Next source gates: independent inert reviewer, then real original-resource supplier
source review, effective ALL observation/policy slice, authenticated peer MTU, full
raw composition and real installed host app-path design. Device access remains
previously denied; no retry/workaround or live release is implied.
