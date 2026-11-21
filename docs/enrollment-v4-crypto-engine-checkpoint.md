# Disconnected experimental-v4 crypto engine

New internal/enrollmentproof implements the existing exact v4 transcript key schedule and separate server-confirmation arithmetic. No daemon/control/source consumer, bootstrap route/scheme, parser, authenticated delivery, source-policy authority, permit or credential installation exists. Older internal/keypossession experimental-v2 is unchanged and is NOT silently upgraded into v4 authority.

## Ephemeral ownership

NewServer takes untrusted Binding DATA and explicit now/expiry/maximum lifetime. It owns CSPRNG challenge ID/nonce/fresh X25519 private key and exact immutable Challenge. Device low-order/all-zero DH output refuses. Maximum lifetime must be explicitly supplied, positive and within an engineering15-minute ceiling; this does not select source freshness/access/eligibility lease. Challenge expiry is also constrained by the existing transcript access/reserve times. No global ID collision, live/per-attempt capacity or failed-bootstrap budget is claimed. Those checks must precede calling this disconnected constructor in any future authenticated owner. Opaque all-zero draws are retried at most four times, not an unbounded entropy loop.

Server.Verify selects ONLY its own stored challenge, accepts exact challenge-ID echo plus raw32byte response and explicit time, and compares response MAC in constant time. It cannot resolve arbitrary caller-supplied binding/challenge/expiry. It is deliberately side-effect-free: correct proof is NOT consumed independently of Store publication. An exact retry returns identical DH server confirmation under the original challenge/deadline; no ID/nonce/expiry replacement. Future consumed-proof/node/receipt publication still requires the actual Store owner and private authenticated evidence.

## Native response ownership

Respond takes a caller-supplied private-key VALUE, full challenge DATA and independently supplied exact expected Binding. Every binding field must match before DH. It verifies that the private key derives the expected public key, rejects malformed/low-order server keys and derives the dedicated response MAC. It returns a copy-safe private lifecycle handle containing only temporary shared output and its exact pinned response/transcript, no private-key/shared-key getter. There is deliberately no bridge to actual native State key.

Response and Confirm use only that outstanding owner. Confirm verifies the separately labeled confirmation MAC for its own exact transcript and response; cross-owner confirmations and response-as-confirmation refuse. Successful confirmation proves ephemeral shared-key knowledge ONLY, never authenticated registrar identity, eligibility, durable Store commit, enrolled status or current network access. A real native consumer must enforce device-initiated exact independently trusted registrar HTTPS delivery and own saved PKCE context BEFORE passing its durable key. Calling Respond on unsolicited matching DATA would still be wrong.

Close is idempotent and shared across value copies. Server drops ephemeral private reference. Native clears temporary shared/response buffers; derived keys/shared scratch are cleared best-effort. Go/crypto runtime may retain copies, no secure-memory-erasure claim. Time before initial admission or at/after expiry refuses. Caller time remains DATA, not a trusted clock or elapsed-time lease. There is no timer/cleanup goroutine or server retention policy.

## Evidence and remaining gates

Actual random X25519 two-party tests cover response/confirmation, wrong key/low-order peers, every independent binding field, invalid challenge/clock/lifetime, cross-challenge/owner replay, wrong ID, exact nonconsuming retries, fresh entropy, expiry, copy/Close and concurrent verification/Close. Actual implementation key schedule matches the existing independently generated Python response/confirmation vectors. Those vectors pin arithmetic, not peer identity or protocol security review.

No file/Store changes, native key invocation, source request, live credential, account effect, network/screen/input access, push or deployment occurs. Authenticated bounded registrar owner, native exact-channel admission, strict transport parser, private verified evidence, source time/lease rules, durable challenge history and atomic consumption/finalization remain unimplemented. Never turn a matching public transcript, Server pointer or confirmation into a permit.
