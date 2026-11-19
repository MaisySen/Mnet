# Multies authorization boundary (Mnet side only)

`internal/multies` is a deny-by-default policy foundation. It is **not wired into a server route or client**, has no production verifier/transport, creates no Multies session and does not enable integrated enrollment. The zero-value policy rejects every request.

A future trusted Multies bridge must verify the person session and resolve current active account, non-demo workspace, membership, SSO and exact device/capability entitlement. It must bind these to the authorization attempt and device key. Caller-supplied JSON evidence, an identity-provider subject/email, a general MCP OAuth token or an old enrollment key cannot replace this bridge. Email is never an ownership key. No Multies code or identity-provider configuration is changed by this package.

The policy checks exact immutable user/workspace/device binding, an explicit audience and requested capability, fresh evidence and bounded expiry. View does not imply control, file transfer or recording. Workspace list and device-scoped operations have different shapes. It revalidates on each check instead of caching local authority. Upstream failure, cancellation, missing verifier and all mismatches return the same empty denial, without leaking upstream error details.

Current local ceilings are15seconds evidence age and60seconds remaining grant lifetime. They are conservative policy limits for development, not tested production revocation or latency promises. The final bridge needs a measured freshness/revocation contract. Host consent, one-use authorization-code exchange, PKCE, allowed callbacks, durable ownership storage, device-credential issuing, session termination and revocation propagation are separate requirements. An accepted policy result is one operation's decision, not a transferable credential or completed consent flow.

The `Verifier` interface is deliberately the trust boundary. A transport implementation must authenticate Multies, verify cryptographic bindings and fetch current authority; merely decoding claims is not sufficient. No route should accept an `Evidence` supplied by a client. Do not add a fake verifier to a runnable production configuration to make setup appear finished.

## Staging inventory store boundary

`internal/control.MultiesInventory` uses the control store's atomic save/rollback, with separate workspace-bound inventory records. Its in-process constructor requires explicit staging opt-in, one exact workspace and a trusted verifier; the in-process consent and removal methods are not exposed by any command, HTTP route or native prompt. It lists active records, persists signed one-use enrollments, and records removal while discarding outstanding consent for that workspace/device key. Pending consent intentionally does not survive a restart. No mesh peer, relay access, session or device bearer credential is created, so inventory removal does not revoke an existing Mnet mesh node.

### Separate read-only listener

The list-only listener is absent by default. When the process is explicitly configured for staging, the separate listener binds only to a numeric loopback address and requires TLS 1.3 with a dedicated client CA. It accepts the exact configured HTTPS authority and workspace list path. The verified leaf must contain exactly one URI SAN, `urn:multies:staging:mnet-device-list:<workspace-id>`. Use a dedicated client identity authorized only for this list; the receiving listener never accepts a person's cookie, the public control password, or a Mnet network key as authority. Do not terminate TLS in front of this listener and substitute authentication headers.

Multies performs the person authorization immediately before requesting the list. Mnet authenticates Multies on the request's TLS channel and reads the same store projection as the in-process inventory. The response has only device ID, label, and enrollment time. No user claims are forwarded and there is no mutation route, device credentials, or session control. This does not configure deployment connectivity or establish shared-staging readiness.

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
