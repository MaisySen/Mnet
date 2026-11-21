# S53 stable-dump policy, real route AF provenance and precreation-only Snapshot

SOURCE only plus fake seams. No actual read, ioctl, TUN/device open/close, helper,
namespace/process/preflight/run or compiled binary executed. No author tests/race.
Input S52 independent209 inert/race13/compile is not S53 test evidence.

## ReadStableInventory comparison correction

Two complete inventory sets now compare through existing exactSetupPostMatches,
which uses pinned comparableSetupRaw comparison copies. No new volatile masks.
The reader returns the ORIGINAL unmodified SECOND successful inventory, not
normalized bytes or a synthetic average. All safety/raw shape/presence/unknown
attrs/rules/address/route values remain exact. Address/route cache/lifetime drift
still fails closed; no ordinary-host reliability or atomic snapshot claim.
Bound remains two whole attempts/250ms context, not a hard kernel stall bound.

## Actual route-socket afReadPass producer SOURCE

collectRouteAFPass records the exact request and raw reply datagrams through a
recording NetlinkWire, with actual source caller supplying its newly opened
routeSocket and kernel-assigned local port. NetlinkClient.Request enforces exact
GETLINK18 AF_UNSPEC16-byte body/flags0x301, increasing sequence, kernel sockaddr
sender0, recipient/local port, multipart type/flags, complete DONE, nontruncation,
length/padding/trailing/error checks. Source route socket still has read allowlist.
Pass retains original request, replies, parsed headers, raw decoded links, sequence
and source-owner lineage. No inertAFReader or fake-only marker implementation is
constructed by the live producer.

afReadPass now has a separate private source-owner path as well as its existing
inert reader path. requestClaim requires exactly ONE provenance owner, same current
last pass/sequence, active source owner and matching request header/port. Source
owner is scoped to controller resident lifetime, invalidated on new pass/failure
and deactivated/last removed at resident scope exit. It is correlation/provenance
from the source caller, not cryptographic authorization or a kernel-version label.

observePrecreation opens the real source route socket, collects this pass, reads
the complete stable inventory through source readers, compares its links to the
pass under the same pinned policy, then reads canonical sys/effective ALL/default
operands and checks resident namespace/TID again. It attaches sys/effective data
to that SAME pass and returns a copied precreation observation. These are
sequential bounded source calls, not atomic kernel evidence; none were executed.

## Connected precreation-only operational conversion

operation1Adapter.Snapshot now has ONLY an additional precreation branch when no
attempt/canonical handle/create-attempt exists. It requests a typed observation
from its installed resident creation source, then immediately consumes it through
consumePrecreation. Other snapshots retain the prior diagnostic-only error path.

Conversion requires exact controller owner, adapter attempt identity, original
Binding, frozen baseline pointer, precreation phase, controller sequence, initial
precreation generation1, current source pass/port, no stopped/closed/Ready state,
no attempt, no canonical/create-attempt and an unconsumed observation. Attempt
identity is this stable session identity before file creation; it is not a claim
that an original FD already exists. After an attempt exists the branch is closed.
Wrong/old/used/phase identity refuses. One observation cannot be consumed twice.

It checks pass link/raw correlation, sys map/effective observation equality,
complete current effective operands and existing pinned originalBaselineMatches:
target absent, all baseline raw/safety/rules/address/route state equal except
already-reviewed comparison fields, sysctl maps exact. Then it calls existing
strict baseline.Compose with normalized comparison copies for baseline/current
raw. It preserves the original baseline fingerprint. There is NO owned TUN in
this phase, no general AF26 exception and no unchecked owned DEVCONF policy.

This yields a baseline Inventory only for precreation. It is NOT postcreate/op1
conversion or readiness. Operational conversion once original creation is attempted
remains ownedAFPolicyIncomplete/unsupported. No fresh-TUN IPv6 profile or unresolved
named owned devconf semantics were guessed. Source creation is now the intended
next Kernel.Create call if a future actual precreation source pass succeeds; the
mandatory postcreate Snapshot still refuses. Failed Start retains/closes original
through owner cleanup. No runtime helper is installed/released, no actual Start
or creation runs and no network behavior is proved.

## Authored UNRUN fake tests, exact scope

- Actual ReadStableInventory over fake DumpSource changes link stats64 counter
  between full dumps; accepts with8 requests and returns second original byte2,
  not masked zero. Safety flag change refuses after bounded16 requests.
- Actual collectRouteAFPass over fake recorded wire: kernel sender/sequence/port/
  truncation/missing-DONE refusals; successful pass has source owner, no inert
  reader, retained request/headers and refuses after source scope deactivation.
  This tests common codec/provenance seam, not a real socket/syscall.
- Actual consumePrecreation over fake wire-produced records: baseline precreation
  succeeds with Ready still false; wrong phase/attempt/sequence/generation/later
  canonical session/target present/pass port/policy mutation refuse; every value
  refuses reuse. No fake pass is installed in production or postcreate conversion.

The baseline fixture is existing recorded raw plus test-only complete effective
operands, so pure success does not prove actual live baseline/source collection
supports those operands or full named owned devconf. Parent owns independent
ordinary/race validation; source build does not compile test files.

## Lifetime and remaining source gates

Whole eventual packet runtime remains INSIDE resident work callback; returning
closes original nonpersistent TUN/revokes suppliers, never returns escaped Ready
network object. Current producer source-owner/pass becomes inactive on that exit.
Owner lane is separate from namespace locked-thread residency; both required.
No postcreate/op1 operational conversion, ops2-5, route/address cleanup tokens,
peer-authenticated MTU, installed ordinary-host routing/firewall/XFRM/overlap or
new Ready wiring. Existing activeInventory always-refuse remains unchanged.

Source guards still mechanically unreleased. No S19 retry/access workaround or
shared sessionproof/Mux/mobile/session/view edits. Full baseline/safety exactness
can conservatively refuse live environments; no ordinary-host reliability claim.

## Compile-only validation

Provisional and final production source explicit package builds succeed with
preinstalled verified Go1.26.11 offline/CGOoff, only development+process_launch:
CGO_ENABLED=0 GOFLAGS= GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
<existing-go1.26.11> build -mod=readonly
-tags 'mnet_enrollment_development mnet_os_process_launch' ./experiments/osownership
No go test/run/binary/preflight_launch. No hook/generation/C import/init introduced.
Compiler success validates source types/APIs, not tests or runtime protocol.
