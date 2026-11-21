# S46 first-step source orchestration and later-step design

Source-only integration, not a release or runtime claim. No TUN/device/ioctl/
netlink/sysctl read/write/helper/process launch/binary or tests were executed.
Compile-only records below are the only execution added. Existing execution guard
remains false. Inert S45 independent194 green was the input, not this slice's test
result. Independent once-only fake validation is still required for S46.

## One shared orchestration path, still not Ready

orchestrateFirstStep is the actual common dependency seam used by fake tests and
applyFirstStepSource. It begins the exact first-step token using held-bound
pre-diagnostic and fresh socket port/sequence, gives the sender ONE token-bound
permit, receives one datagram, requires that permit was consumed, parses that exact
ACK, checks context/live token, collects once, checks context/live token again,
parses post evidence, checks context and finishes with the SAME ACK/token/post.
All errors abandon the token and leave lineage uncertain. No retries or fallback
sockets/adoption. Replay of pre-diagnostic is refused after success or failure.

applyFirstStepSource holds original fileCustody.mu across diagnostic, token begin,
send, original kernel-source identity verification, collection and parse/finish.
Sender and collector now have LockedSource names and do NOT re-lock, eliminating
both the old separate lock windows and recursive deadlock. Original file close
waits behind the entire scope. Owner deadline/stop/close and lineage generation
are checked at begin, permit/send boundary, before/after collection and finish.
Generic invalidation between stages cancels the token; finish cannot revive it.
This does not make owner/session fields into a concurrency-safe public API. Future
real adapter must serialize its transaction/session transitions and external stop
with custody ordering, not call them concurrently under unreviewed locks.

The live sender opens one fresh route socket with sequence1/local kernel-assigned
port; the permit is applied at its actual Sendto boundary. It verifies original
TUN/device-netns before/after send, returns the exact datagram and never reparses a
new fabricated ACK. The collector returns original-before/after identity plus
raw/ALL/TUN and same-owned-generic-socket controller/STRSET/FEATURES record. No
independent helper is handed a retained raw original FD number.

Precondition remains the resident's already-locked OS thread, proved isolated
namespace and same namespaceCustody. Every source dependency checks current TID/
namespace. The method does not unshare, setns, enter a namespace, start a process
or unlock a thread. The file mutex prevents owned original Close from overlapping;
it is not a hard cancellation bound. Per-exchange250ms poll/context deadlines are
approximate. Sequential raw inventory/sysctl/family/names/features reads are NOT an
atomic kernel snapshot, even with before/after identity and stable inventory.
Concurrent uncontrolled kernel changes can force refusal or evade a finite
sampling interval. Source cannot prove absence of every transient external change.

Success only refreshes packet lineage/tag after alias/MTU exact poststate. It does
not advance adapterSession.forwardStep, call adapterSession.observe or active
observation, set Transaction.Ready, or authorize packets. No step2-5 reuse of the
first-step token or generic mutation capability is added.

## Fake vectors exercise this orchestrator, not a parallel recipe

TestFirstStepOrchestratorActualSeam calls orchestrateFirstStep with fake-only
send/readback dependencies. The fake sender uses the SAME firstLinkSendPermit
and returns an actual modeled raw ACK. Vectors: one success; send failure; bad ACK;
config invalidation exactly after ACK but before readback; invalidation during
readback; close-intent at both boundaries; canceled context at both boundaries;
owner monotonic deadline expiry at both boundaries; read error; replaced original
object in readback; sender pretending success without consuming permit. Assertions
require no failed lineage refresh, no extra reads after a detected cancellation,
no retry/send duplication, no original replacement, no session step advance/Ready,
and refusal to replay the same diagnostic. They are authored but UNRUN by author.
Old runInert tests stay as pure historical seam coverage, not live source proof.

## The exact caller gap, not another claim of active integration

There is currently NO real Kernel adapter implementation in this repository.
Kernel is an executor contract in transaction.go; the only Apply implementation
in this package is inertAdapterProtocol.Apply, intentionally fake-only. It must
NOT be wired to this source to simulate a real adapter.

The eventual caller is real-ownership-adapter.Apply handling ONLY Plan operation1
set-alias-mtu-exact, inside residentBootstrapSource's withResidentNamespace locked
thread, after original exclusive creation and retained resource/session binding.
Its dispatch must call applyFirstStepSource, never send/collect independently or
send again through generic ApplyExact. That adapter is absent. This slice makes
the source entrypoint itself call a complete shared chain, not a live app entry.
The absent adapter remains a real integration blocker, explicitly not solved by
naming a hypothetical type or installing callback dispatch tables.

Transaction.Start calls Kernel.Apply then Kernel.Snapshot. A future real adapter
must join that contract to adapterSession.begin/recordACK/strict Compose/observe
without double-applying the operation, skipping pending state, or accepting the
packet-lineage-only finish as the session's canonical readback. In particular,
packet tag refresh does not update session canonical handle. Until the complete
join and all-step snapshots exist, DO NOT return an active Inventory or Ready.

