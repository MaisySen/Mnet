# Explicit installed development setup, incomplete local checkpoint

This is not normal setup or a release. Build tag `mnet_enrollment_development`
adds a separate `cmd/mnet-setup-development` binary and its daemon adapter.
Normal CLI/home/localapi/installers/startup remain unchanged. The independent
cryptographic dependency experiment is excluded. The maintained engine remains.

The entry requires a fresh private canonical root under the OS temporary folder,
a `DISPOSABLE-MNET-SETUP` marker containing `mnet-disposable-enrollment-v1` plus
newline, explicit policy-v2 and workflow limits. Both contacted services must be
literal HTTPS127.0.0.1 with explicit ports. It refuses a preexisting native
directory rather than loading or replacing an ordinary daemon's State. The
marker is accidental-use confinement, not defense against an adversarial same-UID
operator or a substitute for trusted configuration. Only disposable fixture
accounts/configuration are in scope.

The tagged entry creates a new retained native owner, prepares its identity,
emits the owned handoff URL, optionally opens the system browser, accepts the
owned callback, exchanges/proves/finalizes through existing methods, checks
same-State/journal correlation and reads the actual current map. It emits
`joined-not-connected` only afterward. No tunnel, endpoints, relay, screen,
keyboard or mouse starts. An interrupted/failed attempt stays on disk. This
entry does not implement recovery/reset or select production durations.

## Observed new evidence, not accepted

The binary was explicitly built into a new disposable private prefix. A real
xdg-open/system-Chrome journey reached signed-out local login, actual full-key
consent/workspace selection, Return, owned callback, real source proof, durable
finalization/current map and Store deletion. A separate API-speed installed run
also passed. The browser run returned zero live peers: earlier synthetic peers
expired during browser login under unchanged15s fixture leases. That is valid
current-map evidence, not connected traffic. No policy was extended to hide it.

Nine zero-side-effect refusal families pass three race runs: missing marker,
remote source/registrar, long timeout, unknown/trailing limits, symlink root,
existing native State and cancelled context. Default full Go race and tagged
vet pass. Browser launch now writes a private log instead of inheriting the setup
output pipe; the earlier pipe-inheritance join hang is retained as failed
evidence. Early browser timeout/display-fixture and stale two-peer expectation
failures are retained, not counted as passes.

## Known next work before review/readiness

* Browser copy still describes the earlier disconnected handoff, and callback
  is an interim plain page. Actual pixels were inspected: readable, but these
  claims are superseded and must not be user-delivered. Final native status must
  be observable and tied to actual success/failure/cancel, not browser return.
* The private network identifier wraps one trailing character in the narrow
  card. Improve presentation while retaining the full comparison value.
* Browser test assertions pass, but its surrounding shell reports detached
  descendants. Join the entire disposable session before counting cleanup pass.
* Add actual browser decline, cancellation, callback-loss and configuration
  change controls; refusal tests alone do not cover them.
* Repeat final browser+tagged/default+full12 regression and strict affected-entry
  TypeScript checks; archive exact sources and request independent review.
* No real daemon process reconnect/OS interface/physical-device acceptance,
  consumer setup, reachable mesh traffic, installed service or release follows.

Source approval for local joining is separate/default-off development only.
Earlier zero-native-trigger language concerned publication/automatic native CI
coordination, not blanket prohibition of disposable local native execution.
Main explicitly resolved this lane before this new tagged caller was run.
