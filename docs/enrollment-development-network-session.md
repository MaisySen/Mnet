# Explicit disposable connected-session increment, not accepted

This adds a separate optional development-only network action after the accepted
historical joining result. Only the explicitly tagged mnet-setup-development
binary understands --disposable-network-tcp. With that flag absent, ordinary
setup still exits without starting a network. Normal CLI/startup/local API,
OS TUN/service, production ingress, RD host and SharedDesk remain unchanged.
No screen/input consent is inherited.

## One actual journey

The real installed tagged command opens its system browser. Signed-out local
Multies login, full-key comparison, workspace choice, approval and Return feed
the same retained native owner. It proves/finalizes the real source/Store result,
checks the actual current map and reports joining saved before optional display.
The explicit network action then injects a proofnet in-memory TCP/IP device,
starts WireGuard with the SAME already-durable key, publishes only a loopback
UDP endpoint and verifies fresh TCP challenge/response data with the exact
current approved peer at nonreserved port49173. No OS interface is created.

The counterpart is enrolled only AFTER the real browser finishes login and
workspace selection, BEFORE approval/Return completes native proof. This moves
work inside the original intervals instead of widening the15s fixture leases.
The initial native map now truthfully has two live peers. A browser login that
outlasts an earlier authorization still fails honestly; no production lifetime
or recovery policy is selected here.

The current-map selected peer must uniquely have an endpoint. Its exact
key/IP/name is source-checked before and after dialing. Actual proofnet returns
*net.TCPAddr with the pinned mesh IP; this was executed, not merely source-read.
A random32byte request and the counterpart's SHA256 response prove fresh data
both ways over actual WireGuard. The historical browser result opens only after
the peer scope and network owner have stopped. It is not a live status indicator.

## Lifetime and denial

The tagged RunDisposableEnrollmentNetwork API requires mandatory injection,
a finite shared request budget, and a caller callback. Its timeout must equal
the existing workflow limit. The original State/policy/journal and native receipt
remain authoritative. There is no OS fallback, reconnect, renewal or reset.
Every map is validated before IPC/netstack parsing: canonical keys/IPs, unique
identities, finite approved peers, same network, no routes/exit nodes/relay URL,
and loopback-only UDP endpoints. Ambiguous candidates refuse rather than guess.

The controller runs existing known-map-expiry removal. Original native receipt
expiry, cancellation or changed State/policy stops the network. Refresh denial,
unavailability or immutable request-budget exhaustion also ends it. The explicit
command performs finite two-second local-source checks, capped by sixteen shared
requests and the original receipt. This is not production polling or instant
offline account revocation. There is no unlimited background refresh.

An exact peer scope has its OWN original peer deadline. It cancels/closes its
consumers at that deadline without waiting for another source request. A later
map removing/changing key/IP/name also cancels/closes it. CurrentSessionPeers
performs an explicit source check; Peers is only a local snapshot, never a
persistent admission grant. Registered listener/connection/Mux resources close
before device teardown; callbacks still must join their own workers. Arbitrary
blocking consumer Close implementations cannot be forcibly joined.

Actual installed runs separately verify:
* Fresh encrypted TCP followed by actual Store deletion/source-checked peer
  removal, existing-connection closure and joined network/counterpart owners.
* SIGINT AFTER actual TCP verification, retaining the saved native credential
  and all three remote nodes until fixture cleanup. It does not promise rollback
  or open a confirmed-withdrawal success page.
* The selected peer's ORIGINAL lease expiry, with no Store deletion/renewal. Its
  scope closes and the network ends, retaining the saved credential and nodes
  until fixture cleanup. Historical copy distinguishes original peer expiry
  from source-checked removal. Native receipt expiry is separately exercised
  through the actual controller/API path.

The first API-expiry and installed-expiry attempts had stale fixture assumptions
that expired credentials should still return an authorized map, and that the
older counterpart could not expire before the native credential. Their logs
remain failed evidence. Controls now require the respective original deadline
and deny access afterward; no timing policy was widened.

## Still not the final product

This is a disposable local connected TCP proof, not OS routing, a persistent
background network, two physical devices or release readiness. No RD viewer,
real attended-view approval, screen capture or pointer control ran through this
new connection. The supplied RD contract harness uses loopback wrappers and is
not silently substituted as human approval. The next bridge must use this actual
userspace net.Conn with exact current identity and fatal peer context, separately
exercise host default-decline/per-run viewing and separate pointer decisions,
and keep its listener/Mux/display owners joined. Normal5909 and SharedDesk are
outside this increment. Broad affected-entry TypeScript remains unpassed after
prior memory exhaustion; runtime browser success does not replace that check.