Remaining readiness inputs: exclusive original creation retained even on error;
actual nsfs/device-netns/original object checks; scoped IPv6-disable writer and
complete postcondition; exact address/link/route kernel side effects; strict
raw-to-Compose including complete effective ALL/default and IPv6/foreign policy;
all adapter ACK+readback transitions and original owner deadlines; authenticated
mesh-key-bound peer MTU/session evidence; installed ordinary-app host route,
firewall/XFRM/CGNAT-overlap path; packet framing/features and lifetime invalidation;
shutdown/close/absence/cleanup proof with sticky close errors. Each gate stays
independent. Source compile or fake success does not substitute for any of them.

## Source-grounded design for remaining Plan steps2-5, NOT implemented

Exact pinned source was inspected locally at named functions. Source URLs:

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/devinet.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/fib_frontend.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/rtnetlink.c

IPv4 devinet/fib_frontend URLs rechecked through readable fetch this slice. Other
sources are the previously retrieved complete pinned files, not model recollection.

2. disable-owned-link-ipv6: addrconf.c dev_disable_change6370 and
addrconf_disable_ipv66397ff distinguish per-interface, default, all. Per-interface
change dispatches NETDEV_DOWN when disabling; all also propagates/default changes.
Only owned interface path is in scope, NEVER all/default writes. addrconf_ifdown
around3840 disables IPv6 routes, walks addresses, cancels DAD and removes address
state unless narrowly retained. disable_ipv6 prevents the keep-address condition.
Therefore not merely one sysctl/raw DEVCONF bit: expected IPv6 address/route,
multicast/timers/cache/nested raw changes must be derived with exact before-state
requirements, not a broad ignore mask. Existing first-step expectedRaw model is
insufficient and will remain refusing step2. No sysctl writer is added here.

3. address-add-exclusive: existing EncodeAddress emits own IPv4/32, equal
IFA_LOCAL/ADDRESS and IFA_FLAGS NOPREFIXROUTE0x200. devinet.c inet_rtm_newaddr960ff
validates, inserts, sets lifetimes and invokes address notifiers. fib_frontend.c
fib_inetaddr_event1461ff calls fib_add_ifaddr1138ff, which unconditionally adds a
KERNEL-origin RTN_LOCAL own-address/32 route, EVEN while link down. NOPREFIXROUTE
is not suppression of this local route. fib_magic1098ff chooses local table255,
host scope for RTN_LOCAL, protocol kernel, prefsrc, output ifindex; unicast would
use main254. Need exact raw address flags/permanent lifetime representation,
local-route raw attrs and duplicates, not just address-list length+one. A down
link returns before broadcast/prefix route additions. Whole before-state and no
foreign own-address routes required. No address token/sender enabled here.

4. link-up-exact: existing EncodeLink only changes IFF_UP via change-mask1,
retains name/alias/MTU. devinet.c inetdev_event1580ff NETDEV_UP calls ip_mc_up;
fib_frontend.c fib_netdev_event1500ff re-adds interface routes for existing addrs,
bumps route generation/cache. fib_add_ifaddr's broadcast/prefix conditions remain
specific, including /32 and NOPREFIXROUTE, not a generic assumption of zero route
side effects. addrconf.c NETDEV_UP3680ff stops IPv6 addrconf if per-interface
disable is actually set; otherwise permanent/link-local/ready/DAD work can run.
Tun carrier/operstate flags additionally depend on actual retained queue/device
state, not request IFF_UP alone. Need named raw flags/operstate, IPv4 AF counters,
multicast membership and observed features before an exact expected-state model
can be accepted. No blanket volatile AF/raw removal and no token for step4.

5. route-add-exclusive: existing EncodeRoute fixes peer/32 unicast/main254,
protocol242, link scope, output exact ifindex and one metric/priority42700 attr.
fib_frontend.c inet_rtm_newroute909ff converts request then fib_table_insert and
flushes cache; that call site alone does not establish complete EXCL collision/
duplicate semantics or full normalized output attrs. Need pinned fib_trie insertion
and fib_semantics nexthop validation/schema next, plus exact before absence of any
conflicting route/policy/host overlap. No route token or side-effect model added.
The operation must not replace an existing route or adopt kernel-normalized
lookalikes. All-table/policy/XFRM snapshots remain required by strict Compose.

These source facts narrow the work. They do NOT claim complete expected raw deltas
for2-5, runtime support, or that a change of one leaf establishes IPv6 safety.

## Compile record and boundaries

Source audit: no package init/go:generate/go:linkname/C imports. Same preinstalled
Go1.26.11, explicit package only, CGO off, no network/toolchain downloads, no tests,
no run/binary/preflight_launch. Provisional S46 build succeeded exit0 before final
post-collection owner recheck was added. Log retained. Final source-only build is
separately logged for the complete production source in this packet. Test source
is not compiled by go build and remains independently unverified.

CGO_ENABLED=0 GOFLAGS= GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
/home/sandbox/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.11.linux-amd64/bin/go
build -mod=readonly -tags 'mnet_enrollment_development mnet_os_process_launch'
./experiments/osownership

No live-tag tests or helper run. Compile validates type/API availability, not the
state machine or kernel. No access retry/workaround after existing S19 EACCES.
No sessionproof/shared Mux/mobile/session/view edits or scope merge among the
three separate tools. No readiness, deployment or user-visible product claim.
