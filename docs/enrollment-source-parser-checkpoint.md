# Multies v2 source response DATA parser checkpoint

Status: disconnected and unaccepted source integration. This checkpoint does not
accept the Multies source review, authenticate a source, perform own-PKCE, own a
native intent, grant network access or consume a proof. Nothing calls it from
control, daemon, UI or a network route. No desktop/shared-input files changed.

## Implemented against actual producer responses

`internal/enrollmentcontract/source_responses.go` parses the exact output shapes
of Multies c3d1f8f92c6fb9ff18a2061ac2d1f2f6730239ac exchange-v2 and
reserve-evidence-v2. A disposable SQLite test app ran its actual prepare, approval,
PKCE exchange and authenticated live reserve-evidence handlers. Its two JSON
outputs are pinned as `testdata/multies-provenance-source-v2.json`. The ticket,
code-derived identifiers, user and session-binding values belong only to the
new disposable test database. They are NOT captured user credentials. Time in
this fixture is historical; tests supply its explicit observation time, not
machine wall time. It is not a replayable production response.

The source fixture exporter ran as a temporary Vitest file beside the existing
bookkeeping fixture. It called `setupBookkeeping(true)`, then the real service
route using that fixture's service Authorization and exact request. The exporter
was removed after the run. Multies remains clean at c3d1f8f; no producer change is
part of this checkpoint. Independent source review is still required.

Strict decoding requires all fields at every object level, exact case, no unknown
or duplicate fields (including escaped aliases), non-null scalars/objects, exact
versions, valid UTF8, bounded 64KiB input and depth. Source dates must have the
producer's exact UTC millisecond spelling. Native launch tokens are canonical,
nonzero 32-byte base64url values, callback is the dedicated IPv4 loopback path,
and principal namespace is explicitly local-user-v1. Mapping IDs and service IDs
use the producer's ASCII identifier syntax. Principal/audience strings reject
whitespace and control characters. Parser intentionally rejects replacement
characters rather than silently accepting malformed surrogate recovery.

Exchange comparison requires an independently supplied saved launch, principal
authority, BOTH original saved and current allowed mapping snapshots, and an
explicit now. All launch fields are compared, including originally saved key,
credential digest, PKCE challenge and state. Response cannot pick its trusted
origin/config, replace the original map or assert enrolled/input authorization.
The function cannot prove where those expected arguments came from.

Reserve comparison requires the own exchange DATA, independently selected
service/reserve/descriptor, current exact mapping and explicit now. It compares
same-row provenance, launch and mapping, checks original handoff, reserved status,
false authority/enrolled flags, descriptor version, no future checkedAt and strict
unexpired deadline. It also recomputes the exact descriptor from the own exchange
and independent service/reserve, rather than trusting a source-supplied digest.
No stale-age, clock margin or lease policy is silently selected here. Those are
still mandatory future authority-adapter checks. Deadline is not accessUntil.

`source_descriptor.go` implements the existing Multies reserve-descriptor-v1
ordered JSON array. Ticket is intentionally excluded. This is not the possession
transcript encoding. Its JSON string emitter matches JS JSON.stringify, including
HTML-sensitive Unicode/quotes/backslashes without Go HTML escaping. The actual
Multies `reserveDescriptorBytes` and `reserveDescriptor` functions produced the
second test fixture, for both actual response values and a Unicode/HTML/quote/
backslash audience. Their normal digest equals the live handler response digest.
No version or golden reserve contract was changed.

## Evidence and deliberate gaps

Targeted race checks pass for enrollmentcontract, enrollmentidentity and
existing enrollmentconfig. Tests remove/null every field recursively, add unknown
nested fields, alter source/authority/launch/config/service/descriptor/timestamps,
and test aliases/duplicates/false flag preservation. Parser fuzzing seeded with
actual valid responses ran 75,859 executions in the recorded short run without
panic. This is a bounded fuzz sample, not proof of parser safety. Vet and diff
checks pass. No UI, native Windows/macOS runtime or production TLS test occurred.

The exported structs are mutable DATA and can be constructed by any Go caller.
They must NEVER be accepted as a permit or trusted source object merely because
these helpers returned nil. SourceExchange contains a private ticket: String and
GoString redact it, but JSON encoding/field access can still expose it. Future
network adapters must never log response bodies or those fields. The response
fixture is not a license to capture real tickets in tests/reports.

Still required: reviewed c3d1f8f source; independent exact HTTPS source client and
no-redirect/request/logging policy; actual durable owned native key/secret/PKCE
intent and config snapshots; current config adapter with explicit local namespace
(no reuse of issuer/subject config v1); clock/staleness/lease policy; reviewed
bootstrap/challenge/DH/MAC/confirmation; bounded admission and actual Store atomic
consumption transaction. No DATA-only permit constructor is added. General Store
and native journal ownership must not be conflated or duplicated blindly.
