# Multies authorization boundary (Mnet side only)

`internal/multies` is a deny-by-default policy foundation. It is **not wired into a server route or client**, has no production verifier/transport, creates no Multies session and does not enable integrated enrollment. The zero-value policy rejects every request.

A future trusted Multies bridge must verify the person session and resolve current active account, non-demo workspace, membership, SSO and exact device/capability entitlement. It must bind these to the authorization attempt and device key. Caller-supplied JSON evidence, an identity-provider subject/email, a general MCP OAuth token or an old enrollment key cannot replace this bridge. Email is never an ownership key. No Multies code or identity-provider configuration is changed by this package.

The policy checks exact immutable user/workspace/device binding, an explicit audience and requested capability, fresh evidence and bounded expiry. View does not imply control, file transfer or recording. Workspace list and device-scoped operations have different shapes. It revalidates on each check instead of caching local authority. Upstream failure, cancellation, missing verifier and all mismatches return the same empty denial, without leaking upstream error details.

Current local ceilings are15seconds evidence age and60seconds remaining grant lifetime. They are conservative policy limits for development, not tested production revocation or latency promises. The final bridge needs a measured freshness/revocation contract. Host consent, one-use authorization-code exchange, PKCE, allowed callbacks, durable ownership storage, device-credential issuing, session termination and revocation propagation are separate requirements. An accepted policy result is one operation's decision, not a transferable credential or completed consent flow.

The `Verifier` interface is deliberately the trust boundary. A transport implementation must authenticate Multies, verify cryptographic bindings and fetch current authority; merely decoding claims is not sufficient. No route should accept an `Evidence` supplied by a client. Do not add a fake verifier to a runnable production configuration to make setup appear finished.

## Local evidence

- Exact/absent/wrong workspace and device binding, malformed canonical thumbprint, missing/disabled account, suspended workspace, removed membership, demo/guest, SSO block, unverified email, scope escalation, future/stale/expired/overlong evidence and absent verifier are covered.
- Tests cover cancellation, upstream-error redaction, repeated revocation revalidation,50concurrent workspace checks and a fuzz property that caller-controlled request fields cannot expand authority.
- Fresh50race repetitions, `go vet` and15seconds of two-worker fuzzing pass locally (over1.1million executions in the recorded run).
- This evidence verifies only this local policy, not the eventual verifier, live Multies authorization, Android eligibility or legacy-path enforcement. The old test-only enrollment paths have not silently become Multies-only.

## Required permission-gated integration change

Keep the future Multies-side patch small: a scoped authenticated authorization/one-use exchange surface, explicit device capability policy, revocation contract and a Devices navigation entry using the shared navigation model. Its exact code/config proposal requires separate authority while Multies is read-only. Never work around that restriction with a public identity provider or broad user cookie forwarded to Mnet.

## Inactive native handoff intent binding

`Intents` is a local development primitive, not the future bridge. It stores an exact workspace/key/S256-PKCE/callback-platform intent, never a Multies user, cookie, credential, approval or authorization result. Random32-byte correlation tokens are indexed only by SHA-256, formatting is redacted, pending capacity is bounded and full capacity does not evict another unexpired attempt. Native proof consumes the matching intent atomically once; wrong binding/proof cannot consume the owner's attempt. Cancellation and a development2-minute lifetime clear pending state. The zero value denies.

Callback values are a compiled-in platform enum, not URLs. Real callback app association, loopback port handling, browser state/CSRF, issuer authentication, one-use approved code exchange, fresh membership checks and device-credential issuance remain separate permission-gated design/implementation requirements. An intent token is only a correlation secret. It must never be treated as an enrollment key or browser approval, nor placed in logs, referrers or public query strings. The primitive has no routes, network client or production caller, and does not modify Multies. The final measured bridge contract may change these development ceilings and mechanics.

Local tests cover binding mismatch, proof mismatch, arbitrary callback rejection, capacity/expiry/cancellation, single-winner concurrent redemption, token formatting redaction and inability to turn a redeemed intent into a zero-policy Grant.50race repetitions and988,853fuzz executions passed in the recorded run. These checks do not prove live sign-in or production replay/revocation behavior.

## Inactive attended-session consent

