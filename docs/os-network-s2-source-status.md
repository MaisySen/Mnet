# S2 source and fixture status

Source only. No namespace, capability, TUN, rtnetlink socket, sysctl, daemon,
crash signal, host networking, sudo, chmod or installation was executed.
The Linux construction guard is still a compile-time false constant. There is
no real Kernel adapter, launcher, production wiring, consent or runtime grant.

## Implemented source

- Bounded nonblocking NETLINK_ROUTE transport (250ms, 10ms poll, 64KiB recv,
  1MiB response cap). Held namespace identity checked against thread-self netns
  before send/receive; syscall errors poison/close socket. Codec errors close a
  closable transport. No stale reply reuse after failed exchange.
- Sequence/recipient/kernel-sender/ACK original-header validation; typed errno;
  expected RTM_NEW kind for each dump, exact AF_UNSPEC request structures;
  multipart and completion checks, total size, padding/alignment and truncation.
- Strict link/address/route native-endian decoding, unknown attrs retained.
  Dedicated dummy nested linkinfo encoding; exclusive address and route adds,
  ACK-only deletes/link updates, no replace. Linux metric and priority alias
  the same RTA_PRIORITY and must agree.
- Two complete equal raw inventories required, bounded to two attempts total
  and 250ms; restart the entire set after any interrupted/failed dump. Namespace
  checked around each dump. Raw rule records preserved after framing check.
  This is NOT a validated policy Inventory: no rule semantic or forwarding
  decoder, kernel-origin classifier or sysctl inventory composer exists yet.
  Unknown/nested effects are not silently declared harmless. Stable sequential
  dumps are not atomic proof without exclusive namespace/process custody.
- Explicit owned-link disable_ipv6 source with no-follow directory-relative
  file access, namespace/link checks, write and readback. Missing IPv6 tree
  blocks, not proof of disabled support. No all/default sysctl mutations.
- Original FD close at most once, including cleanup readback retries. Full
  own/peer address/destination absence and unchanged multiset/map baseline
  required before cleanup complete. Ambiguous creation without handle remains
  pending, never adopts by name. Baseline slices and maps cloned.

## Deterministic tests, not kernel facts

Added individual corruptions of five embedded ACK fields, trailing frame/byte,
length/alignment/padding, multipart/kind, response cap, DONE payload, dump
success/error ACK and multi-datagram success. Typed EEXIST/EPERM/EBUSY/ENODEV.
Real decoder/encoder byte fixtures and malformed attrs/headers. Whole-inventory
retry bound and namespace mismatch. Missing effects for all four apply steps;
snapshot errors at each readback and Stop; ambiguous create/index0; cancel;
namespace swap; derived-route wrong metadata; orphan destinations/baseline;
step evidence content; exact one-close transaction and ordinary-file FD reuse.
The fake deletes link-derived state at FD close by assumption. That behavior
is expressly NOT evidence of real TUN teardown or crash behavior.

Compiler: Go 1.26.6 via temporary modfile; repository go.mod unchanged.
Race tests and vet run only on this package. No exact Go 1.26.11 claim.

## Remaining implementation and release gates

1. Review this exact source and full tests, including new codec paths.
2. Decode rule/sysctl/origin semantics without lossy safe defaults; capture
   full namespace baseline. Read-only IPv6 readback outside write also needed.
3. Implement reviewed resident-process namespace custody and exclusive runner
   ancestry. Every mutation must revalidate namespace AND exact handle inside
   adapter; generic byte codec alone does not supply ownership authority.
4. Implement separate no-TUN conformance driver from the adjacent proposal.
   Guard stays false until exact driver/source and named runner are accepted.
5. Only afterward consider S1 TUN with a named runner whose existing device
   access is demonstrated without chmod/sudo/install/host fallback.
6. Resident owner/client lifetime, bounded outer supervisor and independent
   crash cleanup remain unimplemented, not proven by transaction Run or fakes.

No prior finite trusted-pair run is repeated or authorized by this source.
Runtime policy remains a draft, with no attended duration chosen.
