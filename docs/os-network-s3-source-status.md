# S3 source correction and recorded read-only fixtures

Supersedes S2 source status where changed. Still SOURCE ONLY. This task ran
pure/fake tests and ordinary temporary-file FD tests only. It did not execute
namespace/capability/TUN/netlink/sysctl/daemon or signal operations. The Linux
construction guard remains compile-time false. No production wiring, runtime
activation, repeated pair run or duration grant was added.

## Corrected ownership sequencing

Create binds an unconfigured observed link under original exclusive FD custody:
index/name/kind and initial MTU, empty alias. It does not pretend the ownership
tag is set. The next explicit operation sets alias+1280 MTU with ACK and exact
readback before disabling IPv6, /32 address, link-up and exact peer route.
Link identity now includes MTU. Pending tagging can match only its recorded
initial link or exact desired tagged link. After confirmed tagging the initial
shape is no longer accepted. No-handle binding failures close the original FD
exactly once in the binding helper; they do not leak it to name-based adoption.

Public byte+kind mutation is removed: Request is dumps only. Opaque
ExactMutation is constructed from a bound handle and exact operation. Internal
mutation rejects DELLINK. TUN removal remains original-FD close, never link delete.
Dummy creation is internal and separate. No dummy deletion executor is wired.
Every live request uses a new socket and port identity, closes on success/error,
and can be retried only as an entire inventory attempt on fresh sockets.

## Actual fixture correction, no mutation evidence

Parent supplied two reviewer-owned read-only capture outcomes. Run1 stopped on
an invalid sysfs-derived precondition, with no dump. Run2 corrected enumeration
via rtnetlink. Both original verdicts are retained unchanged under testdata.
Run2's raw bytes replay through the real codec/decoders in offline tests.

Run2 has ten down links, zero addresses, zero routes, seven rules. The down
kernel fallback links invalidate the old lo-only preparation assumption on that
runner. No name allowlist was added. The source candidate instead captures each
run's exact immutable raw baseline. Any changed/missing/replaced baseline link,
including unmodeled attrs, fails. It requires a distinct/exclusive fresh
namespace, all links down, no addresses/routes, no master/cross-namespace link,
default rules and explicit forwarding/sysctl readback. All baseline links remain
unowned and must never be brought up or mutated. Isolation/ancestry, not a
friendly device name, is the required trust boundary. This threat model remains
a review proposal, not permission or proof of no egress.

Only EMPTY nested IFLA_DEVLINK_PORT (62) and IFLA_DPLL_PIN (65) gained decoder
acceptance, grounded in the actual fixture and UAPI. Nonempty instances and
other unreviewed nested flags still fail with type/length in the error.
Reference inspected:
https://github.com/torvalds/linux/blob/master/include/uapi/linux/if_link.h
Rules use linux/fib_rules.h and linux/rtnetlink.h UAPI structures/constants.
Complete rule validation covers exact count, family, source/destination length,
TOS, reserved bytes, action, flags, table and FRA_TABLE agreement, priority,
FRA_SUPPRESS_PREFIXLEN=UINT32_MAX, FRA_PROTOCOL=kernel, and rejects every extra
selector/unknown attr. Both multicast routing families128/129 are explicit.

## Raw-to-policy source, conservative and still blocked on real applicability

Baseline candidate deep-clones raw records/sysctl evidence. Composition checks
all raw baseline links/rules and baseline sysctl contents remain unchanged.
Forwarding reads ipv4/ip_forward and all/default/every link ipv4/ipv6 forwarding.
IPv6 disable/addr_gen must be explicitly present and valid. A guarded read-only
collector uses no-follow dir-relative file traversal and namespace checks around
each read. A missing per-device IPv6 leaf is an applicability gap, NOT disabled
or unsupported IPv6.

IMPORTANT: actual captured sysctls omit IPv6 leaves for several IPv4 tunnel
links (e.g. tunl0). The actual capture test therefore EXPECTS composition to be
blocked. Separately named synthetic completion tests exercise candidate logic;
they are never saved over or represented as the actual fixture. More source
semantics and reviewed evidence are needed to distinguish missing applicability
from collector omission. No new live capture is requested/executed here.

Owned-link attrs are intentionally narrower than a real full dump and reject
unreviewed attrs. Address flags require exact permanent+NOPREFIXROUTE; extra
address attrs fail. Origin is never inferred just from protocol2: only exact
owned /32 local/host/table255 route metadata plus the address-ACK witness can
produce the model's derived-origin flag. IPv6 effects and unexplained route
attributes fail. These owned-route/address forms still have synthetic fixtures
only; no TUN or dummy mutation fixture exists. This source does NOT claim a
working live policy adapter. Captured empty routes cannot prove derived teardown.

## Resident placement, supervisor and verdict source

Guarded re-exec launcher creates NEWUSER|NEWNET with exact uid/gid maps and a
startup gate. Parent opens pidfd, proves netns distinct and nsfs/type, verifies
owning userns equals the child's distinct userns, checks pidfd lifetime around
/proc access, then opens the gate. It keeps target nsfd/pidfd/verdict pipe and
bookkeeping, never a TUN FD. Child receives initial-parent nsfd for comparison.
Resident callback locks OS thread for the entire namespace-custody lifetime;
checks reject different TIDs. No setns/unshare-in-process call exists. Separate
client re-exec inherits resident namespace and no TUN ExtraFiles.

Source supervisor has finite conformance-only 5s bound, 500ms abort/reap reserve,
one<=64KiB JSON verdict and EOF. Duplicate/trailing/unknown/oversize/wrong-ns
verdicts fail. Decode/baseline/sysctl/custody/cleanup/runner failure classes remain
failures. Successful verdict requires same baseline/final digest and explicit
cleanup. Child exit is not a networking-absence oracle. Failed reaping retains
namespace custody in a typed pending outcome instead of closing it as complete.
Pipe reader is joined before custody close. All of this is unexecuted source.

A fake-stage no-TUN driver fixes the finite stage order and stops/joins clients
before cleanup at every failure cut. Stage executor remains unimplemented; no
re-exec handlers are wired into an executable. Launcher/namespace syscalls,
startup gate, userns proof and abort behavior have not been run or proven by the
fake-process tests. These are NOT conformance-ready claims.

## Tests and remaining gate

Tests add independently decisive record cap, own/peer address/route absence,
nil baseline, multiset/maps, route scope, MTU, typed operations/raw API refusal,
per-success socket close, two unequal complete inventories, unconfigured tag
phase, no-handle FD close, command placement configuration, fake supervision,
verdict bounds, failure cleanup ordering, unreaped custody retention, captured
raw replay, exact rule mutations, empty nested-attr bounds, baseline raw/sysctl
copying and narrow origin classification.

Go1.26.6 via temporary modfile, not repository-pinned Go1.26.11. Race suite/vet
logs accompany the packet. No wider repository/API/daemon suite claimed.

Next requirements before any live mutation: independent exact-source review;
settle immutable baseline threat model and IPv6 applicability evidence; complete
reviewed owned dump schema, exact per-mutation adapter and dummy-only deletion;
wire reviewed stage executor/re-exec handlers; review launcher/cleanup source;
name capable namespace runner and obtain a separate one-shot release. TUN,
crash, real WireGuard and product activation stay later independent gates.
