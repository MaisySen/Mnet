# Two-installed-owner userspace HTTP feasibility candidate

Source review candidate only. Integrated adapter execution has NOT occurred.
No claim of window fit, production readiness, OS networking, startup, renewal,
screen/input permission, relationship grant, or consent-record integration.

## Boundary

Separate explicit `mnet-pair-development` command, development build tag only.
Existing setup/connect command semantics and existing registrar/source protocol
are unchanged. Both owners use the installed save command, their own key and
pending journal, actual browser decisions and standalone registrar. The
coordinator stages both browser Returns before releasing either callback. It
requires both actual save exits before either separate connect is launched.
No bare counterpart, copied identity, reset, adoption, replacement, retry or
budget refund. A partial attempt never refunds a connection slice already spent;
if setup misses the pre-connect window, neither connection is launched.

Public config is DATA, not authority. Strict exact fields, canonical base64
public keys, distinct identities/IPs in 100.64.0.0/10, exact role, original earlier
expiry and dummy per-run body. Config is capped at 2048 bytes. The native owner
independently validates its saved journal, installed receipt/policy/state and
current authenticated map before binding. Snapshot helper exposes only public
self IP/key and original self/peer expiry. Exact peer key/IP/name and currentness
are rechecked after dial or accept and before application payload. Replacement,
missing/ambiguous key/IP matches, endpoint absence, wrong remote IP fail closed.

B's listener binds only its userspace tunnel IPv4, port49175. No host listener,
OS-TUN fallback, arbitrary application destination or legacy connection path.
Readiness follows session endpoint publication, exact peer binding and listener
registration. It contains only public stage/time/IP/port. A waits for that
public coordinator gate, then refreshes its exact peer and endpoint once.

A opens one probe stream and one HTTP stream. Both first exchange fresh random
32-byte challenge/SHA256 acknowledgement. The old probe stream ALSO successfully
answers a second pre-expiry fresh challenge, so the later denial is not an
unimplemented echo-server artifact. B keeps servicing that old stream until its
scope closes. HTTP is GET, fixed Host/path, no proxy, redirects, keepalive,
connection reuse or redial, 4096-byte header cap, 256-byte body limit, exact
64-byte random hex body and one-second operation cap. Server refuses bodies,
chunking, wrong Host/path/method, missing close and oversized headers.

After the original earlier expiry, A makes a NEW independent-context TCP dial
attempt, then attempts fresh Write/read on the existing probe stream even if
SetDeadline reports it closed. Both must fail. B's exact peer scope owns its
listener and all accepted streams. The extra Accept must return on closure,
accept/application and old-probe goroutines join, and final counters must still
be two initial proofs, one pre-expiry old-stream proof and one HTTP response.
No post-expiry accepted application acknowledgement is permitted.

## Unproven timing envelope

Historical logs have about9.9s after counterpart ready, not stage-duration data.
The first callback releases at T, BEFORE any receipt commitment. Both decisions
are already staged; each callback commits once, serially to respect unchanged
source max_per_source1, before either connection starts. Dual save must finish
and both connects must start by T+1s. Binding/listener must exist by T+3s. The
remaining dial/proof/HTTP operations each use stricter phase caps, and ALL
positive application work must finish by min(T+9s, originalEarlierExpiry-1s).
Additional time is only for expiry denial/cleanup observation, never authority.
New dual-owner/listener cost is not measured. These caps may be too tight; a
miss is inconclusive and must not trigger a replacement, retry or wider window.
Every coordinator phase and native observed setup event has a wall-clock time;
native adapter events include their actual wall-clock timestamp. Wall clock
scheduling is not a proven all-load timing guarantee.

## Finite request accounting

Same existing per-owner14-request connection reservation, spent once before any
map/device work. No background map poll and no new quota or persistent grant.
Actual success-path registrar requests:

- A: initial map1, endpoint publication1, session map1, BindPeer current map1,
  readiness current map1, postdial map for each of two connections2 =7.
- B: initial map1, endpoint publication1, session map1, BindPeer current map1,
  prepayload map for each accepted connection2 =6.

Cached/public snapshots and local25ms journal/authorization checks make no HTTP
request. No refresh for the repeated established challenge. Unused allowance is
not returned. There is no request after expiry for these application oracles.

## Validation and remaining gates

Available compiler is Go1.26.6. Repository still requires Go1.26.11 unchanged.
Required-toolchain local build refuses; record retained. Native race compile,
protocol/config/identity/time negative unit tests and existing ownership/map/
peer-expiry unit regression pass using a separate temporary modfile whose only
difference is go1.26.6. This is NOT exact-required-toolchain validation.

Paired browser coordinator has syntax-transpile and scoped ESLint validation,
not a successful full app typecheck. Full app dependency graph is deliberately
not retried after earlier memory-limit failures. Shell syntax check only.

The coordinator/test and runner are gated by MNET_PAIR_EXECUTION_REVIEWED=1 to
prevent accidental discovery-time execution. That variable is a mechanical
switch, not permission. Independent source review/release is required before
setting it. One actual paired run, if released, must be reported as success,
inconclusive timing, or failure without replacing its identities or retrying.
Browser screenshots and real listener/HTTP/expiry proof remain unverified until
that separate run. No live user-owned account/device/network is used by this
candidate or permitted by the test fixture.
