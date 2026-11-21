# Future network grants and hosted admission, design only

No schema migration, route, longer interval, refresh loop or key creation is
implemented. Values below are states, not chosen durations/capacities.

## Required separation

Initial handoff/proof remains bounded by its original reservation, observation and
proof freshness. It may produce a durable enrollment identity tied to the exact
owned key, source principal namespace, workspace, registrar/service audience,
policy epoch and historical proof. It cannot itself authorize indefinite traffic.
A renewable finite NETWORK GRANT is a separately versioned operation requiring
current Multies eligibility and exact enrollment possession. History/status is
never renewal. Product approval must settle offline residual guarantee versus
continuity; no interval/cadence is selected in this design.

Enrollment states proposed: pending-proof -> enrolled -> suspended/revoked.
Grant states proposed: reserved -> source-observed -> committed -> expired/revoked.
Terminal revoked epochs cannot become current through backup restore, a stale
snapshot, same-ID retry or legacy account/Join. Server-side epoch and version floor
must be independently durable. Legacy separation is a prerequisite, not a flag
assumed to exist. Direct peers and relay must enforce current epoch/grant and join
existing consumers on withdrawal; an API403 alone is not traffic revocation.

## Hosted admission ownership

One stable service/store lease owns a transactional indexed admission record and
rate-window counters. Identity includes operation purpose, authenticated scoped
service/device identity, canonical original attempt/enrollment ID, immutable
request hash, policy epoch and original deadline. No raw secrets persist in it.
Before source/crypto, cheap TLS/header/body/socket/concurrency limits apply.
Then atomically reserve the chosen service/principal/global budget and immutable
attempt. Invalid admitted work spends its reserved cost. Transport-layer spoofed
principal IDs cannot choose which trusted principal budget to consume. Per-IP
and pre-auth budget is separate from post-auth principal budget.

State machine / crash contract:
- no record: transaction validates current explicit config and reserves once;
  on failure before durable commit, no expensive external action is permitted.
- reserved: durable debit exists; acquire same owner/attempt on reopen. Do not
  create another challenge or reset counts. Before any new external action,
  reconcile whether source reservation can be repeated IDEMPOTENTLY with SAME
  original descriptor/deadline and current source eligibility.
- source-started: uncertain remote outcome spends debit. Retry only original
  source's exact idempotent reservation lookup, no new authority. If source has
  no safe idempotent operation, fail closed/manual outcome, not assume no effect.
- source-observed: persisted authenticated observation/fingerprint/deadline,
  still DATA insufficient alone. Freshness and current epoch must hold before
  local grant/proof publication. Reopen cannot refresh its checkedAt implicitly.
- committing: whole enrollment/grant/consumption/result/counter candidate and
  outbox notification persisted as ONE transaction. Postcommit error returns
  unknown. Exact retry reads same immutable stored result, no reconversion/
  source call/time/epoch/grant allocation. Separate files require an explicit
  recoverable transaction protocol, not two sequential writes called atomic.
- committed: dispatch versioned outbox; duplicate dispatch idempotent. Client
  acknowledgement does not mint another grant. Expired/revoked stored result
  remains historical. Never turn status into current eligibility.
- terminal failure/expiry: original debit/tombstone retained under declared
  retention policy. No refund unless a separately proved preexternal rollback
  transaction exists. No deletion that lets same attempt resurrect authority.

All owner acquisition, transitions, counter debit, indexed lookup, epoch checks,
publication and receipt replay must have concurrency/fault/kill/reopen tests.
A process-local limiter in front is useful defense but cannot implement this
contract. A lifetime total is not a rate limit. Window rollover requires explicit
monotonic/backward-wall-clock handling, authenticated identity and durable epochs.
Limits and rates are policy, not constants chosen from current fixture ceilings.

## Retention and cost before production

Need bounded startup and indexed per-attempt history queries, not full-history
scan/marshal under one mutex on every request. Preserve permanent proof/key/epoch
anti-replay meaning while pruning secret-free detailed events under a reviewed
retention policy. Per-purpose costs include network egress/source calls/crypto/
persistent bytes; request counts do not predict hosting money by themselves.
Choose a supported persistent single-writer transactional backend before hosted
implementation. Do not bolt a second SQLite journal onto the JSON Store and
claim atomic grant/counter semantics without a recovery protocol.

## Evidence required for any later activation

Real source/account membership withdrawal and key rotation, restart during every
state, concurrent duplicate winner1, stale/mixed policy epochs, source loss and
clock regression, unchanged original proof interval, no retry budget reset,
exact historical replay, relay AND direct-peer resource teardown, offline/old
client residual tests, deterministic backup restore behavior, native grant swap
crash recovery and signed update/version floor. Native OS and physical hosts need
separate authorized acceptance. This design does not claim any of those ran.
