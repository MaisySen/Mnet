# Attended runtime grant, policy and implementation boundary draft

Not a grant, selected duration, new receipt format or activation. No source
service, database, credentials, APIs or production code are changed. Existing
joining receipts remain immutable finite initial-development credentials.

## Separate three decisions

1. Join: the person signs into Multies and approves this exact device possessing
   its key to join the named network. Current source proof/session required.
2. Start networking now: an attended action names exact own device/key, exact
   peer device/key, network, intended network access and maximum session end.
   Both endpoints need matching consent or a separately explicit on-point grant.
3. Unattended/repeated starts: a separate future decision, NOT inferred from
   joining, app install, remembered friendship, accepted connection or this plan.

A runtime grant is not an extension of proof freshness. Short-lived challenge
must be verified before commit; an independently authorized runtime may last
longer ONLY under explicit service policy and human selection. Current native
credential cannot be reinterpreted as that runtime grant. No default number is
chosen in this draft; UI must show actual maximum and precise stop behavior.

## Proposed grant semantics, to review before serialization

Required binding: distinct own/peer public keys and device IDs; network ID;
issuer/audience/service; purpose=attended-mesh-peer-only; source principal and
session/security generation; both endpoint decision provenance; immutable
not-before and absolute not-after; nonce/ID and revocation generation. Route
scope explicitly exact peer/32 for the first OS candidate; no internet/subnet,
third party, screen, microphone, keyboard or mouse authority. One active run ID,
one start reservation, no implicit reconnect/reuse after crash or logout.

Service authenticates source/key possession, current source session/SSO and
current device/peer eligibility atomically with grant reservation. Source outage
is refusal, not cached success. Source checks cannot be fabricated from app
headers or DATA-only consent rows; the latter still have a full app typecheck
and security gates. Reuse source-of-truth predicates only after exact review.
A public UI summary is DATA, not authorization. A caller cannot supply its own
expiry, trusted time, scope or key replacement.

The user sees: own device, peer, network, app-networking scope, explicit duration
and end/Stop, and whether logout/closing this app ends networking. No allow by
timeout, hidden background persistence or one checkbox for all three tools.
For the initial attended design, recommend Stop, app/owner exit, source logout,
peer refusal/withdrawal, key replacement, security-generation change and absolute
end all retire access. This recommendation is not an approved product choice.

## Currentness and failure, unresolved choices named

Longer time needs a new explicit bounded checking policy; old14-request slices
and short receipt expiry cannot silently become unlimited polling. Choose a
maximum stale-currentness interval and disclose it in the UI/service contract,
including which revocations are immediate and which can lag. Stronger revocation
needs pushed updates or short renewable service attestations. Those attestations
must be narrower than the original human maximum, never extend it, and must not
become permission to recover after crash. No cadence selected or enabled here.

If source becomes unreachable, stop at the earlier currentness horizon, not at
a newly guessed grace time. Clock uncertainty is conservative and monotonic
elapsed time supplements absolute signed bounds, never exceeds them. Two endpoint
ends can differ: effective route/consumer end=min(own runtime, peer runtime,
currentness horizon, revocation, operator stop), with specific reasons. A renewal
request is not approved by possession of an old receipt. No offline durable run
lease or startup recovery until separately designed and explicitly authorized.

## Implementation boundary

- Keep attended runtime in a separate purpose/version/storage namespace and
  validation path from joining and the immutable original credential. No legacy
  auth-key/node-token or normal Up fallback, no consent DATA-as-credential.
- Add an explicit local start request only after service protocol/source-policy,
  UX words and owner lifecycle review. It must show source-grounded actual keys,
  scope, absolute end and currentness state before opening OS routes.
- Grant reservation/finalization uses durable one-use consumption and exact
  identity. Partial local setup consumes start or remains visibly pending; no
  automatic replacement identity/retry to hide failure. Network cleanup is not
  permission to reuse a consumed start.
- OS owner depends on a validated runtime handle, not raw JSON, local ledger or
  a generic bool. That handle exposes only bound peer, permitted routes,
  effective deadline/currentness and cancellation; no secret copied to helpers.
- Separate local owner/supervisor owns TUN/routes and consumers. State changes
  cannot outlive the effective handle; revoke/expiry/crash cleanup is testable
  without real user accounts. No foreground UI means no attended runtime unless
  explicit later policy says otherwise.
- Logs contain public event/time/resource IDs and reason categories. No source
  session token, private key, grant bearer, decision URL or remote app payload.

## Needed decisions before real use

Human: which actual devices/peer/network and exact networking scope, attended
start and maximum duration, app-close/logout behavior, any repeat/unattended
rights. Operator: maximum grant lifetime/currentness lag, revocation delivery,
source-policy accountability and service configuration. Engineering: exact
serialization/authenticity, monotonic/absolute clock behavior, one-use storage,
OS supervisor privileges and cross-platform cleanup, UI truth and independent
security tests. These can be designed/source-modeled without asking the human
for secrets or prematurely requesting a broad permanent-access grant.

Smallest first implementation preparation remains pure eligibility/time/one-use
model and negative tests, not changing current max_lease_ms or challenge_ms.
No runtime duration policy is made effective by this document.
