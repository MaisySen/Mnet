# Experimental exact-origin registrar bootstrap client

Disconnected client only in internal/enrollmentregistrar. No production caller, native durable key/crypto access, pending journal loader, installer, State write, bootstrap server route, source verifier, permit, receipt producer/current-control API or finalization exists. Main application/legacy controlclient unchanged. Schema candidate consistency is not used as authority or channel provenance.

## Explicit inactive transport proposal

New experimental POST endpoints, exact configured HTTPS origin only:

- /api/mnet-registrar-experimental-v4/challenge
- /api/mnet-registrar-experimental-v4/response

Authorization is exactly Mnet-Enrollment-Bootstrap-V4 followed by one space and the canonical native secret. This is bearer-on-wire within verified HTTPS, not proof-of-possession authentication of a peer endpoint, and not a legacy token or membership/admin credential. No server implements these paths/scheme here. Origin must exactly equal separately supplied expected Binding.RegistrarOrigin, never selected from a response. Constructor checks canonical nonzero secret TEXT digest against expected binding, but cannot prove binding source or that secret was durably saved. Calling New with caller data creates no permission. No private key operation follows from the returned challenge.

Challenge body has exactly version4/attempt_id/handoff_id/reserve_id, no secret/full binding. Server design must independently authenticate SAME owned attempt/reserve/source before challenge allocation, not accept body/digest as a permit. Client accepts existing strict ChallengeWire only when full binding exactly equals pinned expected binding. All values remain observed DATA. Actual time must be after expected checked-at and before access/reserve deadline. Challenge must be live, with both remaining interval and interval from checked-at within an explicit caller-supplied max engineering lifetime, capped15min. This bound is not source freshness, eligibility or lease policy.

Only one challenge request is admitted per shared owner. It spends before I/O; invalid response, TLS loss, timeout, wrong scope, redirect or cancellation never refund the slot. Submission requires own pinned challenge ID, live original deadline and nonzero canonical32-byte MAC. It sends only strict version4/challenge_id/response_mac, never caller binding/key/expiry. Invalid local ID/MAC does not spend submission, but once admitted submission spends before I/O regardless of outcome. Returned strict same-ID ConfirmationWire is DATA, not cryptographically verified by this transport, and never means a node was durably committed. No receipt/history response exists in this checkpoint.

## HTTP and lifecycle

Private constructor owns fresh transport/client, no caller HTTP/TLS/root/dial injection API, environment proxy, DefaultClient/DefaultTransport, cookie jar, ambient auth, redirects or plaintext downgrade. Normal certificate/hostname validation, TLS1.2 minimum, HTTP1.1 only, no keepalive reuse, body rewind disabled. No automatic application retry, no legacy X-Node headers or auth-secret query/body. Both endpoints use no-store request headers; response must be200/exact no-store/one JSON Content-Type/no encoding and bounded16KiB body/header. Raw error bodies, Locations, URLs and transport causes never reach returned error strings. No cache/pool/HTTP2 stale retry path.

Client copies share mutex/one-shot budgets/lifetime. Close marks closed, cancels in-flight request, closes idle transport, removes reachable secret and pinned challenge before returning. It does not wait for remote handler completion, guarantee memory zeroization or undo a spent remote request. Post-I/O checks suppress late data. Request-local string/header copies and test JSON fixtures are not secure zeroizable vault storage. A future live secret owner must address storage/exposure policy before activation. This client is inactive and never attached to a native credential source.

Bounds are per client lifetime, not durable global admission. New can be called again, so reconstructing it cannot erase real pending/unknown history. Future native owner must pin actual key/policy/origin/current own-PKCE context and durably journal exchange-may-have-started before I/O, then own crash/restart reconciliation and exact source/receipt authority. No durable claim is made here.

## Evidence

Real local TLS fixtures use explicit test-only certificate root trust, never production insecure skip verify or injected transport. Cover exact paths/headers/bodies, zero legacy/ambient auth/cookie inheritance, pinned full binding and ID, one successful challenge/submission, concurrent20-copy one-shot budgets, malformed/wrong-purpose/wrong-ID/wrong-origin/body-limit/cache/encoding/status/redirect/TLS-untrusted/lost-response refusal, no redirect sink hit, no retry, Close/caller cancellation and expiry after response, checked-at/remaining lifetime bounds, invalid secret/origin zero-send rejection. Error/candidate formatting never includes secret or upstream text. Confirmation tests intentionally do not claim cryptographic commit acceptance.

Actual owning daemon/Store bootstrap and current-control transport are NOT covered by these fixtures. They have no route to receive this scheme. No browser, account, source, real network or deployed service is contacted. No rollout/release/old-binary/physical acceptance implied.
