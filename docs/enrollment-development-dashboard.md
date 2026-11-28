# Disposable live network dashboard

This remains an explicit tagged development command. No normal startup, service, OS interface, screen or input path is added.

Build `cmd/mnet-setup-development` with `mnet_enrollment_development`. Supply the same authoritative disposable private root, source policy and finite limits as the existing setup command. Select `--disposable-network-dashboard`, not `--disposable-network-tcp`. The selectors are mutually exclusive. No implicit default activation occurs.

After actual joining/finalization and fresh encrypted userspace TCP verification, the same owner opens a local dashboard. Joining saved is separate from Network active. Peers come from the validated native snapshot, with source checks still bounded by the original request budget and interval. The page is not independent proof of instantaneous remote revocation.

Stop requests the same network owner to stop. The page hides peers while stopping and cannot show ended until the network function and its device/resources have returned, plus the Stop worker has joined. The HTTP listener remains only for a three-second post-join display grace and then shuts down, closes connections and joins handlers. A browser that misses that grace shows Live status unavailable, not a false confirmation. Reloading after shutdown does not resurrect a session. A tab already showing ended is historical.

The listener binds literal 127.0.0.1 on an ephemeral port. A single-use 256-bit bootstrap capability is exchanged for a per-run uniquely named host-only HttpOnly SameSiteStrict cookie, followed by a redirect to the clean local URL. Cookie names must differ because cookies are not port-scoped. Exact Host and loopback remote are required. Stop requires an exact Origin, per-run CSRF header, JSON content type and fixed empty-object body. CSP forbids framing, external resources and arbitrary connections; referrers and caching are disabled. The bootstrap and cookie are not put in HTML. The CSRF token is intentionally present only in the authorized page. No registrar secret/private key is exposed.

No generic unauthenticated control endpoint, reconnect, renewal, OS device, physical-device activation, screen/input authority or production configuration is added. The development caller's stdout carries the short-lived bootstrap URL as a normal local UI event and must remain private.

## Validation scope

Actual installed browser joining/approval/return, actual enrolled same-key WireGuard/proofnet TCP, active current-peer page, clicked Stop, stopped owner, ended page and original selected-peer expiry were exercised. Active and ended pixels were inspected. Capability/origin/CSRF/bootstrap replay/cross-port cookie/host-rebinding/requested-versus-joined state checks pass under race count 3. The existing 12 cross-app cases pass. Exact final Stop/expiry browser replays and tagged vet/build pass.

The attempted full default Go race command hit its 120-second outer execution limit after reporting passes through enrollmentidentity; later packages and the chained default vet/build did not complete in that attempt. This is not a full default-suite pass. A separate final default build passed. Broad TypeScript remains unpassed from earlier memory limits. Original failed browser launches and test-configuration errors are preserved as failed evidence, not retroactive passes. The UI remains engineering-facing, not a finished Multies consumer experience.

## Corrective candidate: monotonic browser status

The original 687a087 client has a known display race. Overlapping `/state`
requests can deliver an older active response after a newer ended response.
That older callback can restore Network active and enabled Stop while the
internal ended flag prevents further polling. A late Stop error can also replace
the terminal reason. Narrow successful native/browser runs did not rule these
out. A general truthful-terminal-display claim for that old client is invalid.
This is a UI defect; it does not extend native authorization or keep a session
alive. The old immutable review head remains evidence, not a corrected version.

This separate corrective candidate serializes status reads, rejects responses
from before a Stop intent, makes Stop intent and server stopping monotonic
non-actionable states, and rejects every late success/error/Stop callback after
ended. Unavailable clears peers and disables Stop. A later fresh status request
can recover from ordinary unavailability, but no concurrent older response can
restore controls. After a Stop request the page remains non-actionable even if
an intermediate response says active; an unconfirmed Stop explicitly warns that
the network may still be active. Only native ended reports terminal completion.
There is no timer, authority, budget, service or account change.

The companion `tests/mnet-dashboard-client-order.test.ts` runs the exact checked
out client script in Chrome with controlled responses. It tests serialization,
late active after Stop/stopping, unavailable, late Stop error after ended and
Stop error followed by active. These are deterministic client fixtures, not
WireGuard or native-owner proofs. `tests/dashboard-client-order-before-repro.mjs`
against an exact original Mnet checkout reproduces ended -> stale active with
screenshots and the SHA256 of the actual source used.

Separately, installed-browser timing tests now finish the approval/Return page
before peer commitment, foreground the actual dashboard tab, record actual
original peer commit/expiry timestamps, and use that deadline plus a bounded
observation margin. The original receipt is never moved, renewed or widened.
Observed original access was about10s under the unchanged challenge/policy,
within the configured15s maximum. Timing-only execution passed3/4 with one
ended-wait failure; a later diagnostic run passed. The exact client race is NOT
proven to explain every installed timeout. Retain those failed observations.
The corrective candidate then passed four consecutive installed expiry runs
and one real Stop run at author execution. This does not prove universal timing
stability. Full repository default race and broad TypeScript remain unfinished.
No rollout or consumer-ready claim follows until separate review.
