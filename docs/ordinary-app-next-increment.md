# Smallest product-directed increment after the finite pair proof

Proposal and source-grounded design only. Not a new execution, duration grant,
normal entry point, startup service, OS route or activation decision.

## What is actually missing

The finite proof exchanged application bytes between two genuine installed
saved identities, but its HTTP client lives inside the test adapter. Ordinary
apps cannot reach this userspace network. Native development deliberately
requires an injected TUN and refuses ManagedIface/LocalDNS. Normal daemon
startDevice can create an OS TUN, but ordinary Up includes older join/rejoin
paths, so connecting that path by default is NOT a safe small change.

Useful duration is a separate policy/authority gap. The current receipt is
immutable and binds access_until at issuance. The registrar computes this from
its maximum lease, reserve deadline, challenge and source-observation bounds.
An adapter cannot legally improve duration by extending a deadline, reissuing a
receipt, retrying under a new identity, keeping a stream open, trusting the local
ledger as authorization, or reusing relationship-consent DATA as network access.
The existing user-approved branch scope does not approve unattended/recurring
network access, permanent devices, a service install or real-account activation.

## Recommended smallest implementation step

A development-only explicit local TCP application ingress, not an OS VPN:

- A caller opts in to one destination, the exact already authenticated B peer
  key/IP and fixed49175 service, not a general proxy target. Bind127.0.0.1 with
  an ephemeral port and immediately own the listener in the existing peer scope.
- Accept exactly one local connection. Invoke an ordinary application with an
  explicit one-command URL, e.g. curl to that returned loopback address. Never
  edit proxy environment, browser/system settings, DNS, routes or startup.
- Internally dial the one pinned userspace peer, refresh exact currentness and
  perform the existing fresh challenge before forwarding the bounded HTTP
  request. Fixed Host/path/method/body caps remain, no arbitrary CONNECT, SOCKS,
  DNS resolution, redirects, credentials, keepalive or second connection.
- Own/close local listener, accepted app connection, remote stream and forwarding
  owners together at stop/expiry. Local-only is not an authentication boundary:
  another local app could race the single accept. Use an unguessable per-run
  path token, do not log it, and refuse mismatches without contacting the peer.
  This one-shot capability is not a new persistent login or blanket local access.
- Persistent public evidence must capture listener/child spawn/exit, exact peer
  provenance, before/after reservations and retirement. Never capture app input
  beyond allowlisted dummy fixture bytes. No real user data in the first check.

This adds the missing ordinary-process ingress while retaining the tested
userspace path and original finite authority. It is more product-directed than
another internal HTTP fixture and smaller/safer than enabling the existing
normal daemon. It still is NOT transparent networking or a usable VPN. The
current proof ran with only about190ms spare on B's save budget; adding curl at
runtime is unmeasured and can be inconclusive. Prebuild/stage all code and app
arguments first, retain the same positive work cap and no-retry rule. No such
listener or curl process has been created by this proposal.

### Source preparation versus execution

Source-only work can add pure destination matching/HTTP ingress parsing,
resource-owner state machine and unit tests under the development tag, with
normal entry points unchanged. A unit should reject arbitrary target, peer/key
replacement, second local connection, token mismatch, expired grant and late
payload; ownership tests must prove local Accept and app forwarding join.
Actual loopback ingress/curl integration, a fresh pair and new source receipts
require a separately released bounded check. Current one-run releases are spent.
No new integration is implied by preparing these sources.

## Next true networking milestone, not hidden by the ingress

OS-TUN/network-interface integration for ordinary apps that cannot opt in to a
local URL needs its own design/review: platform privilege, exact interface and
route ownership, no default/subnet/exit route, no unrelated DNS change, rollback
on every failure and expiry/stop, conflict refusal, crash owner cleanup, and an
isolated capable runner before touching the user's machine. Reuse installed
native identity/source checks, not legacy join/rejoin. A dedicated local forward
is not evidence that these OS requirements are solved. No OS implementation or
privilege action is released by this document.

## Duration design, still no human authority to activate it

Prepare a separate proposal separating short enrollment-challenge freshness
from an explicitly attended runtime grant. A grant must pin own device and peer,
network/purpose, source-session/security generation, explicit start and stop,
maximum runtime and revocation/expiry semantics. Currentness must be checked at
start and during that longer runtime using a purpose-specific bounded policy;
old14-request one-shot slices cannot become an unlimited polling/retry budget.
No default duration or renewal interval is selected here. Offline/lost-currentness
must stop, not silently maintain or renew access. Logout, source withdrawal,
key replacement and peer opt-out need explicit behavior on both endpoints.

Before real use, the person must see and approve the particular devices/peer,
what access is opened, attended duration/stop, whether repeated starts are allowed
and any unattended/startup permission. A selected duration also needs matching
operator/service policy and independent security review. Browser joining consent
alone is not permission for these effects. None of this asks for screen/input
access, which remain separate tools and separate approvals.

## Decision for main

Prefer development-only fixed-destination local ingress source preparation as
the smallest ordinary-process bridge, while duration and OS networking remain
separate named gates. If transparent VPN behavior is required immediately, skip
this bridge and plan OS ownership first; do not call a local forward completion
of that requirement. No human question is needed for isolated source drafts;
main should resolve actual runtime authority before any real activation.