`AttendedConsent` separately models host view approval, viewer control request, host control approval, control revocation and terminal stop. It is bound to one exact workspace, both user IDs and both device keys. A control-request generation prevents a late approval from a canceled request approving a newer request. Control always requires view; pending control does not grant input. Expiry, decline and End are terminal. Reconnect requires a new instance and fresh approvals; it cannot resume an ended object's input authority.

This state machine is not connected to current session transport or UI. It authenticates no actor by itself. Future adapters must verify current Multies grants, bind messages to a unique session/request, enforce OS permissions, call End on membership/permission/network loss and close actual capture/input/transport resources. Snapshot booleans are not persisted credentials. A development10-minute maximum lifetime is not a production session duration or revocation promise. The existing session implementation has not silently acquired these protections.

Local tests cover separate view/control, exact actor/device/workspace binding, cancellation versus late approval/new request, terminal expiry/decline, single-object concurrent stop/approval and zero-value denial.30race repetitions, vet and27,302fuzz executions passed in the recorded run. This does not prove live host consent, identity, traffic or remote input cleanup.

Adversarial review found that participant binding and a per-object control generation alone would not distinguish a later session with the same people/devices. The constructor now generates a fresh random256-bit session ID, rejects caller-supplied session IDs and includes that ID in every decision binding. A stale view/control approval from an earlier instance is rejected even when participants and request-generation numbers match. This remains an inactive primitive, not authenticated message transport.30race repetitions including the cross-session replay regression pass locally.

## Proposed production handoff contract, not an implemented integration

The current code in `internal/multies/policy.go` defines the precise missing dependency: a trusted `Verifier.Verify(context.Context, Request)` returning freshly resolved `Evidence`. `Request` contains an opaque credential, immutable workspace ID, exact device thumbprint and one capability. A list operation is workspace-scoped and must not carry a device key. `Evidence` binds the verified user, exact workspace/device, audience `mnet`, current account/workspace/membership states, SSO/guest/demo decisions, explicit allowed capabilities and checked/expiry times. A decoded request body is not this evidence.

The intended implementation split is concrete, but remains permission-gated:

| Stage | Multies-owned work | Mnet-owned work |
| --- | --- | --- |
| Begin setup | Expose a scoped authorization entry within the existing authenticated Multies experience. Show the exact workspace, enrolling device and requested capabilities. | Create a fresh native attempt bound to device key, platform callback enum, S256 challenge and correlation state. Open only the approved configured issuer. |
| Approve or decline | Resolve the current person/account, workspace membership, SSO and capability policy. Record the exact approved attempt or terminal decline. Issue a short-lived, single-use authorization code bound to that attempt and challenge. | Keep the native attempt inactive. A browser redirect, claimed user ID, email or public inspection grants nothing. Cancel closes the attempt without enrollment. |
| Exchange once | Authenticate the exchange service/issuer, verify the one-use code and proof, consume it atomically, recheck current authority and return only the narrow device result. Never hand a broad Multies cookie to the native client. | Verify issuer, native state/callback and exact attempt/device/workspace binding. Securely save only an issued device credential after success. Never log codes, proof or credentials. No legacy-key or fake-verifier fallback. |
| Use and revoke | Supply a measured freshness/revocation contract and authenticated current capability checks. Membership loss, disabled accounts/workspaces and removed capabilities must have defined terminal effects. | Implement the trusted verifier against that contract. Recheck each operation and tear down affected active resources on authority loss. Preserve separate host view/control consent and native OS permissions. |

Concrete route names, callback associations, cryptographic issuer details, storage schema, deployment/configuration and measured revocation bounds must be selected against the actual Multies application only after authority to inspect/change that integration is available. They are not guessed here. The current 15-second evidence age and 60-second grant ceiling are local development constraints, not a live service promise.

Acceptance before enabling a real Continue action must include wrong issuer/audience/workspace/key, altered callback/state/challenge, code replay and simultaneous exchange, terminal decline/cancel, stale or revoked membership, upstream outage, credential redaction, native restart and actual teardown of affected sessions. The present policy, intent and consent unit tests are useful prerequisites, not substitutes for that integrated acceptance. The installation slice can ship as source independently; the real first-run button must remain unavailable until this bridge exists and is verified.
