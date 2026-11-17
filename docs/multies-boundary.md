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
