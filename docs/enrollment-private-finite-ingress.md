# Private finite development ingress on the connected enrollment fixture

This local change adds no exported constructor, Server route, production default,
startup caller, deployment or activation. The actual crossapp workflow fixture
explicitly creates one gate on its owning Store, after its initial state file
exists, and installs a new gate only after real Store Close/reopen. Its handler
pointer handoff is atomic. Its inner handlers still strict-parse and authenticate
all purpose-specific requests and own every enrollment/current-map effect.

## Bounds and lifecycle

Explicit development limits cap lifetime admitted requests (4096 maximum), active
requests (16 maximum), body bytes (16384 maximum), retained/candidate state bytes
(16 MiB maximum), per-collection entries (4096 maximum), and request timeout (one
minute maximum). These are engineering ceilings, not selected production policy.
The fixture explicitly chooses smaller limits. One retained gate owner per Store
prevents reconstruction or copied handles refunding the budget. Concurrency
refusals do not queue or spend a slot; admitted invalid envelopes do spend one.
Store Close/freeze retires the gate and cancels admitted request contexts.

The envelope permits only the exact five private workflow/history/map/endpoints
paths, POST over TLS, one canonical purpose-specific 32-byte secret and JSON
content type, without query, escaped path, ambient cookie/origin/referrer or
content encoding. Secret length is checked before decoding. Supported connection
read/write deadlines and bounded body reading precede the Store check; writers
without deadline support fail closed. After cheap envelope checks, busy Store
admission uses TryLock and refuses without waiting on its mutex.

Before full retained-history validation, O(1) top-level counts and current file
size/link checks reject oversized images. A bounded reflection walk over the
fixed acyclic DATA schema caps nested collections, total visited values (128
per configured entry allowance) and aggregate raw string bytes. It special-cases
bounded time.Time, rejects unknown kinds, and uses map iteration without making a
key slice. This is bounded work, NOT O(1) validation. Exact indented/escaped JSON
bytes are checked again after marshal and before temporary-file publication.
Capacity refusal never evicts history or silently rolls back a published image.
Unconfigured Store behavior is unchanged.

## Evidence and limits

Focused race tests cover cheap refusals while Store.mu is held, canonical secret
and header edge cases, shared finite budget, reconstruction refusal, concurrent
no-queue/no-spend refusal, body stalls, unsupported writers, Close/freeze,
context expiry, busy Store admission, top-level and nested candidate caps, and
whole-file/version/in-memory rollback on count/serialized-byte refusal.

The actual workflow fixture exercises this gate for success, expired historical
recovery, recovered finalization, prepared5/7 restart, source denial, and actual
native-key encrypted direct UDP. The separate four native crossapp cases remain
part of regression coverage, not claims that those separate fixture handlers
were wrapped by this gate.

This is not a production ingress: header/TLS connection limits, accepted socket
count, header deadlines, IP/rate limits, durable cross-restart budgets and retention
policy are still server/operator work. The finite gate is susceptible to budget
exhaustion by invalid requests, deliberately fail closed. Existing active
synchronous Store operations can wait on their own mutex after admission and are
not forcibly preempted by context cancellation. Full-history validation/marshal
remain under the Store mutex, with bounded but nonconstant work and JSON escape
allocation overhead. Gate construction checks an already loaded Store; it does
not bound Open's prior file loading. No physical-device, NAT, relay-auth, OS
routing, installed UI or production lease/revocation guarantee follows.

A fixture setup bug initially installed the gate before the state file existed;
setup now creates the initial network first, preserving missing-file refusal.
An initial atomic handoff edit referenced its local gate inside the inner
closure and failed tagged compilation. That edit was corrected and the full
crossapp graph rerun; initial failure logs are retained.
