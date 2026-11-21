# OS networking ownership candidate, source preparation only

Goal: ordinary applications can address an approved peer through an actual OS
interface without special proxies. This replaces the local-ingress proposal as
the next product direction. It is not an activation or duration grant.

No TUN, netlink socket, namespace, route, privileged helper, daemon or child
process has been created for this candidate. Only the inert model's race unit
tests ran. No normal entry, production wiring or accepted source was changed.

## Actual current seams and hazards

- internal/daemon/enrollment_development_session.go requires injected Device,
  forbids ManagedIface and LocalDNS, publishes loopback endpoints, owns peer
  scopes, closes resources and device on original authority expiry.
- internal/daemon/daemon.go startDevice uses tun.CreateTUN only when no injected
  device exists. Normal Up includes legacy join/rejoin, and uses
  ifacectl.Configure when ManagedIface is true. Do NOT call normal Up here.
- internal/ifacectl/ifacectl.go Configure uses Linux addr REPLACE with /10 and
  route REPLACE100.64/10. It has no per-step ownership rollback. That is too broad
  for this first exact-pair experiment. It must not be reused unmodified.
- internal/ifacectl/subnets.go models exclusive add/exact protocol+metric cleanup
  for RFC1918 serving, but intentionally excludes mesh. It is not a ready mesh
  owner and does not solve crash lifetime or namespace ownership.
- Pinned WireGuard tun_linux.go CreateTUN opens /dev/net/tun, uses TUNSETIFF,
  IFF_TUN|IFF_NO_PI|IFF_VNET_HDR, without enabling persistence. Its Close releases
  the descriptor and related owners. Crash teardown must still verify actual
  descriptor inheritance/lifetimes, not assume closing one Go object suffices.

## First platform and exact resource contract

Linux in a fresh dedicated user+network namespace only. Parent holds an original
namespace FD and proves child namespace device/inode differs and is network
isolated. Child sees only lo, no veth/bridge/default/off-namespace route, no
forwarding and no foreign policy rules. Inspection is on that exact open
namespace handle, not a caller assertion. No setns into the initial namespace,
no sudo, no host sysctl/firewall/DNS/default/subnet/exit changes. Other platforms
remain design-only until their own ownership/privilege model is reviewed.

Require enrolled own/peer key and assigned IP, current receipt/source/session,
and effective deadline min(self,peer). A device grant does not authorize any
third peer or local LAN forwarding. Generate dedicated random short interface
name and128-bit alias. Refuse existing name/alias/address/conflicting route;
never adopt, rename, replace or remove another interface.

Create NONPERSISTENT TUN only after preflight; capture actual ifindex, name,
alias, namespace inode/dev and open TUN FD. Set MTU1280; exclusive ownIP/32 with
NOPREFIXROUTE, then up; add only exact peerIP/32 link route, main table,
protocol242. No mesh/10 takeover, no IPv6 yet. Exact Peer Map WireGuard allowedIP
is also /32. A later multi-peer product increment needs independent route/grant
handling, not a broad default mesh grant inferred from membership.

Interface/route operations must use acknowledged exclusive rtnetlink operations
and readback in that namespace, not a shell command string or PATH utility.
The inert model has NO executor. Its Namespace.Isolated and inventories are
DATA; a real executor cannot trust those fields without kernel proof.

## Ownership transaction and failure semantics

Preflight all resources. Commit one acknowledged step at a time and record only
public owned resource identifiers. Capture intent before operation, kernel ACK
and readback after it. A timed-out/failed netlink request may have succeeded:
readback before deciding ownership or rollback. Do not call a cleanup-success
flag merely because the original command errored.

Route deletion is exact namespace+destination+ifindex+protocol, not name-only.
Before EVERY cleanup mutation, compare actual link name/ifindex/alias/kind and
namespace. An ifindex can be reused; a name alone is not identity. Refuse on
replacement or foreign address mutation; retain pending cleanup diagnostics.
Then exact route delete, exact address removal, close original TUN FD; namespace
teardown is the ultimate confined fallback. If a route/link disappeared already,
record verified absence. Never broaden deletion to force a clean result.

All normal stop, partial setup error, source withdrawal and effective expiry
must close consumer connections, route/interface owners, WireGuard and TUN, join
workers, then verify absence and unchanged baseline outside owned resources.
Expiry begins from immutable original authority, never reset after configuration.

Crash is a different owner: a separate parent owns the test namespace lifetime,
the child PID/pidfd and cleanup deadline. Child holds all TUN FDs with CLOEXEC,
no descriptor passed to curl/other app. A process crash closes descriptors, but
parent verifies routes/interfaces really disappear in the held namespace before
calling it rollback. No persistent TUN flag, no daemonization, no inherited FD.
If child hangs, a separately reviewed direct-owned PID-only kill policy may
terminate it; do not discover unrelated PIDs or run blanket process kills. A
supervisor must itself contain parent death, e.g. dedicated lifetime process
with armed parent-death signal and parent-alive check after arming. Go thread
parent semantics require a live pinned supervising thread or launcher, not
LockOSThread only around Start. Crash execution is NOT released here.

## Smallest meaningful OS-path validation, proposed before execution

Stage S0 is SOURCE/UNIT ONLY, now: pure planning, conflict/replacement refusals
and exact rollback model. These tests are not OS proof.

Stage S1, next separate source-review packet then explicit execution release:
A new direct-owned disposable namespace with real nonpersistent TUN, no
WireGuard, source accounts or daemon. Inspect exact namespace via FD and baseline
kernel routes/links. Set one synthetic mesh/32 plus one exact peer/32 route.
A child ordinary TCP client performs a bounded SYN to that peer; parent reads
an IP/TCP SYN from the actual TUN FD and validates source/destination/port.
No response/full connection is claimed. This proves the OS routes an ordinary
socket into our actual TUN, not just a fake channel device. Stop/expiry closes
FD and verifies interface/route disappearance; inject one reviewed partial
configuration failure and verify rollback. Original outside namespace snapshot
must be byte-semantically unchanged. No source receipts, real user data or
network packets can escape the namespace, since no veth/default/real NIC.

S1 is meaningful but NOT encrypted peer communication. Crash lifetime should be
a separately reviewed case, one owned child abrupt exit and verified kernel
cleanup, not folded into a generic repeat loop. Capability unavailable means
blocked/inconclusive, no host fallback. /dev/net/tun is present here, but opening
it/CAP_NET_ADMIN in fresh userns is UNTESTED. ip utility is not found; executor
should use a small reviewed rtnetlink implementation, not install a helper or
weaken route checks. No namespace/TUN capability test was run for this proposal.

Only after S1 and crash ownership evidence: separately review a finite native
pair composition injecting actual OS TUN devices into the existing development
session, never normal Up. Two isolated peers with loopback-only underlay and
exact route grant; ordinary client -> actual TUN -> WireGuard -> peer OS TCP
server and original expiry. Both controls on current source must fit unchanged
window, or be honestly inconclusive. No longer duration is assumed to make it
fit. Parent/machine networking, production and native real-account activation
stay closed. Windows Wintun/Darwin NetworkExtension/helper designs are future,
not falsely covered by this Linux candidate.
